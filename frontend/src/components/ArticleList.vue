<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { config } from '../config.js';
import { listArticles } from '../api/articles.js';
import { ALL, listCategories } from '../api/categories.js';
import { readListQuery, writeListQuery } from '../list-query.js';
import ArticleEntry from './ArticleEntry.vue';

const route = useRoute();
const router = useRouter();

/* query 是「已经生效的那份条件」——它的家是地址栏（见 list-query.js 和下面那个
   watcher）；search 是输入框里正在敲的那串字，搜索要按回车或点按钮才生效，所以
   两者不能是同一个值。 */
const query = reactive({ page: 1, pageSize: 6, q: '', category: ALL });
const search = ref('');
const items = ref([]);
/* 筛选器是这一页上唯一要用到分类表的地方，而文章不靠它——所以这两个请求各走各的。
   分类没读到时整排按钮不出现，搜索框照旧在，读者还是能找到文章。 */
const cats = ref([]);
const pagination = ref({ total: 0, totalPages: 0 });
const loading = ref(true);
const error = ref('');
const heading = ref(null);
const section = ref(null);
let controller;
let catsController;
/* 带着 #articles 进来的那一次导航，等第一次取数落地后替它滚到列表头（见 load 尾注）。 */
let hashPending = route.hash === '#articles';
const status = computed(() => loading.value ? '正在整理文章…' : error.value ? '加载失败' : `${pagination.value.total} 篇文章${query.q ? ` · 搜索“${query.q}”` : ''}`);
/* 「最新」标的是全站最新的一篇，不是「本页第一条」：列表未被筛选且停在第一页时，
   首条即最新（仓储层按 published_at desc, id asc 排序），其余情况无从判断，就不标。 */
const latestId = computed(() => !query.q && query.category === ALL && query.page === 1 ? items.value[0]?.id : undefined);

/* 地址一变就取数：进来那一次、每次筛、每次翻页都从这儿走——加载只有一条路，
   「地址是筛选的家」才不是一句空话。离开这一页时地址也会变（点进一篇文章），
   那一次不必再拉一遍列表，所以先认一下名字。 */
watch(() => route.query, () => {
  if (route.name !== 'articles') return;
  const next = readListQuery(route.query);
  /* 地址里的关键词真的变了才动输入框：点个分类筛一下，不该把读者正在敲的字抹掉。 */
  if (next.q !== query.q) search.value = next.q;
  Object.assign(query, next);
  load();
}, { immediate: true });

/* 每次取数先掐掉上一次：连点筛选时两个请求会在天上赛跑，先发的未必先回，晚到的旧结果
   会把新的盖掉。current 是这一次的控制器——回调里靠它认「我还是最新的那一次吗」。 */
async function load() {
  controller?.abort();
  const current = new AbortController();
  controller = current;
  loading.value = true;
  error.value = '';
  try {
    const result = await listArticles({ ...query }, { signal: current.signal });
    if (current.signal.aborted) return;
    items.value = result.items;
    pagination.value = result.pagination;
  } catch (cause) {
    if (current.signal.aborted) return;
    error.value = cause.name === 'TimeoutError' ? '请求超时，请稍后重试。'
      : cause instanceof TypeError ? '连接不上服务器，请检查网络后重试。'
      : cause.message;
  } finally {
    if (!current.signal.aborted) loading.value = false;
  }
  /* 「全部 N 篇」从分类页过来时地址上带着 #articles。这一步等取数落地再做：挂载
     那一刻页面上还是三行骨架，整页比列表撑起来之后矮，滚动会被底部截住，文档变高
     之后也不会再对一次（无头实测落点会一直偏 8px）。也不放进 router 的
     scrollBehavior——翻页过渡还没走完时新页还没挂上，那边找不到这个锚点。 */
  if (hashPending && !current.signal.aborted) {
    hashPending = false;
    await nextTick();
    section.value?.scrollIntoView();
  }
}
/* 分类读不出来的话这一排就不出现——不为一个筛选项在页面上留一句错，读者本来也
   没在等它。文章那边有它自己的错误分支，两件事分开说。 */
async function loadCategories() {
  catsController?.abort();
  const current = new AbortController();
  catsController = current;
  try {
    const result = await listCategories({ signal: current.signal });
    if (current.signal.aborted) return;
    cats.value = result.items;
  } catch {
    if (current.signal.aborted) return;
    cats.value = [];
  }
}
/* 所有操作都是同一件事：把新条件写回地址，取数留给上面那个 watcher——「点筛选」和
   「按后退键退回上一次筛选」于是走的是同一条路。 */
function go(change) {
  router.push({ query: writeListQuery({ ...query, ...change }) });
}
function filter(category) { go({ category, page: 1 }); focusHeading(); }
function submitSearch() { go({ q: search.value.trim(), page: 1 }); focusHeading(); }
function reset() { search.value = ''; go({ q: '', category: ALL, page: 1 }); focusHeading(); }
/* 「返回」：撤掉这次搜索，分类与页码回默认——分类留在哪一个不动，因为它撤的不是筛选。
   点它的两处（列表末尾、空结果那一格）都会让它自己消失，所以焦点要送回标题，不能悬在空处。 */
function clearSearch() { search.value = ''; go({ q: '', page: 1 }); focusHeading(); }
/* 框里那串字清空了，已经生效的词跟着撤：✕、Esc、退格删空是同一个手势的三种按法，三种都
   发 input 事件（2026-10-16 用真鼠标真键盘点过：✕ 与 Esc 另发 search，退格只发 input）。
   条件里那个 query.q 是「本来就有词」——本来没有就别动，否则每一次清空都往历史里推一条
   空记录。不调 focusHeading：焦点本来就在框里，别把人赶走。 */
function onSearchInput() { if (search.value.trim() === '' && query.q) go({ q: '', page: 1 }); }
/* 换过页或筛过之后把焦点送回标题：读屏会念出新的一段，键盘用户的下一次 Tab 也从这里
   重新走。preventScroll 是因为滚动位置由调用方另外决定（见 turnPage）。 */
async function focusHeading() { await nextTick(); heading.value?.focus({ preventScroll: true }); }
// 换页之后连视口一起带回列表顶端，否则读者会停在页脚那条分页栏旁边。
function turnPage(page) { go({ page }); focusHeading(); heading.value?.scrollIntoView({ block: 'start' }); }
function retry() { load(); focusHeading(); }
onMounted(() => { loadCategories(); });
onUnmounted(() => { controller?.abort(); catsController?.abort(); });
</script>

<template>
  <section id="articles" ref="section" class="archive" aria-labelledby="archive-title">
    <div class="archive-head row ruled">
      <p class="archive-count" role="status" aria-live="polite">{{ status }}</p>
      <div class="archive-main">
        <h2 id="archive-title" ref="heading" tabindex="-1">文章</h2>
        <span v-if="config.dataSource === 'mock'" class="archive-sample">示例内容</span>
      </div>
    </div>

    <div class="archive-tools row">
      <div class="archive-tools-main">
        <div v-if="cats.length" class="filters" role="group" aria-label="文章分类">
          <button class="filter" type="button" :aria-pressed="query.category === ALL" @click="filter(ALL)">全部</button>
          <button v-for="cat in cats" :key="cat.id" class="filter" type="button" :aria-pressed="query.category === cat.id" @click="filter(cat.id)">{{ cat.name }}</button>
        </div>
        <form class="search" role="search" @submit.prevent="submitSearch">
          <label class="sr-only" for="search">搜索文章标题与摘要</label>
          <input id="search" v-model="search" type="search" placeholder="搜索标题或摘要" maxlength="100" @input="onSearchInput">
          <button type="submit">搜索</button>
        </form>
      </div>
    </div>

    <div class="entries" :aria-busy="loading">
      <template v-if="loading">
        <div v-for="n in 3" :key="n" class="entry row ruled" aria-hidden="true">
          <div class="facts"><span class="sk sk-fact"></span><span class="sk sk-fact sk-fact-sm"></span></div>
          <div class="main"><span class="sk sk-title"></span><span class="sk sk-line"></span><span class="sk sk-line sk-line-short"></span></div>
        </div>
      </template>
      <div v-else-if="error" class="entry row ruled">
        <div class="main notice">
          <h3>暂时无法读取文章</h3>
          <p>{{ error }}</p>
          <button class="pager-btn" @click="retry">重新加载</button>
        </div>
      </div>
      <!-- 空结果这一格自带去路，所以下面那一行不在（两处都摆就成了「上下两层返回」）。
           两枚并排时：一枚撤词、留在同一个分类；一枚连分类一起放开。分类本来就停在
           「全部」时这两枚是同一件事，只留一枚。 -->
      <div v-else-if="!items.length" class="entry row ruled">
        <div class="main notice">
          <h3>还没有找到这样的文章</h3>
          <p>试试其他关键词，或回到全部文章。</p>
          <div class="notice-acts">
            <button v-if="query.q && query.category !== ALL" class="pager-btn" @click="clearSearch">返回</button>
            <button class="pager-btn" @click="reset">查看全部文章</button>
          </div>
        </div>
      </div>
      <template v-else>
        <ArticleEntry v-for="article in items" :key="article.id" :article="article" :latest="article.id === latestId" />
      </template>
    </div>

    <!-- 搜过的词还在生效、这一页又有条目时才在：没有搜索（或这一页空着）时列表末尾什么
         都不加。用 items.length 不用 pagination.total——页码超出时接口不夹取，这时这一页
         是空的、上面那格正摆着它自己的两枚，两处都会站出来。 -->
    <div v-if="query.q && !loading && !error && items.length" class="archive-foot row ruled">
      <div class="archive-foot-main">
        <button class="pager-btn" type="button" @click="clearSearch">返回</button>
      </div>
    </div>

    <nav class="pager row ruled" aria-label="文章分页">
      <div v-if="!loading && !error && pagination.totalPages > 1" class="pager-nav">
        <button class="pager-btn" :disabled="query.page <= 1" @click="turnPage(query.page - 1)">上一页</button>
        <span class="pager-count">{{ query.page }} / {{ pagination.totalPages }}</span>
        <button class="pager-btn" :disabled="query.page >= pagination.totalPages" @click="turnPage(query.page + 1)">下一页</button>
      </div>
    </nav>
  </section>
</template>
