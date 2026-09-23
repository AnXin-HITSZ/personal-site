<script setup>
import { categories } from '../api/articles.js';
defineProps({ article: { type: Object, required: true }, number: { type: Number, required: true } });
const formatDate = value => new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit', timeZone: 'Asia/Shanghai',
}).format(new Date(value));
</script>

<template>
  <article class="article-card">
    <div class="card-top">
      <span :class="['category-name', article.category]">{{ categories[article.category] }}</span>
      <span class="article-number">{{ String(number).padStart(2, '0') }}</span>
    </div>
    <h3>{{ article.title }}</h3>
    <p class="summary">{{ article.summary }}</p>
    <div class="tags"><span v-for="tag in article.tags" :key="tag">{{ tag }}</span></div>
    <div class="card-meta">
      <time :datetime="article.publishedAt">{{ formatDate(article.publishedAt) }}</time>
      <span>{{ article.readingMinutes }} 分钟阅读</span>
    </div>
  </article>
</template>
