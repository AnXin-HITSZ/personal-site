<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue';
import { config } from '../config.js';
import { categories, listArticles } from '../api/articles.js';
import ArticleCard from './ArticleCard.vue';

const query = reactive({ page: 1, pageSize: 6, q: '', category: 'all' });
const search = ref('');
const items = ref([]);
const pagination = ref({ total: 0, totalPages: 0 });
const loading = ref(true);
const error = ref('');
const heading = ref(null);
let controller;
const status = computed(() => loading.value ? '正在整理文章…' : error.value ? '加载失败' : `${pagination.value.total} 篇文章${query.q ? ` · 搜索“${query.q}”` : ''}`);

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
    error.value = cause.name === 'TimeoutError' ? '请求超时，请稍后重试。' : cause.message;
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
  <section id="articles" class="articles" aria-labelledby="articles-title">
    <div class="section-heading">
      <div><p class="eyebrow">THE NOTEBOOK</p><h2 id="articles-title" ref="heading" tabindex="-1">文章与札记<span v-if="config.dataSource === 'mock'" class="sample-label">示例内容</span></h2></div>
      <span class="section-side">持续学习，保持好奇。</span>
    </div>
    <div class="toolbar">
      <div class="categories" role="group" aria-label="文章分类">
        <button v-for="(label, key) in categories" :key="key" class="category" type="button" :aria-pressed="query.category === key" @click="filter(key)">{{ label }}</button>
      </div>
      <form role="search" @submit.prevent="submitSearch">
        <label class="sr-only" for="search">搜索文章标题与摘要</label>
        <input id="search" v-model="search" type="search" placeholder="搜索文章…" maxlength="100">
        <button type="submit">搜索</button>
      </form>
    </div>
    <div class="result-info" role="status" aria-live="polite">{{ status }}</div>
    <div class="article-list" :aria-busy="loading">
      <template v-if="loading"><div v-for="n in 3" :key="n" class="skeleton" aria-hidden="true"></div></template>
      <div v-else-if="error" class="empty-state"><h3>暂时无法读取文章</h3><p>{{ error }}</p><button class="page-button" @click="retry">重新加载</button></div>
      <div v-else-if="!items.length" class="empty-state"><h3>还没有找到这样的文章</h3><p>试试其他关键词，或回到全部文章。</p><button class="page-button" @click="reset">查看全部文章</button></div>
      <template v-else><ArticleCard v-for="(article, index) in items" :key="article.id" :article="article" :number="(query.page - 1) * query.pageSize + index + 1" /></template>
    </div>
    <nav class="pagination" aria-label="文章分页">
      <template v-if="!loading && !error && pagination.totalPages > 1">
        <button class="page-button" :disabled="query.page <= 1" @click="turnPage(query.page - 1)">← 上一页</button>
        <span class="page-count">{{ query.page }} / {{ pagination.totalPages }}</span>
        <button class="page-button" :disabled="query.page >= pagination.totalPages" @click="turnPage(query.page + 1)">下一页 →</button>
      </template>
    </nav>
  </section>
</template>
