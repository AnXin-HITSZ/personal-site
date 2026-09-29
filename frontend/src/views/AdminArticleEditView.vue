<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import MarkdownIt from 'markdown-it';
import {
  articleLimits, articleProblems, categoryOptions, createArticle, getAdminArticle, updateArticle, uploadImage,
} from '../api/admin-articles.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText, uploadText } from '../copy.js';
import { clear } from '../session.js';
import { useUnsavedChanges } from '../dirty.js';
import { altFromFileName, imageMarkdown, insertAtCursor, shortenURL } from '../insert.js';
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
const form = reactive({ slug: '', title: '', summary: '', body: '', category: 'backend', tags: [], status: 'draft' });
const local = reactive({ slug: '', title: '', summary: '', body: '' });
/* record 是库里那一篇，form 是手上这一稿。注文栏只放前者，输入框只放后者。 */
const record = ref(null);
const baseline = ref(snapshot({}));
const state = ref('loading');
const failing = ref(null);
const saved = ref(null);
const busy = ref(false);

const bodyArea = ref(null);
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
    category: source.category ?? 'backend',
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

onMounted(() => {
  if (isNew.value) setMetadata({ title: '写新的一篇 · 写作 · Anxin', path: route.fullPath });
  load();
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
          id="article-category" v-model="form.category" label="分类" :options="categoryOptions" :disabled="busy"
        />
      </div>

      <TagInput
        id="article-tags" v-model="form.tags" label="标签"
        :max="articleLimits.tags" :max-length="articleLimits.tagRunes" :disabled="busy"
      />
    </div>
  </form>

  <section v-if="state === 'ready'" class="md">
    <div class="md-split">
      <div>
        <label class="md-label" for="article-body">Markdown 原文</label>
        <textarea
          id="article-body" ref="bodyArea" class="md-source" spellcheck="false"
          :value="form.body" :disabled="busy" @input="form.body = $event.target.value"
        ></textarea>
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
      <div>
        <span class="md-label">预览</span>
        <div class="prose" v-html="preview"></div>
      </div>
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
      <button class="primary" type="button" :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存' }}</button>
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
