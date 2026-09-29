<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  categoryNameProblem, categoryNameRunes, createCategory, deleteCategory,
  listAdminCategories, maxCategories, renameCategory, reorderCategories,
} from '../api/categories.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText } from '../copy.js';
import { clear } from '../session.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import NoticeBanner from '../components/NoticeBanner.vue';

const route = useRoute();
const router = useRouter();

const items = ref([]);
const state = ref('loading');
const failing = ref(null);

/* 一次只做一件事：busy 是那件事的名字（'', 'order', 'create', `rename:${id}`,
   `delete:${id}`）。做的时候这一页的动作全灰掉——顺序是整份提交的，两下「上移」
   叠在一起就会把中间那一份发上去的顺序冲掉。 */
const busy = ref('');
const actionError = ref('');

const creating = ref(false);
const draftName = ref('');
const draftError = ref('');

const renaming = ref('');
const renameName = ref('');
const renameError = ref('');

const confirming = ref('');
const moveTo = ref('');
const deleteError = ref('');

const totalArticles = computed(() => items.value.reduce((sum, cat) => sum + cat.articleCount, 0));
/* 去处得是别的分类，所以下拉里不含它自己。空分类也行——「挪走」和「去哪儿」是
   两件事，这里只问去哪儿。 */
const destinations = computed(() => items.value.filter(cat => cat.id !== confirming.value));
/* 到上限时那枚按钮就不出现，但得说一句为什么——一列按钮少了一个，看着像坏了。
   上限写死在这儿是为了让读者那一行不至于长到没法看。 */
const toolsNote = computed(() => {
  if (items.value.length >= maxCategories) return `已经到上限了：分类最多 ${maxCategories} 个。`;
  return creating.value
    ? '新建表单落在它将成为的那一行上：新分类排在最后。'
    : '每一篇都归在其中一个下面，没有「未分类」这一档。';
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

async function load() {
  state.value = 'loading';
  failing.value = null;
  actionError.value = '';
  try {
    const result = await listAdminCategories();
    items.value = result.items;
    state.value = 'ready';
    /* 一个都没有的时候没有列表可以退回去，表单就直接摆在那儿——没有「取消」，
       也没有那枚点开它的按钮。 */
    if (!items.value.length) {
      creating.value = true;
      draftName.value = '';
      draftError.value = '';
    }
  } catch (cause) {
    if (authRedirect(cause)) return;
    failing.value = describeFailure(cause);
    state.value = 'failed';
  }
}

function resetForms() {
  creating.value = false;
  draftName.value = '';
  draftError.value = '';
  renaming.value = '';
  renameName.value = '';
  renameError.value = '';
  confirming.value = '';
  moveTo.value = '';
  deleteError.value = '';
}

async function move(index, delta) {
  const target = index + delta;
  if (target < 0 || target >= items.value.length) return;

  const current = items.value;
  const ids = current.map(cat => cat.id);
  [ids[index], ids[target]] = [ids[target], ids[index]];

  busy.value = 'order';
  actionError.value = '';
  try {
    await reorderCategories(ids);
    /* 发上去的就是整份新顺序，所以不必再读一次——读一次只会让这一页闪一下。 */
    items.value = ids.map(id => current.find(cat => cat.id === id));
  } catch (cause) {
    if (authRedirect(cause)) return;
    actionError.value = commonText(describeFailure(cause));
    /* 没写成，就把库里那一份读回来：页面上不能留着一次没发生的交换。 */
    await load();
  } finally {
    busy.value = '';
  }
}

async function create() {
  draftError.value = categoryNameProblem(draftName.value);
  if (draftError.value) return;

  busy.value = 'create';
  actionError.value = '';
  try {
    await createCategory(draftName.value);
    resetForms();
    await load();
  } catch (cause) {
    if (authRedirect(cause)) return;
    const failure = describeFailure(cause);
    // 名字本身没填错、只是撞了上限时说的一句话，放在这一页顶上。
    if (failure.code === codes.invalidArgument) {
      draftError.value = failure.message;
    } else {
      draftError.value = commonText(failure);
    }
  } finally {
    busy.value = '';
  }
}

function startCreate() {
  creating.value = true;
  draftName.value = '';
  draftError.value = '';
}

function startRename(item) {
  confirming.value = '';
  deleteError.value = '';
  renaming.value = item.id;
  renameName.value = item.name;
  renameError.value = '';
}

async function rename(item) {
  const name = renameName.value.trim();
  renameError.value = categoryNameProblem(name);
  if (renameError.value) return;

  busy.value = `rename:${item.id}`;
  actionError.value = '';
  try {
    const updated = await renameCategory(item.id, name);
    /* 一次只改一行，其余的行不必跟着重读——顺序没动，计数也没动，动的只有那几个字。 */
    items.value = items.value.map(cat => (cat.id === updated.id ? updated : cat));
    renaming.value = '';
  } catch (cause) {
    if (authRedirect(cause)) return;
    const failure = describeFailure(cause);
    /* 撞名是服务端说的，但这一句要说出撞的是哪个名字。名字是这一次请求里的事实，
       拼进去不会变成第二个判断——判的仍是服务端那一次 409。 */
    if (failure.code === codes.conflict) {
      renameError.value = `已经有一个叫「${name}」的分类了。名字在站上只能有一个。`;
    } else if (failure.code === codes.invalidArgument) {
      renameError.value = failure.message;
    } else if (failure.code === codes.notFound) {
      renameError.value = '';
      actionError.value = '这个分类不在了——可能已经在别处删掉。这一页重新读一次就好。';
      renaming.value = '';
      await load();
    } else {
      renameError.value = commonText(failure);
    }
  } finally {
    busy.value = '';
  }
}

function startConfirm(item) {
  renaming.value = '';
  renameError.value = '';
  confirming.value = item.id;
  moveTo.value = '';
  deleteError.value = '';
}

/* 空分类不问去处：多问一步只是走个形式。有文章的必须选了去处才点得动——
   默认一个等于替人做了他正在犹豫的那个决定。 */
async function remove(item) {
  if (item.articleCount > 0 && !moveTo.value) return;

  busy.value = `delete:${item.id}`;
  deleteError.value = '';
  try {
    await deleteCategory(item.id, item.articleCount > 0 ? moveTo.value : '');
    confirming.value = '';
    await load();
  } catch (cause) {
    if (authRedirect(cause)) return;
    const failure = describeFailure(cause);
    /* 已经不在库里了也算删成了：目的达到了，不必为它报一次错。 */
    if (failure.code === codes.notFound) {
      confirming.value = '';
      await load();
      return;
    }
    deleteError.value = failure.code === codes.conflict
      ? '文章没挪成，分类也就没删——先给它们找个去处。'
      : commonText(failure);
  } finally {
    busy.value = '';
  }
}

onMounted(() => {
  setMetadata({ title: '分类 · 写作 · Anxin', path: route.fullPath });
  load();
});
</script>

<template>
  <section class="piece row ruled">
    <div class="facts">
      <p v-if="state === 'loading'"><span class="sk sk-fact"></span></p>
      <p v-else-if="state === 'failed'">没有读到</p>
      <p v-else-if="!items.length">0 个分类</p>
      <p v-else><span class="key">{{ items.length }} 个分类</span><br>{{ totalArticles }} 篇归在其中</p>
    </div>
    <div class="piece-main">
      <h1 class="piece-title">分类</h1>
      <p v-if="state === 'ready' && !items.length" class="piece-summary">
        还没有分类。文章必须归在其中一个下面，所以先建一个——读者那边的筛选项也是它。
      </p>
      <p v-else-if="state === 'ready'" class="piece-summary">
        读者在文章列表上看见的筛选项就是这几个，先后也照这个顺序。改名字不会动到任何一篇文章的地址——地址里带的是 id。
      </p>

      <div v-if="state === 'failed'" class="banner" style="margin-top: 1.5rem">
        <h3>分类没读出来</h3>
        <p>{{ commonText(failing || {}) }}分类还在，文章也还在——只是这一页没读到，重试一次即可。</p>
        <div class="banner-actions"><button class="pager-btn" type="button" @click="load">重新加载</button></div>
      </div>
    </div>
  </section>

  <div v-if="state === 'ready' && items.length" class="archive-tools row ruled">
    <p class="facts">第一个是写新文章时的默认分类</p>
    <div class="archive-tools-main">
      <div class="tools">
        <p class="facts">{{ toolsNote }}</p>
        <button
          v-if="!creating && items.length < maxCategories" class="primary" type="button"
          :disabled="Boolean(busy)" @click="startCreate"
        >新建一个分类</button>
      </div>
    </div>
  </div>

  <NoticeBanner v-if="actionError" title="这一下没做成">{{ actionError }}</NoticeBanner>

  <div v-if="state !== 'failed'" class="entries entries-ledger" :aria-busy="state === 'loading'">
    <template v-if="state === 'loading'">
      <div v-for="n in 3" :key="n" class="entry row ruled" aria-hidden="true">
        <div class="facts"><span class="sk sk-fact"></span></div>
        <div class="main"><span class="sk sk-title"></span></div>
      </div>
    </template>

    <template v-else>
      <article v-for="(cat, index) in items" :key="cat.id" class="entry row ruled">
        <div class="facts">
          <p>{{ cat.articleCount }} 篇</p>
          <p v-if="cat.draftCount">其中 {{ cat.draftCount }} 篇草稿</p>
        </div>
        <div class="main">
          <template v-if="renaming === cat.id">
            <div class="cat-name-field">
              <FormField
                :id="`cat-rename-${cat.id}`" v-model="renameName" label="名字" placeholder="比如：读书笔记"
                :hint="`改的是它在站上的叫法。文章存的是分类的 id，所以这 ${cat.articleCount} 篇一个字都不动。`"
                :error="renameError" :disabled="Boolean(busy)" keep-hint
              />
            </div>
            <div class="form-actions">
              <button class="primary" type="button" :disabled="Boolean(busy)" @click="rename(cat)">
                {{ busy === `rename:${cat.id}` ? '保存中…' : '保存' }}
              </button>
              <button class="pager-btn" type="button" :disabled="Boolean(busy)" @click="renaming = ''">取消</button>
            </div>
          </template>

          <template v-else-if="confirming === cat.id">
            <h3 class="entry-title">{{ cat.name }}</h3>
            <div class="confirm">
              <p><strong>删除「{{ cat.name }}」？</strong></p>
              <template v-if="cat.articleCount">
                <p>
                  这个分类下还有 {{ cat.articleCount }} 篇文章<template v-if="cat.draftCount">（{{ cat.draftCount }} 篇草稿）</template>。
                  删掉分类之前，得先给它们找个去处。
                </p>
                <div class="cat-name-field field">
                  <label class="field-label" for="cat-move">这 {{ cat.articleCount }} 篇改归到</label>
                  <span class="select">
                    <select id="cat-move" v-model="moveTo" :disabled="Boolean(busy)">
                      <option value="" disabled>选一个分类</option>
                      <option v-for="dest in destinations" :key="dest.id" :value="dest.id">{{ dest.name }}</option>
                    </select>
                  </span>
                </div>
                <p>文章本身一个字都不动，只有分类换成新的那一个。它们的地址里带的是 id，也不会变。</p>
              </template>
              <p v-else>
                这个分类下面还没有文章，删掉不影响任何一篇。读者那边本来也看不见它——没有已发布的文章，它就不在筛选项里。
              </p>
              <p v-if="deleteError" class="field-error">{{ deleteError }}</p>
              <div class="confirm-actions">
                <button
                  class="btn-danger" type="button"
                  :disabled="Boolean(busy) || (cat.articleCount > 0 && !moveTo)" @click="remove(cat)"
                >
                  {{ busy === `delete:${cat.id}` ? '正在删除…'
                    : cat.articleCount ? `挪走这 ${cat.articleCount} 篇，删掉分类` : '删除这个分类' }}
                </button>
                <button class="pager-btn" type="button" :disabled="Boolean(busy)" @click="confirming = ''">取消</button>
              </div>
            </div>
          </template>

          <template v-else>
            <h3 class="entry-title">{{ cat.name }}</h3>
            <p class="entry-acts">
              <button class="act" type="button" :disabled="Boolean(busy) || index === 0" @click="move(index, -1)">上移</button>
              <button class="act" type="button" :disabled="Boolean(busy) || index === items.length - 1" @click="move(index, 1)">下移</button>
              <button class="act" type="button" :disabled="Boolean(busy)" @click="startRename(cat)">改名</button>
              <button class="act-danger" type="button" :disabled="Boolean(busy)" @click="startConfirm(cat)">删除</button>
            </p>
          </template>
        </div>
      </article>

      <article v-if="creating" class="entry row ruled">
        <div class="facts"><p class="mark">新的</p></div>
        <div class="main">
          <div class="cat-name-field">
            <FormField
              id="cat-new-name" v-model="draftName" label="名字" placeholder="比如：读书笔记"
              :hint="`写的就是读者在文章列表上看见的那几个字。最多 ${categoryNameRunes} 个字。`"
              :error="draftError" :disabled="Boolean(busy)"
            />
          </div>
          <div class="form-actions">
            <button class="primary" type="button" :disabled="Boolean(busy)" @click="create">
              {{ busy === 'create' ? '正在新建…' : '加上' }}
            </button>
            <!-- 一个分类都没有的时候没有「取消」——没有列表可以退回去，所以那一屏
                 只有这个表单，见下面那个分支。 -->
            <button
              v-if="items.length" class="pager-btn" type="button"
              :disabled="Boolean(busy)" @click="creating = false"
            >取消</button>
          </div>
        </div>
      </article>
    </template>
  </div>
</template>
