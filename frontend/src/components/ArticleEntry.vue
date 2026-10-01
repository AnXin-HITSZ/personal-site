<script setup>
import { formatDate } from '../format.js';
/* 列表里的一行。latest 是由列表页算好传进来的：判断「这一篇是不是全站最新」得先知道
   当前的筛选和页码，而那件事只有列表页知道。 */
defineProps({ article: { type: Object, required: true }, latest: { type: Boolean, default: false } });
</script>

<template>
  <article class="entry row ruled">
    <div class="facts facts-stamp">
      <p v-if="latest" class="mark">最新</p>
      <!-- 名字跟着文章一起回来的，不是从分类表里现查的：分类那一行读不出来的时候，
           这一行照旧要写得出落款。 -->
      <p class="key">{{ article.categoryName }}</p>
      <!-- datetime 是给机器读的那一份（ISO 原文），页面上显示的是给人读的一份。 -->
      <p><time class="date" :datetime="article.publishedAt">{{ formatDate(article.publishedAt) }}</time></p>
      <p>{{ article.readingMinutes }} 分钟阅读</p>
    </div>
    <div class="main">
      <!-- 地址里 id 和 slug 都在：id 决定查到哪一篇，slug 只是转发出去时让人看出这是哪一篇。
           改 slug 不作废链接，靠的正是「查库只看 id」。 -->
      <h3 class="entry-title"><router-link :to="`/articles/${article.id}/${article.slug}`">{{ article.title }}</router-link></h3>
      <p class="entry-summary">{{ article.summary }}</p>
      <p v-if="article.tags.length" class="entry-tags">{{ article.tags.join(' / ') }}</p>
    </div>
  </article>
</template>
