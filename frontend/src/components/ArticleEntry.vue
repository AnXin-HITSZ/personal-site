<script setup>
import { categories } from '../api/articles.js';
import { formatDate } from '../format.js';
defineProps({ article: { type: Object, required: true }, latest: { type: Boolean, default: false } });
</script>

<template>
  <article class="entry row ruled">
    <div class="facts facts-stamp">
      <p v-if="latest" class="mark">最新</p>
      <p class="key">{{ categories[article.category] }}</p>
      <p><time class="date" :datetime="article.publishedAt">{{ formatDate(article.publishedAt) }}</time></p>
      <p>{{ article.readingMinutes }} 分钟阅读</p>
    </div>
    <div class="main">
      <h3 class="entry-title"><router-link :to="`/articles/${article.id}/${article.slug}`">{{ article.title }}</router-link></h3>
      <p class="entry-summary">{{ article.summary }}</p>
      <p v-if="article.tags.length" class="entry-tags">{{ article.tags.join(' / ') }}</p>
    </div>
  </article>
</template>
