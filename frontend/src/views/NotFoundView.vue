<script setup>
import { useRoute } from 'vue-router';
import { setMetadata } from '../metadata.js';

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
