<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { config } from '../config.js';
import { listArticles } from '../api/articles.js';
import { listCategories } from '../api/categories.js';
import { formatDate } from '../format.js';
import { defaultMetadata, setMetadata } from '../metadata.js';

/* 每一类先摆这么多篇，与文章列表每页同数；读完这一截还有剩，就接一行「全部 N 篇」。
   觉得早或晚改这一个数——等真需要按屏高自适应再说。 */
const previewSize = 6;

const groups = ref([]);
const total = ref(0);
const loading = ref(true);
const error = ref('');
let controller;

const status = computed(() => loading.value ? '正在整理分类…' : error.value ? '加载失败' : `${groups.value.length} 个分类 · ${total.value} 篇文章`);

/* 先取分类表（每一条自带篇数和最近时间），再给每一类借现成的文章接口拉一小截：
   「N 篇」不必数前端手里的这几条——数是服务端给的，摆出来的只是最近几篇。
   N 个分类并 N 个请求，分类数今天是个位数。 */
async function load() {
  controller?.abort();
  const current = new AbortController();
  controller = current;
  loading.value = true;
  error.value = '';
  try {
    const { items } = await listCategories({ signal: current.signal });
    const previews = await Promise.all(items.map(cat =>
      listArticles({ category: cat.id, pageSize: previewSize }, { signal: current.signal })));
    if (current.signal.aborted) return;
    groups.value = items.map((cat, index) => ({ ...cat, articles: previews[index].items }));
    total.value = items.reduce((sum, cat) => sum + cat.articleCount, 0);
  } catch (cause) {
    if (current.signal.aborted) return;
    error.value = cause.name === 'TimeoutError' ? '请求超时，请稍后重试。'
      : cause instanceof TypeError ? '连接不上服务器，请检查网络后重试。'
      : cause.message;
  } finally {
    if (!current.signal.aborted) loading.value = false;
  }
}

onMounted(() => {
  load();
  setMetadata({ ...defaultMetadata, title: '分类 · Anxin', path: '/categories' });
});
onUnmounted(() => { controller?.abort(); });
</script>

<template>
  <div class="cat-body">
    <section class="archive" aria-labelledby="archive-title">
      <div class="archive-head row ruled">
        <p class="archive-count" role="status" aria-live="polite">{{ status }}</p>
        <div class="archive-main">
          <h2 id="archive-title">分类</h2>
          <span v-if="config.dataSource === 'mock'" class="archive-sample">示例内容</span>
        </div>
      </div>
    </section>

    <template v-if="loading">
      <section v-for="n in 2" :key="n" class="cat row ruled" aria-hidden="true">
        <div class="facts"><span class="sk sk-fact"></span><span class="sk sk-fact sk-fact-sm"></span></div>
        <div class="main"><span class="sk sk-title"></span><span class="sk sk-line"></span><span class="sk sk-line sk-line-short"></span></div>
      </section>
    </template>

    <section v-else-if="error" class="cat row ruled">
      <div class="main notice">
        <h3>暂时无法读取分类</h3>
        <p>{{ error }}</p>
        <button class="pager-btn" @click="load">重新加载</button>
      </div>
    </section>

    <section v-else-if="!groups.length" class="cat row ruled">
      <div class="main notice">
        <h3>还没有可以看的分类</h3>
        <p>文章发布之后，它所在的分类会出现在这里。</p>
        <router-link class="pager-btn" :to="{ name: 'articles' }">查看全部文章</router-link>
      </div>
    </section>

    <template v-else>
      <!-- 每一类一节：左注文栏是这一类的全部情况，右栏的类名就是节头，行里不再
           重复写类名。类内从新到旧，与文章列表同一条排序。 -->
      <section v-for="group in groups" :key="group.id" class="cat row ruled" :aria-labelledby="'cat-' + group.id">
        <div class="facts">
          <p>{{ group.articleCount }} 篇</p>
          <p>最近 {{ formatDate(group.latestPublishedAt) }}</p>
        </div>
        <div class="main">
          <h3 :id="'cat-' + group.id" class="cat-name">{{ group.name }}</h3>
          <ul class="cat-list">
            <li v-for="article in group.articles" :key="article.id">
              <router-link :to="`/articles/${article.id}/${article.slug}`">{{ article.title }}</router-link>
              <time :datetime="article.publishedAt">{{ formatDate(article.publishedAt) }}</time>
            </li>
          </ul>
          <!-- 篇数超过摆出来的这一截：去文章列表里筛好的那一类，那里有翻页和
               搜索，是能翻到底的地方。 -->
          <p v-if="group.articleCount > group.articles.length" class="cat-all">
            <router-link class="link-quiet" :to="{ name: 'articles', query: { category: group.id }, hash: '#articles' }">全部 {{ group.articleCount }} 篇</router-link>
          </p>
        </div>
      </section>
    </template>
  </div>
</template>
