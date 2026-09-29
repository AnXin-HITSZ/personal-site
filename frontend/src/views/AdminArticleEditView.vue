<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import MarkdownIt from 'markdown-it';
import {
  articleLimits, articleProblems, createArticle, getAdminArticle, updateArticle, uploadImage,
} from '../api/admin-articles.js';
import { listAdminCategories } from '../api/categories.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText, uploadText } from '../copy.js';
import { clear } from '../session.js';
import { useUnsavedChanges } from '../dirty.js';
import { altFromFileName, imageMarkdown, indentLines, insertAtCursor, outdentLines, shortenURL, tabIntent } from '../insert.js';
import { formatMonthDay } from '../format.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import FormSelect from '../components/FormSelect.vue';
import FormTextarea from '../components/FormTextarea.vue';
import NoticeBanner from '../components/NoticeBanner.vue';
import StatusChoice from '../components/StatusChoice.vue';
import TagInput from '../components/TagInput.vue';

const route = useRoute();
const router = useRouter();

const isNew = computed(() => route.name === 'admin-article-new');
/* 分类那一格留空等着读回来的第一个——它就是默认分类。 */
const form = reactive({ slug: '', title: '', summary: '', body: '', category: '', tags: [], status: 'draft' });
const local = reactive({ slug: '', title: '', summary: '', body: '' });
/* record 是库里那一篇，form 是手上这一稿。注文栏只放前者，输入框只放后者。 */
const record = ref(null);
const baseline = ref(snapshot({}));
const state = ref('loading');
const failing = ref(null);
const saved = ref(null);
const busy = ref(false);

/* 分类是一份随写作变的数据，不是一张写死的表，所以它得现取。取不到的时候这一格
   不能画一个空的下拉——一个点不开、或者点开什么都没有的控件只是占位。 */
const cats = ref([]);
const catsState = ref('loading');
const categoryOptions = computed(() => cats.value.map(cat => ({ value: cat.id, label: cat.name })));

/* 这一格说不了话的两种情况，各给各的一句：这次没读到，和真的一个都还没有，
   不是同一件事，去路也不同。已经存过的那一篇不受影响——它本来就归在某个下面，
   原样存回去就是了。 */
const categoryProblem = computed(() => {
  if (catsState.value === 'loading' || cats.value.length) return '';
  /* 已经存过的那一篇不受影响：它本来就归在某个下面，这一格收起来也不挡着保存。 */
  if (!isNew.value && record.value && catsState.value === 'failed') {
    return '分类没读出来，这一格先收着。这一篇仍归在原来那个下面，保存不受影响。';
  }
  return catsState.value === 'failed'
    ? '分类没读出来。文章必须归在其中一个下面——先重试一次，再回来存这一篇。'
    : '还没有分类。文章必须归在其中一个下面——先去建一个，再回来存这一篇。';
});
/* 去路只有两条：这次没读到就再读一次；真的一个分类都没有，就去建一个。 */
const retryCategories = computed(() => catsState.value === 'failed' || Boolean(record.value));

const bodyArea = ref(null);
/* 正文那一面。进来是原文——这一页是来写字的，预览是校对时看一眼的东西；
   换到另一篇（同一个组件被复用）也回到原文。 */
const mode = ref('source');
const uploading = ref(false);
const uploaded = ref('');
const uploadError = ref('');

/* markdown-it 默认 html: false，正文里的 HTML 会被转义后原样显示——和详情页同一套。 */
const markdown = new MarkdownIt();
const preview = computed(() => markdown.render(form.body));

function snapshot(source) {
  return {
    slug: source.slug ?? '',
    title: source.title ?? '',
    summary: source.summary ?? '',
    body: source.body ?? '',
    category: source.category ?? '',
    tags: [...(source.tags ?? [])],
    status: source.status ?? 'draft',
  };
}

/* 「有改动」要拿现在的值和载入时那份快照比，不能敲一下就置个 true：
   打了字又删回去，就该回到干净。 */
const dirty = computed(() => JSON.stringify(snapshot(form)) !== JSON.stringify(baseline.value));
const { blocked, discard, release } = useUnsavedChanges(dirty);

const stored = computed(() => record.value !== null);
/* 撞名那一刻，提示要说的是「换个名字就行，改这一下不会弄坏别的东西」——人这时
   第一反应是「改坏了」。其余时候说的才是这格该怎么填。 */
const slugHint = computed(() => {
  if (serverFieldError('slug')) {
    return '换个名字就行。链接不会因此失效——地址里真正管用的是 id，旧链接照样能打开。';
  }
  return stored.value
    ? '改它不会让任何既有链接失效——地址里真正管用的是 id。'
    : '小写字母、数字和连字符。以后可以改，链接不会失效——地址里真正管用的是 id。';
});
const heading = computed(() => (isNew.value && !record.value ? '写新的一篇' : record.value?.title ?? ''));
const publicAddress = computed(() => (record.value ? `/articles/${record.value.id}/${record.value.slug}` : ''));
const formatCount = value => value.toLocaleString('zh-CN');

const metrics = computed(() => {
  const current = record.value;
  if (!current) return '';
  const minutes = `约 ${current.readingMinutes} 分钟`;
  return current.bodyRunes === null ? minutes : `${formatCount(current.bodyRunes)} 字 · ${minutes}`;
});

/* 右边那句说的是「按下保存会发生什么」，所以它跟着选中的那个走，而且要看库里
   存着的那个是什么——同一句话在「新写一篇」和「改一篇已经发出去的」里不通用。 */
const statusHint = computed(() => {
  const before = record.value?.status ?? null;
  if (form.status === 'published') {
    return before === 'published' ? '保存后改动立刻在外面可见。' : '保存后会出现在文章列表里，谁都能打开。';
  }
  return before === 'published' ? '保存后会从文章列表撤下来，公开地址同时失效。' : '保存后只有你看得到。';
});

function authRedirect(cause) {
  if (cause?.code === codes.unauthorized) {
    clear();
    router.replace({ name: 'login', query: { next: route.fullPath } });
    return true;
  }
  if (cause?.code === codes.forbidden) {
    router.replace({ name: 'articles' });
    return true;
  }
  return false;
}

function apply(detail) {
  Object.assign(form, snapshot(detail));
  baseline.value = snapshot(detail);
  record.value = detail;
}

async function load() {
  /* 换一篇就回到原文那一面。保存不走这里——正在校对时按保存，看的还是那一面。 */
  mode.value = 'source';
  if (isNew.value) {
    state.value = 'ready';
    return;
  }
  state.value = 'loading';
  failing.value = null;
  try {
    const detail = await getAdminArticle(String(route.params.id));
    apply(detail);
    state.value = 'ready';
    setMetadata({ title: `${detail.title} · 写作 · Anxin`, path: route.fullPath });
  } catch (cause) {
    if (authRedirect(cause)) return;
    if (cause?.code === codes.notFound) {
      state.value = 'missing';
      return;
    }
    failing.value = describeFailure(cause);
    state.value = 'failed';
  }
}

/* 新写的一篇保存成功后会走到 /admin/articles/:id。这一跳里组件是同一个实例，
   不会重新挂载，所以自己认一下地址换了没有；那一篇刚从响应里拿到，不必再问一次。 */
watch(() => route.params.id, id => {
  if (isNew.value || record.value?.id === id) return;
  load();
});

/* 分类和文章各问各的：分类读不到不该把整页拖住，这一页本来也还有别的字段要填。 */
async function loadCategories() {
  catsState.value = 'loading';
  try {
    const result = await listAdminCategories();
    cats.value = result.items;
    catsState.value = 'ready';
    /* 「第一个是写新文章时的默认分类」。只在还没选过的时候替它选上，而且基线也要
       跟着走——否则一进来就是「有改动没保存」，而人一个字都还没敲。 */
    if (!form.category && cats.value.length) {
      form.category = cats.value[0].id;
      baseline.value = { ...baseline.value, category: form.category };
    }
  } catch (cause) {
    if (authRedirect(cause)) return;
    cats.value = [];
    catsState.value = 'failed';
  }
}

onMounted(() => {
  if (isNew.value) setMetadata({ title: '写新的一篇 · 写作 · Anxin', path: route.fullPath });
  load();
  loadCategories();
});

function serverFieldError(name) {
  const failure = failing.value;
  if (!failure || failure.field !== name) return '';
  return failure.code === codes.invalidArgument || failure.code === codes.conflict ? failure.message : '';
}

const fieldError = name => local[name] || serverFieldError(name);

const failureBanner = computed(() => {
  const failure = failing.value;
  if (!failure) return null;
  if (failure.code === codes.conflict) return { title: '没有保存', text: '地址里的名字被占用了，其余字段没有改动。' };
  if (failure.code === codes.invalidArgument) return { title: '没有保存', text: '有几个字段没填对，改完再保存一次。' };
  return { title: '没有保存', text: commonText(failure) };
});

/* 成功提示里要说清「这一次保存让外面发生了什么」——已发布的改动立刻可见，草稿
   的仍然只有你看得到。同一句话在两处不能通用，所以看的是保存前那个状态。 */
function metricsChange(before, after) {
  if (after.bodyRunes === null) return '';
  if (before?.bodyRunes == null) return `字数 ${formatCount(after.bodyRunes)}，约 ${after.readingMinutes} 分钟。`;
  const runes = after.bodyRunes === before.bodyRunes
    ? `字数还是 ${formatCount(after.bodyRunes)}`
    : `字数从 ${formatCount(before.bodyRunes)} 更新到 ${formatCount(after.bodyRunes)}`;
  const minutes = after.readingMinutes === before.readingMinutes
    ? `阅读时长还是 ${after.readingMinutes} 分钟`
    : `阅读时长从 ${before.readingMinutes} 分钟变成 ${after.readingMinutes} 分钟`;
  return `${runes}，${minutes}。`;
}

const savedText = computed(() => {
  const entry = saved.value;
  if (!entry) return '';
  const { before, after } = entry;
  const head = after.status === 'published'
    ? (before?.status === 'published' ? '这一篇仍然是已发布，改动现在外面就看得见。' : '这一篇已经发布，改动现在外面就看得见。')
    : (before?.status === 'published' ? '这一篇已经撤下来，公开地址同时失效。' : '这一篇还是草稿，只有你看得到。');
  return head + metricsChange(before, after);
});

async function save() {
  failing.value = null;
  saved.value = null;

  const problems = articleProblems(form);
  for (const key of Object.keys(local)) local[key] = problems[key];
  if (Object.values(local).some(Boolean)) return;

  busy.value = true;
  try {
    const before = record.value;
    const result = isNew.value ? await createArticle({ ...form }) : await updateArticle(before.id, { ...form });
    apply(result);
    /* 保存成功之后拦下的那一次就不算数了：人本来就还在这一页。 */
    release();
    saved.value = { before, after: result };
    if (isNew.value) await router.replace({ name: 'admin-article', params: { id: result.id } });
  } catch (cause) {
    if (authRedirect(cause)) return;
    const failure = describeFailure(cause);
    /* describeFailure 只替 INVALID_ARGUMENT 留着服务端那句话。撞名也一样是「关于某
       一格的具体一句话」，同样留着——它就是要显示在那一格底下的。 */
    failing.value = failure.code === codes.conflict ? { ...failure, message: cause.message } : failure;
  } finally {
    busy.value = false;
  }
}

/* 这一页不留删除确认：回到列表页去问，问法就是那一页本来的问法。路上会经过
   路由守卫——还有改动没保存的话，它先拦下来。 */
function deleteHere() {
  router.push({ name: 'admin-articles', query: { confirm: record.value.id } });
}

/* Tab 落在正文框里该是缩进，不是跳到下一格。出路那条路（Esc 再 Tab）归 tabIntent
   管，这里只管把它的答复落下来。 */
const escaped = ref(false);

function onBodyKeydown(event) {
  const intent = tabIntent(event, escaped.value);
  escaped.value = intent.armed;
  if (!intent.indent) return;

  event.preventDefault();
  const area = bodyArea.value;
  if (!area) return;
  /* 这一格的文字走的是浏览器那套编辑命令：值改了，可它不会冒出 input 事件，所以
     得把结果抄回表单里，否则存下去的仍是旧的那一份。 */
  form.body = event.shiftKey ? outdentLines(area) : indentLines(area);
}

async function pickImage(event) {
  const file = event.target.files?.[0];
  /* 选同一个文件两次也要能再触发一次 change。 */
  event.target.value = '';
  if (!file) return;

  uploading.value = true;
  uploadError.value = '';
  uploaded.value = '';
  try {
    const object = await uploadImage(file);
    const alt = altFromFileName(file.name);
    form.body = insertAtCursor(bodyArea.value, imageMarkdown(object.url, alt));
    uploaded.value = imageMarkdown(shortenURL(object.url), alt);
  } catch (cause) {
    if (authRedirect(cause)) return;
    uploadError.value = uploadText(describeFailure(cause));
  } finally {
    uploading.value = false;
  }
}
</script>

<template>
  <section class="piece row ruled">
    <div class="facts">
      <p v-if="record">
        <span class="key">地址</span><br>{{ record.id }}
      </p>
      <p v-else><span class="key">地址</span><br>保存之后才有</p>
      <p v-if="record">
        写于 <time class="date" :datetime="record.createdAt">{{ formatMonthDay(record.createdAt) }}</time><br>
        改于 <time class="date" :datetime="record.updatedAt">{{ formatMonthDay(record.updatedAt) }}</time>
      </p>
      <p v-if="metrics">{{ metrics }}</p>
    </div>
    <div class="piece-main">
      <h1 class="piece-title">{{ heading }}</h1>
      <p v-if="record" class="piece-summary">
        <template v-if="record.status === 'published'">公开地址 <code>{{ publicAddress }}</code></template>
        <template v-else>还没发布过，对外没有地址。</template>
      </p>

      <NoticeBanner v-if="saved" tone="done" title="已保存">
        {{ savedText }}
        <template v-if="saved.after.status === 'published'" #actions>
          <a class="link-quiet" :href="publicAddress" target="_blank" rel="noopener noreferrer">
            去公开页看一眼<span class="sr-only">（新窗口）</span>
          </a>
        </template>
      </NoticeBanner>

      <NoticeBanner v-if="state === 'failed'" title="这一篇没读出来">
        {{ commonText(failing || {}) }}库里那一篇没有动过，刷新一下这一页再试。
      </NoticeBanner>
      <NoticeBanner v-else-if="state === 'missing'" title="文章不存在">
        这一篇可能已经被删掉了。回到列表看看还有哪些。
      </NoticeBanner>
    </div>
  </section>

  <!-- 表单里回车就交出去；标签框那一下自己拦掉了，不会走到这里。 -->
  <form v-if="state === 'ready'" class="composer row ruled" novalidate @submit.prevent="save">
    <p class="facts">状态决定它现在外面看不看得到</p>
    <div class="composer-form">
      <NoticeBanner v-if="failureBanner" :title="failureBanner.title">{{ failureBanner.text }}</NoticeBanner>

      <StatusChoice id="article-status" v-model="form.status" :hint="statusHint" :disabled="busy" />

      <FormField
        id="article-title" v-model="form.title" label="标题" control-class="input-title"
        placeholder="一句话说清这篇在讲什么" :error="fieldError('title')" :disabled="busy"
      />

      <FormTextarea
        id="article-summary" v-model="form.summary" label="摘要" rows="3"
        placeholder="列表页上跟在标题后面的那两三行"
        hint="必填。写清楚这篇解决什么问题，读者靠它决定点不点进来。"
        :error="fieldError('summary')" :disabled="busy"
      />

      <div class="field-pair">
        <FormField
          id="article-slug" v-model="form.slug" label="地址里的名字" placeholder="go-api-first-step"
          :hint="slugHint" :error="fieldError('slug')" keep-hint :disabled="busy"
        />
        <FormSelect
          v-if="!categoryProblem" id="article-category" v-model="form.category" label="分类"
          :options="categoryOptions" :disabled="busy || catsState === 'loading'" :hint="catsState === 'loading' ? '正在读取分类…' : ''"
        />
        <!-- 说不了话的时候这一格让位给一句话和一条去路，而不是一个空的下拉。 -->
        <div v-else class="field">
          <span class="field-label">分类</span>
          <p class="field-error">{{ categoryProblem }}</p>
          <p class="field-hint">
            <button v-if="retryCategories" class="link-quiet" type="button" @click="loadCategories">重试</button>
            <router-link v-else class="link-quiet" :to="{ name: 'admin-categories' }">去建一个分类</router-link>
          </p>
        </div>
      </div>

      <TagInput
        id="article-tags" v-model="form.tags" label="标签"
        :max="articleLimits.tags" :max-length="articleLimits.tagRunes" :disabled="busy"
      />
    </div>
  </form>

  <section v-if="state === 'ready'" class="md">
    <!-- 两个 radio 加 label，和「草稿／已发布」同一个来路：键盘、焦点圈、方向键都由
         浏览器给。左边那个词写现在在哪一面，右边那枚写点下去会去哪一面。 -->
    <div class="md-head">
      <span class="md-label">{{ mode === 'source' ? 'Markdown 原文' : '预览' }}</span>
      <div class="md-switch" role="radiogroup" aria-label="正文显示方式">
        <input id="article-body-source" v-model="mode" class="sr-only" type="radio" name="article-body-mode" value="source">
        <input id="article-body-preview" v-model="mode" class="sr-only" type="radio" name="article-body-mode" value="preview">
        <label
          class="link-quiet"
          :for="mode === 'source' ? 'article-body-preview' : 'article-body-source'"
        >{{ mode === 'source' ? '看预览' : '看原文' }}</label>
      </div>
    </div>

    <!-- 换面用 v-show 不用 v-if：切回来时框里的字、光标、滚动位置都还在（只有焦点会丢，
         那是该丢的）。 -->
    <div v-show="mode === 'source'" class="md-pane" data-pane="source">
      <textarea
        id="article-body" ref="bodyArea" class="md-source" aria-label="Markdown 原文" spellcheck="false"
        aria-describedby="article-body-keys" :value="form.body" :disabled="busy"
        @input="form.body = $event.target.value" @keydown="onBodyKeydown" @blur="escaped = false"
      ></textarea>
      <p id="article-body-keys" class="field-hint">Tab 缩进，Shift+Tab 退回一格。要离开这一格，先按 Esc，再按 Tab。</p>
      <div class="md-tools">
        <label class="pager-btn upload-btn" for="article-file">{{ uploading ? '正在上传…' : '插入图片' }}</label>
        <input
          id="article-file" class="sr-only" type="file" accept="image/*"
          :disabled="uploading || busy" @change="pickImage"
        >
        <p class="field-hint">JPEG / PNG / GIF / WebP，单张不超过 4 MB。传完插在光标处。</p>
      </div>
      <p v-if="uploadError" class="field-error">{{ uploadError }}</p>
      <p v-else-if="uploaded" class="field-done">已插入 {{ uploaded }}</p>
    </div>

    <div v-show="mode === 'preview'" class="md-pane" data-pane="preview">
      <div class="prose" v-html="preview"></div>
    </div>
  </section>

  <div v-if="state === 'ready'" class="save row ruled" :data-dirty="String(dirty)" :data-leaving="String(blocked)">
    <div class="facts">
      <p v-if="stored" class="save-state-clean">改动都保存了</p>
      <p v-else class="save-state-clean">还没有保存过</p>
      <p class="save-state-dirty"><span class="mark">有改动没保存</span></p>
      <p v-if="stored" class="save-note-clean">删掉就没有了。</p>
      <p v-else class="save-note-clean">关掉这一页就没了。</p>
      <p class="save-note-dirty">离开这一页就没了。</p>
      <p class="save-note-leaving">要去别的页，改动还没保存。</p>
    </div>
    <div class="main save-actions">
      <!-- 没有分类可选时保存是灰的。这不是前端自己加的规矩，是服务端本来就存不下去
           （文章表到分类表之间有外键）——与其让人填完一整篇再吃一个 400，不如现在停住。 -->
      <button class="primary" type="button" :disabled="busy || !form.category" @click="save">{{ busy ? '保存中…' : '保存' }}</button>
      <button class="link-quiet act-abandon" type="button" @click="discard">放弃改动</button>
      <template v-if="stored">
        <a
          v-if="record.status === 'published'" class="link-quiet act-away"
          :href="publicAddress" target="_blank" rel="noopener noreferrer"
        >查看公开页<span class="sr-only">（新窗口）</span></a>
        <button class="act-danger act-away" type="button" @click="deleteHere">删除这一篇</button>
      </template>
      <router-link v-else class="link-quiet act-away" :to="{ name: 'admin-articles' }">放弃这一篇</router-link>
    </div>
  </div>
</template>
