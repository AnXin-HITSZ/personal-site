<script setup>
/* QA-Agent 的项目详情页。正文还没写，这里先把骨架立住：标题、一条回到首页的路、
   一枚去应用的按钮，加上正文那一栏的位置。空着就空着——不摆空框，也不写「内容
   建设中」，那类字比一片空白更像这一页坏了。 */
import { onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import MarkdownIt from 'markdown-it';
import { config } from '../config.js';
import { setMetadata } from '../metadata.js';
/* 正文写在 src/content/projects/qa-agent.md：Markdown 原文，和写文章同一套语法，
   文件现在是空的。存盘之后开发服务器自己会读到新的一版。 */
import source from '../content/projects/qa-agent.md?raw';

const route = useRoute();
const heading = ref(null);

/* markdown-it 默认 html: false，正文里手写的标签会被转义后原样显示，能成为标签的
   只有 Markdown 语法自己生成的——和文章详情页同一条。 */
const markdown = new MarkdownIt();
const body = markdown.render(source);

/* 页面上不再写一遍简介（正文由主人自己写），但 description 不能空着：空着会留着
   上一页那一句，搜索引擎读到的就是别的页。这一句与首页「项目」那一栏是同一句。 */
const summary = '融合 SOP 流程指引、知识库检索与图文理解的实验室智能问答助手。';

onMounted(() => {
  setMetadata({ title: 'QA-Agent · Anxin', description: summary, path: route.fullPath });
  /* 从首页那一栏点进来的人，焦点本来在「了解项目」上，那枚链接跟着上一页消失了。
     交给标题：接着按 Tab 是往下走，而不是回到报头重走一遍。 */
  heading.value?.focus({ preventScroll: true });
});
</script>

<template>
  <article class="piece">
    <header class="piece-head row ruled">
      <div class="facts">
        <p class="key">项目</p>
      </div>
      <div class="main">
        <!-- tabindex="-1" 是留给脚本聚焦的，见上面那段 onMounted。 -->
        <h1 ref="heading" class="piece-title" tabindex="-1">QA-Agent</h1>
        <!-- 去应用的那一枚，是全站唯一还带「（新窗口）」提示的站内按钮。站外链接：
             noopener 让新窗口拿不到本页的 window 句柄，noreferrer 顺手把来源也隐掉；
             读屏用户听不见「会开新窗口」，所以要写出来。 -->
        <p class="piece-act">
          <a class="pager-btn" :href="config.qaUrl" target="_blank" rel="noopener noreferrer">在线体验<span class="sr-only">（新窗口）</span></a>
        </p>
      </div>
    </header>

    <!-- 正文一栏。空的时候整块不出现：一块空的 .prose 照样占着 3rem 的上边距，
         看着像内容丢了。 -->
    <div v-if="body" class="piece-body row">
      <!-- v-html 在这里是安全的：上面那个 MarkdownIt 用的是默认的 html: false，
           正文里手写的标签会被转义成文字，能成为标签的只有 Markdown 语法自己生成的。 -->
      <div class="main prose" v-html="body"></div>
    </div>

    <nav class="piece-foot row ruled" aria-label="返回">
      <div class="main">
        <router-link class="piece-back" to="/">返回首页</router-link>
      </div>
    </nav>
  </article>
</template>
