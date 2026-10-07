<script setup>
/* 一篇文章的正文页。四种状态占的是同一块版面：正在读、读失败、这篇不存在、读到了。
   每一种都得有出口——地址错了的人也要走得回列表，而不是停在一句「不存在」上。 */
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { renderMarkdown } from '../markdown.js';
import { ArticleNotFoundError, getArticle } from '../api/articles.js';
import { setMetadata } from '../metadata.js';
import { formatDate } from '../format.js';

const route = useRoute();
const router = useRouter();
const article = ref(null);
const loading = ref(true);
const error = ref('');
const missing = ref(false);
const heading = ref(null);
let controller;

const body = computed(() => article.value ? renderMarkdown(article.value.body) : '');

async function load() {
  controller?.abort();
  const current = new AbortController();
  controller = current;
  loading.value = true;
  error.value = '';
  missing.value = false;
  article.value = null;
  const id = String(route.params.id);
  try {
    const found = await getArticle(id, { signal: current.signal });
    if (current.signal.aborted) return;
    article.value = found;
    /* 地址里那段 slug 不参与查询，所以它可能是旧的、也可能整个没带。换成服务端返回
       的那一份，用 replace 不留历史：一篇文章在地址栏里只有一个写法，canonical 也
       才不会把旧 slug 原样写出去。 */
    if (route.params.slug !== found.slug) {
      router.replace({ name: 'article', params: { id: found.id, slug: found.slug } });
    }
    setMetadata({ title: `${found.title} · Anxin`, description: found.summary, path: `/articles/${found.id}/${found.slug}` });
  } catch (cause) {
    if (current.signal.aborted) return;
    if (cause instanceof ArticleNotFoundError) {
      missing.value = true;
      setMetadata({ title: '文章不存在 · Anxin', path: route.fullPath });
    } else {
      error.value = cause.name === 'TimeoutError' ? '请求超时，请稍后重试。'
        : cause instanceof TypeError ? '连接不上服务器，请检查网络后重试。'
        : cause.message;
      setMetadata({ title: '文章暂时无法加载 · Anxin', path: route.fullPath });
    }
  } finally {
    if (!current.signal.aborted) loading.value = false;
  }
}
async function retry() {
  await load();
  await nextTick();
  heading.value?.focus({ preventScroll: true });
}

/* 从一篇文章跳到另一篇时组件被复用，靠 id 的变化重新取数；首次进入是页面加载，不抢焦点。
   上面那次 replace 只改 slug 段，id 不动，所以不会绕回来重新取数。 */
watch(() => route.params.id, async (id, previous) => {
  await load();
  if (previous === undefined) return;
  await nextTick();
  heading.value?.focus({ preventScroll: true });
}, { immediate: true });

onUnmounted(() => controller?.abort());
</script>

<template>
  <div :aria-busy="loading">
    <template v-if="loading">
      <section class="piece row ruled">
        <div class="facts"><span class="sk sk-fact"></span><span class="sk sk-fact sk-fact-sm"></span></div>
        <div class="main" aria-hidden="true">
          <span class="sk sk-piece-title"></span>
          <span class="sk sk-line"></span>
          <span class="sk sk-line sk-line-short"></span>
        </div>
      </section>
    </template>

    <template v-else-if="error">
      <section class="piece row ruled">
        <div class="main notice">
          <!-- tabindex="-1" 是留给脚本聚焦的：换一篇文章、或者重试之后把焦点送到标题上，
               读屏才会念出新的一段（见上面那个 watch）。 -->
          <h3 ref="heading" tabindex="-1">暂时无法读取这篇文章</h3>
          <p>{{ error }}</p>
          <button class="pager-btn" @click="retry">重新加载</button>
        </div>
      </section>
    </template>

    <template v-else-if="missing">
      <section class="piece row ruled">
        <div class="main notice">
          <h1 ref="heading" tabindex="-1">文章不存在</h1>
          <p>这篇文章可能已下线或地址有变。</p>
          <router-link class="pager-btn" to="/">返回文章列表</router-link>
        </div>
      </section>
    </template>

    <template v-else>
      <article class="piece">
        <header class="piece-head row ruled">
          <div class="facts facts-stamp">
            <p class="piece-return"><router-link class="piece-back" to="/">返回文章列表</router-link></p>
            <p class="key">{{ article.categoryName }}</p>
            <p><time class="date" :datetime="article.publishedAt">{{ formatDate(article.publishedAt) }}</time></p>
            <p>{{ article.readingMinutes }} 分钟阅读</p>
          </div>
          <div class="main">
            <h1 ref="heading" class="piece-title" tabindex="-1">{{ article.title }}</h1>
            <p class="piece-summary">{{ article.summary }}</p>
            <p v-if="article.tags.length" class="piece-tags">{{ article.tags.join(' / ') }}</p>
          </div>
        </header>

        <div class="piece-body row">
          <!-- v-html 在这里是安全的：正文由 src/markdown.js 渲染，那里用的是 html: false，
               正文里手写的标签会被转义成文字，能成为标签的只有 Markdown 语法自己生成的。 -->
          <div class="main prose" v-html="body"></div>
        </div>

        <nav class="piece-foot row ruled" aria-label="返回">
          <div class="main">
            <router-link class="piece-back" to="/">返回文章列表</router-link>
          </div>
        </nav>
      </article>
    </template>
  </div>
</template>
