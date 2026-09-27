<script setup>
import { categories } from '../api/articles.js';
import { formatDate } from '../format.js';
defineProps({ article: { type: Object, required: true }, latest: { type: Boolean, default: false } });
</script>

<template>
  <article class="entry row ruled">
    <div class="entry-facts">
      <p v-if="latest" class="entry-mark">最新</p>
      <p class="entry-category">{{ categories[article.category] }}</p>
      <p><time class="entry-date" :datetime="article.publishedAt">{{ formatDate(article.publishedAt) }}</time></p>
      <p>{{ article.readingMinutes }} 分钟阅读</p>
    </div>
    <div class="entry-main">
      <h3 class="entry-title"><router-link :to="`/articles/${article.slug}`">{{ article.title }}</router-link></h3>
      <p class="entry-summary">{{ article.summary }}</p>
      <p v-if="article.tags.length" class="entry-tags">{{ article.tags.join(' / ') }}</p>
    </div>
  </article>
</template>
