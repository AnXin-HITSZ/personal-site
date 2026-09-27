<script setup>
import { config } from './config.js';
import { signedIn } from './session.js';

const year = new Date().getFullYear();
</script>

<template>
  <a class="skip-link" href="#main">跳到主要内容</a>

  <header class="masthead shell">
    <router-link class="brand" to="/" aria-label="Anxin 首页">Anxin</router-link>
    <nav class="nav" aria-label="主导航">
      <router-link to="/" :class="{ 'nav-here': $route.name === 'articles' || $route.name === 'article' }">文章</router-link>
      <a :href="config.qaUrl" target="_blank" rel="noopener noreferrer">QA-Agent<span class="sr-only">（新窗口）</span></a>
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
