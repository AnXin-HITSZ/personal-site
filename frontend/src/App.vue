<script setup>
import { config } from './config.js';
import ArticleList from './components/ArticleList.vue';
const year = new Date().getFullYear();
</script>

<template>
  <a class="skip-link" href="#articles">跳到文章列表</a>
  <header class="site-header wrap">
    <a class="brand" href="/" aria-label="Anxin 首页"><span class="brand-mark">a.</span> ANXIN<span class="brand-sub">/ 个人空间</span></a>
    <nav aria-label="主导航"><a class="active" href="#articles">文章</a><a :href="config.qaUrl" target="_blank" rel="noopener noreferrer">QA-Agent <span aria-hidden="true">↗</span><span class="sr-only">（新窗口）</span></a></nav>
  </header>
  <main class="wrap">
    <section class="intro" aria-labelledby="intro-title">
      <div><p class="eyebrow">NOTES & EXPLORATIONS</p><h1 id="intro-title">把思考写下来，<br>让想法发生。</h1><p class="intro-copy">这里是 Anxin 的个人空间。<br>记录代码、学习，以及从问题到答案的过程。</p></div>
      <aside class="project-note"><span class="note-index">PROJECT / 01</span><h2>QA-Agent</h2><p>从一个问题开始，<br>探索另一种寻找答案的方式。</p><a :href="config.qaUrl" target="_blank" rel="noopener noreferrer">打开 QA-Agent <span aria-hidden="true">↗</span><span class="sr-only">（新窗口）</span></a></aside>
    </section>
    <ArticleList />
  </main>
  <footer class="wrap"><span>© {{ year }} Anxin</span><span>记录，是为了走得更远。</span><a :href="config.siteUrl">anxin-hitsz.com <span aria-hidden="true">↗</span></a></footer>
</template>
