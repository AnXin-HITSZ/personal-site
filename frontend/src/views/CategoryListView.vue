<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { config } from '../config.js';
import { listArticles } from '../api/articles.js';
import { listCategories } from '../api/categories.js';
import { formatDate } from '../format.js';
import { defaultMetadata, setMetadata } from '../metadata.js';
import { isAuthor } from '../session.js';

/* 每一类先摆这么多篇，与文章列表每页同数；读完这一截还有剩，就接一行「全部 N 篇」。
   觉得早或晚改这一个数——等真需要按屏高自适应再说。 */
const previewSize = 6;

const groups = ref([]);
const total = ref(0);
const loading = ref(true);
const error = ref('');
let controller;

/* 页头注文栏两行。上面那行是活的（正在整理 / 加载失败 / 几个分类），读屏靠 role="status"
   知道这一页读完了没有，所以它一个人占一行；篇数那行读完之后才有，不与它并排。 */
const state = computed(() => loading.value ? 'loading' : error.value ? 'failed' : 'ready');
const headLine = computed(() => state.value === 'loading' ? '正在整理分类…' : state.value === 'failed' ? '加载失败' : `${groups.value.length} 个分类`);
const headTotal = computed(() => state.value === 'ready' ? `${total.value} 篇文章` : '');

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
    <!-- 页头与后三种（写作 / 账号 / 写作-分类）同构：左注文栏摆这一页的事实，右栏是页题、
         一行摘要、以及作者才看得见的入口。全是现成的 .piece 一族，没有新样式。
         标题是整页唯一的一个，所以是 h1——这一页从前是站里唯一没有 h1 的页面。 -->
    <section class="piece row ruled" aria-labelledby="archive-title">
      <div class="facts">
        <p role="status" aria-live="polite"><span :class="{ key: state === 'ready' }">{{ headLine }}</span></p>
        <p v-if="headTotal">{{ headTotal }}</p>
        <!-- 开发数据源下才有的标记，只有本地看得见。 -->
        <p v-if="config.dataSource === 'mock'">示例内容</p>
      </div>
      <div class="piece-main">
        <h1 id="archive-title" class="piece-title">分类</h1>
        <p class="piece-summary">每个分类下先摆最近几篇；想看全的，去文章列表里筛那一类。</p>
        <!-- 到这一页来的作者多半是来看读者看见了什么的；想改分类，口子就在摘要下面——
             .piece 的标题自己占一行，标题旁没有槽位，所以走项目页「在线体验」占过的
             .piece-act。这是站里第一件长在公开页里的作者专属物：整行一起给，读者那份
             DOM 连一段空白都不多。 -->
        <p v-if="isAuthor" class="piece-act">
          <router-link class="link-quiet" :to="{ name: 'admin-categories' }">编辑分类</router-link>
        </p>
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
