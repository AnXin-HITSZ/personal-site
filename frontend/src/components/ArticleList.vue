<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue';
import { config } from '../config.js';
import { listArticles } from '../api/articles.js';
import { ALL, listCategories } from '../api/categories.js';
import ArticleEntry from './ArticleEntry.vue';

const query = reactive({ page: 1, pageSize: 6, q: '', category: 'all' });
const search = ref('');
const items = ref([]);
/* 筛选器是这一页上唯一要用到分类表的地方，而文章不靠它——所以这两个请求各走各的。
   分类没读到时整排按钮不出现，搜索框照旧在，读者还是能找到文章。 */
const cats = ref([]);
const pagination = ref({ total: 0, totalPages: 0 });
const loading = ref(true);
const error = ref('');
const heading = ref(null);
let controller;
let catsController;
const status = computed(() => loading.value ? '正在整理文章…' : error.value ? '加载失败' : `${pagination.value.total} 篇文章${query.q ? ` · 搜索“${query.q}”` : ''}`);
/* 「最新」标的是全站最新的一篇，不是「本页第一条」：列表未被筛选且停在第一页时，
   首条即最新（仓储层按 published_at desc, id asc 排序），其余情况无从判断，就不标。 */
const latestId = computed(() => !query.q && query.category === 'all' && query.page === 1 ? items.value[0]?.id : undefined);

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
function filter(category) { query.category = category; query.page = 1; load(); }
function submitSearch() { query.q = search.value.trim(); query.page = 1; load(); }
async function focusHeading() { await nextTick(); heading.value?.focus({ preventScroll: true }); }
function reset() { search.value = ''; Object.assign(query, { q: '', category: 'all', page: 1 }); load(); focusHeading(); }
function turnPage(page) { query.page = page; load(); focusHeading(); heading.value?.scrollIntoView({ block: 'start' }); }
function retry() { load(); focusHeading(); }
onMounted(() => { load(); loadCategories(); });
onUnmounted(() => { controller?.abort(); catsController?.abort(); });
</script>

<template>
  <section id="articles" class="archive" aria-labelledby="archive-title">
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
          <input id="search" v-model="search" type="search" placeholder="搜索标题或摘要" maxlength="100">
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
      <div v-else-if="!items.length" class="entry row ruled">
        <div class="main notice">
          <h3>还没有找到这样的文章</h3>
          <p>试试其他关键词，或回到全部文章。</p>
          <button class="pager-btn" @click="reset">查看全部文章</button>
        </div>
      </div>
      <template v-else>
        <ArticleEntry v-for="article in items" :key="article.id" :article="article" :latest="article.id === latestId" />
      </template>
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
