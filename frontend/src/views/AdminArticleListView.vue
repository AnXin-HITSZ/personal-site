<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { deleteArticle, listAdminArticles } from '../api/admin-articles.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText } from '../copy.js';
import { clear } from '../session.js';
import { formatMonthDay } from '../format.js';
import { setMetadata } from '../metadata.js';

const route = useRoute();
const router = useRouter();

const PAGE_SIZE = 10;
const statuses = [
  { value: 'all', label: '全部' },
  { value: 'draft', label: '草稿' },
  { value: 'published', label: '已发布' },
];

const query = reactive({ page: 1, pageSize: PAGE_SIZE, status: 'all', q: '' });
const search = ref('');
const items = ref([]);
const pagination = ref({ total: 0, totalPages: 0 });
const drafts = ref(0);
const state = ref('loading');
const failing = ref(null);
const confirming = ref('');
const deleting = ref(false);
const deleteError = ref('');
let controller;

/* 一列筛不出东西的按钮只是占位，所以整库空着的时候不出筛选器和翻页。 */
const empty = computed(() => state.value === 'ready' && pagination.value.total === 0 &&
  drafts.value === 0 && !query.q && query.status === 'all');
const filteredEmpty = computed(() => state.value === 'ready' && pagination.value.total === 0 && !empty.value);

const facts = computed(() => {
  if (state.value === 'loading') return { count: '', note: '' };
  if (state.value === 'failed') return { count: '没有读到', note: '' };
  if (empty.value) return { count: '0 篇', note: '' };
  const note = query.q ? `搜索“${query.q}”`
    : query.status === 'draft' ? '都是草稿'
    : query.status === 'published' ? '都是已发布'
    : drafts.value ? `其中 ${drafts.value} 篇还是草稿` : '没有草稿';
  return { count: `${pagination.value.total} 篇`, note };
});

/* 会话在某一刻被服务端作废、或者这个账号根本不是作者时，这一页都不该留着——
   它存不存在本来也不该让人知道。返回 true 表示这一次失败已经交代过了。 */
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

/* 同一时刻只留最后一次请求：翻页、筛选点得快时，先发的那次即使晚回来也会被丢掉
   （它带着的是上一组条件的数据）。下面每处 await 之后的 aborted 检查就是为这个。 */
async function load() {
  controller?.abort();
  const current = new AbortController();
  controller = current;
  state.value = 'loading';
  deleteError.value = '';
  failing.value = null;
  try {
    /* 草稿数是整库的事实，和这一页筛的是什么无关，所以另问一次：列表不读正文，
       光看这一页也数不出来。 */
    const [list, draftList] = await Promise.all([
      listAdminArticles({ ...query }, { signal: current.signal }),
      listAdminArticles({ page: 1, pageSize: 1, status: 'draft' }, { signal: current.signal }),
    ]);
    if (current.signal.aborted) return;
    items.value = list.items;
    pagination.value = list.pagination;
    drafts.value = draftList.pagination.total;
    state.value = 'ready';
  } catch (cause) {
    if (current.signal.aborted) return;
    if (authRedirect(cause)) return;
    failing.value = describeFailure(cause);
    state.value = 'failed';
  }
}

/* 换筛选、换搜索词都回第一页：不停在第 5 页，因为新条件下未必有第 5 页。 */
function filterBy(status) {
  query.status = status;
  query.page = 1;
  load();
}

function submitSearch() {
  query.q = search.value.trim();
  query.page = 1;
  load();
}

function clearFilters() {
  search.value = '';
  Object.assign(query, { q: '', status: 'all', page: 1 });
  load();
}

function turnPage(page) {
  query.page = page;
  load();
}

function openConfirm(id) {
  confirming.value = id;
  deleteError.value = '';
}

async function remove(id) {
  deleting.value = true;
  deleteError.value = '';
  try {
    await deleteArticle(id);
    confirming.value = '';
    /* 删掉的是这一页最后一条时，留在这一页会看见一个空页，所以让回一格。 */
    if (items.value.length === 1 && query.page > 1) query.page -= 1;
    await load();
  } catch (cause) {
    if (authRedirect(cause)) return;
    /* 已经不在库里了也算删成了：目的达到了，不必为它报一次错。 */
    if (cause?.code === codes.notFound) {
      confirming.value = '';
      await load();
      return;
    }
    deleteError.value = commonText(describeFailure(cause));
  } finally {
    deleting.value = false;
  }
}

onMounted(() => {
  setMetadata({ title: '写作 · Anxin', path: route.fullPath });
  /* 从编辑台点「删除这一篇」过来的：那一页不留确认框，回到这一页来问，
     问法就是这一页本来的问法。 */
  if (typeof route.query.confirm === 'string') confirming.value = route.query.confirm;
  load();
});
</script>

<template>
  <section class="piece row ruled">
    <div class="facts">
      <p v-if="facts.count"><span class="key">{{ facts.count }}</span><template v-if="facts.note"><br>{{ facts.note }}</template></p>
    </div>
    <div class="piece-main">
      <h1 class="piece-title">写作</h1>
      <p v-if="empty" class="piece-summary">还没有文章。写一篇，存成草稿也行——草稿只有你看得到。</p>
      <p v-else-if="state === 'ready'" class="piece-summary">草稿只有你看得到。改成「已发布」并保存，那一篇才会出现在文章列表里。</p>

      <div v-if="empty" class="form-actions">
        <router-link class="primary" :to="{ name: 'admin-article-new' }">写新的一篇</router-link>
      </div>
      <div v-else-if="state === 'failed'" class="banner" style="margin-top: 1.5rem">
        <h3>文章列表没读出来</h3>
        <p>{{ commonText(failing || {}) }}列表没读到不代表文章没了，重试一次即可。</p>
        <div class="banner-actions"><button class="pager-btn" type="button" @click="load">重新加载</button></div>
      </div>
    </div>
  </section>

  <div v-if="state !== 'failed' && !empty" class="archive-tools row ruled">
    <p class="facts">按最近改动排</p>
    <div class="archive-tools-main">
      <div class="tools">
        <div class="tools-find">
          <div class="filters" role="group" aria-label="按状态筛选">
            <button
              v-for="status in statuses" :key="status.value" class="filter" type="button"
              :aria-pressed="query.status === status.value" @click="filterBy(status.value)"
            >{{ status.label }}</button>
          </div>
          <form class="search" role="search" @submit.prevent="submitSearch">
            <label class="sr-only" for="admin-search">搜索文章标题和摘要</label>
            <input id="admin-search" v-model="search" type="search" placeholder="搜标题和摘要" maxlength="100">
            <button type="submit">搜索</button>
          </form>
        </div>
        <div class="tools-acts">
          <!-- 编辑分类去的是这一页要用的东西（每一篇都归在其中一个下面），但它不是
               这一页的主操作，所以走安静的那一枚，印色留给「写新的一篇」。 -->
          <router-link class="link-quiet" :to="{ name: 'admin-categories' }">编辑分类</router-link>
          <router-link class="primary" :to="{ name: 'admin-article-new' }">写新的一篇</router-link>
        </div>
      </div>
    </div>
  </div>

  <div v-if="state !== 'failed' && !empty" class="entries entries-ledger" :aria-busy="state === 'loading'">
    <template v-if="state === 'loading'">
      <div v-for="n in 3" :key="n" class="entry row ruled" aria-hidden="true">
        <div class="facts"><span class="sk sk-fact"></span><span class="sk sk-fact sk-fact-sm"></span></div>
        <div class="main"><span class="sk sk-title"></span><span class="sk sk-line"></span><span class="sk sk-line sk-line-short"></span></div>
      </div>
    </template>

    <div v-else-if="filteredEmpty" class="entry row ruled">
      <div class="main notice">
        <h3>没有符合条件的文章</h3>
        <p>换个状态，或者清掉搜索词。</p>
        <button class="pager-btn" type="button" @click="clearFilters">查看全部文章</button>
      </div>
    </div>

    <template v-else>
      <article v-for="article in items" :key="article.id" class="entry row ruled">
        <div class="facts">
          <p v-if="article.status === 'draft'" class="mark">草稿</p>
          <p v-else>已发布</p>
          <p>改于 <time class="date" :datetime="article.updatedAt">{{ formatMonthDay(article.updatedAt) }}</time></p>
          <p>约 {{ article.readingMinutes }} 分钟</p>
        </div>
        <div class="main">
          <h3 class="entry-title">
            <router-link :to="{ name: 'admin-article', params: { id: article.id } }">{{ article.title }}</router-link>
          </h3>
          <p class="entry-summary">{{ article.summary }}</p>
          <p v-if="article.tags.length" class="entry-tags">{{ article.tags.join(' / ') }}</p>

          <!-- 删除就地展开，替掉这一行的动作：全站没有弹窗，也不该为一处破例。 -->
          <div v-if="confirming === article.id" class="confirm">
            <p><strong>删除《{{ article.title }}》？</strong></p>
            <p v-if="article.status === 'published'">
              这一篇已经发布，公开地址 <code>/articles/{{ article.id }}/{{ article.slug }}</code> 会立刻失效。
              没有回收站，删掉就找不回来了。
            </p>
            <p v-else>这一篇还没发布过，对外没有链接，删掉不影响任何人。没有回收站，删掉就找不回来了。</p>
            <p v-if="deleteError" class="field-error">{{ deleteError }}</p>
            <div class="confirm-actions">
              <button class="btn-danger" type="button" :disabled="deleting" @click="remove(article.id)">
                {{ deleting ? '正在删除…' : '删除这一篇' }}
              </button>
              <button class="pager-btn" type="button" :disabled="deleting" @click="confirming = ''">取消</button>
            </div>
          </div>

          <p v-else class="entry-acts">
            <router-link class="act" :to="{ name: 'admin-article', params: { id: article.id } }">
              {{ article.status === 'draft' ? '继续写' : '编辑' }}
            </router-link>
            <a
              v-if="article.status === 'published'" class="act"
              :href="`/articles/${article.id}/${article.slug}`" target="_blank" rel="noopener noreferrer"
            >查看公开页<span class="sr-only">（新窗口）</span></a>
            <button class="act-danger" type="button" @click="openConfirm(article.id)">删除</button>
          </p>
        </div>
      </article>
    </template>
  </div>

  <nav v-if="state === 'ready' && !empty && pagination.totalPages > 1" class="pager row ruled" aria-label="翻页">
    <div class="pager-nav">
      <button class="pager-btn pager-prev" type="button" :disabled="query.page <= 1" @click="turnPage(query.page - 1)">上一页</button>
      <span class="pager-count">第 {{ query.page }} / {{ pagination.totalPages }} 页 · 共 {{ pagination.total }} 篇</span>
      <button class="pager-btn pager-next" type="button" :disabled="query.page >= pagination.totalPages" @click="turnPage(query.page + 1)">下一页</button>
    </div>
  </nav>
</template>
