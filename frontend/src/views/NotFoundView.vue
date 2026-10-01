<script setup>
import { useRoute } from 'vue-router';
import { setMetadata } from '../metadata.js';

/* 兜底路由：没有匹配上别的规则的地址都落到这一页。它没有数据要等，元信息在 setup 里
   直接设好；canonical 指的就是地址栏里这一条——不把它硬指到首页去，错地址就是错地址。 */
const route = useRoute();
setMetadata({ title: '页面不存在 · Anxin', path: route.fullPath });
</script>

<template>
  <section class="piece row ruled">
    <div class="main notice">
      <h1>页面不存在</h1>
      <p>这个地址下没有内容，也许链接已经变更。</p>
      <router-link class="pager-btn" to="/">返回文章列表</router-link>
    </div>
  </section>
</template>
