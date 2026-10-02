<script setup>
import { onMounted } from 'vue';
import { defaultMetadata, setMetadata } from '../metadata.js';
import ArticleList from '../components/ArticleList.vue';

/* 左栏那个大字年份。取的是打开页面那一刻的年份，不写死——跨年那天它自己会变。 */
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
      <p class="project-line">融合 SOP 流程指引、知识库检索与图文理解的实验室智能问答助手。</p>
      <!-- 站内链接：项目页是这一站自己的页，就在这一页里翻开，不开新窗口，因此
           也没有「（新窗口）」那句话。直接去应用的那条路留在报头「QA-Agent」上。 -->
      <router-link class="project-link" to="/projects/qa-agent">了解项目</router-link>
    </div>
  </aside>

  <ArticleList />
</template>
