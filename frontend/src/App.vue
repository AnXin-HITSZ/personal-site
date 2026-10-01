<script setup>
import { computed } from 'vue';
import { config } from './config.js';
import { session, signedIn } from './session.js';

const year = new Date().getFullYear();

/* 「写作」只在作者看得见的地方出现——对别人它连一个字都不该有。 */
const isAuthor = computed(() => session.account?.role === 'admin');
</script>

<template>
  <!-- 键盘用户按第一下 Tab 就能跳过整个报头，直接落到正文。 -->
  <a class="skip-link" href="#main">跳到主要内容</a>

  <!-- 报头 + 正文 + 页脚，三段都套 shell 收在同一个版心里。 -->
  <header class="masthead shell">
    <router-link class="brand" to="/" aria-label="Anxin 首页">Anxin</router-link>
    <nav class="nav" aria-label="主导航">
      <!-- nav-here 是比路由名，不是比路径前缀：列表页和详情页同属「文章」这一栏。 -->
      <router-link to="/" :class="{ 'nav-here': $route.name === 'articles' || $route.name === 'article' }">文章</router-link>
      <a :href="config.qaUrl" target="_blank" rel="noopener noreferrer">QA-Agent<span class="sr-only">（新窗口）</span></a>
      <router-link
        v-if="isAuthor" to="/admin/articles"
        :class="{ 'nav-here': $route.name === 'admin-articles' || $route.name === 'admin-article' || $route.name === 'admin-article-new' }"
      >写作</router-link>
      <router-link v-if="signedIn" to="/account" :class="{ 'nav-here': $route.name === 'account' }">账号</router-link>
      <router-link v-else to="/login" :class="{ 'nav-here': $route.name === 'login' }">登录</router-link>
    </nav>
  </header>

  <main id="main" class="shell">
    <router-view />
  </main>

  <footer class="footer row shell ruled">
    <p class="footer-made">© {{ year }} Anxin</p>
    <div class="footer-main">
      <p>记录，是为了走得更远。</p>
      <a :href="config.siteUrl">anxin-hitsz.com</a>
    </div>
  </footer>
</template>
