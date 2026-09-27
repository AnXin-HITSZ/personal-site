<script setup>
import { onMounted } from 'vue';
import { config } from '../config.js';
import { defaultMetadata, setMetadata } from '../metadata.js';
import ArticleList from '../components/ArticleList.vue';

const year = new Date().getFullYear();

onMounted(() => setMetadata({ ...defaultMetadata, path: '/' }));
</script>

<template>
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
</template>
