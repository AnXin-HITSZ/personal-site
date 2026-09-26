<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue';
import { config } from '../config.js';
import { categories, listArticles } from '../api/articles.js';
import ArticleEntry from './ArticleEntry.vue';

const query = reactive({ page: 1, pageSize: 6, q: '', category: 'all' });
const search = ref('');
const items = ref([]);
const pagination = ref({ total: 0, totalPages: 0 });
const loading = ref(true);
const error = ref('');
const heading = ref(null);
let controller;
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
function filter(category) { query.category = category; query.page = 1; load(); }
function submitSearch() { query.q = search.value.trim(); query.page = 1; load(); }
async function focusHeading() { await nextTick(); heading.value?.focus({ preventScroll: true }); }
function reset() { search.value = ''; Object.assign(query, { q: '', category: 'all', page: 1 }); load(); focusHeading(); }
function turnPage(page) { query.page = page; load(); focusHeading(); heading.value?.scrollIntoView({ block: 'start' }); }
function retry() { load(); focusHeading(); }
onMounted(load);
onUnmounted(() => controller?.abort());
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
        <div class="filters" role="group" aria-label="文章分类">
          <button v-for="(label, key) in categories" :key="key" class="filter" type="button" :aria-pressed="query.category === key" @click="filter(key)">{{ label }}</button>
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
          <div class="entry-facts"><span class="sk sk-fact"></span><span class="sk sk-fact sk-fact-sm"></span></div>
          <div class="entry-main"><span class="sk sk-title"></span><span class="sk sk-line"></span><span class="sk sk-line sk-line-short"></span></div>
        </div>
      </template>
      <div v-else-if="error" class="entry row ruled">
        <div class="entry-main notice">
          <h3>暂时无法读取文章</h3>
          <p>{{ error }}</p>
          <button class="pager-btn" @click="retry">重新加载</button>
        </div>
      </div>
      <div v-else-if="!items.length" class="entry row ruled">
        <div class="entry-main notice">
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
