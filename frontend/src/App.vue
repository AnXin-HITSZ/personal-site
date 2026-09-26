<script setup>
import { config } from './config.js';
import ArticleList from './components/ArticleList.vue';
const year = new Date().getFullYear();
</script>

<template>
  <a class="skip-link" href="#articles">跳到文章列表</a>

  <header class="masthead shell">
    <a class="brand" href="/" aria-label="Anxin 首页">Anxin</a>
    <nav class="nav" aria-label="主导航">
      <a href="#articles" aria-current="true">文章</a>
      <a :href="config.qaUrl" target="_blank" rel="noopener noreferrer">QA-Agent<span class="sr-only">（新窗口）</span></a>
    </nav>
  </header>

  <main class="shell">
    <section class="statement row">
      <p class="statement-fact">{{ year }}</p>
      <div class="statement-body">
        <h1>把思考写下来，<br>让想法发生。</h1>
        <p class="statement-lede">这里是 Anxin 的个人空间。记录代码、学习，以及从问题到答案的过程。</p>
      </div>
    </section>

    <aside class="project row">
      <p class="project-fact">项目</p>
      <div class="project-body">
        <h2 class="project-name">QA-Agent</h2>
        <p class="project-line">从一个问题开始，探索另一种寻找答案的方式。</p>
        <a class="project-link" :href="config.qaUrl" target="_blank" rel="noopener noreferrer">打开 QA-Agent<span class="sr-only">（新窗口）</span></a>
      </div>
    </aside>

    <ArticleList />
  </main>

  <footer class="footer row shell ruled">
    <p class="footer-made">© {{ year }} Anxin</p>
    <div class="footer-main">
      <p>记录，是为了走得更远。</p>
      <a :href="config.siteUrl">anxin-hitsz.com</a>
    </div>
  </footer>
</template>
