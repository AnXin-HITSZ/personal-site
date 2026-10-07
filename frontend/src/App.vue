<script setup>
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { config } from './config.js';
import { isAuthor, signedIn } from './session.js';

const route = useRoute();

const year = new Date().getFullYear();

/* 翻页那一层按这个键认人：键变了才播进出场，键不变就只是同一页换了地址。
   文章页只认 id——详情页取到数据后会把地址栏补成规范 slug（router.replace），
   那不是换页，键里带着 slug 就会白播一次过渡。
   写作台新建与编辑共用一个键——新建保存成功后地址从 /new 换成 /:id，
   那一趟连「已保存」那条提示和手上这一稿都该留在原处。
   查询串不进键：它换的是一页里的筛选，不是页。 */
const pageKey = computed(() => {
  if (route.name === 'article') return `article/${route.params.id}`;
  if (route.name === 'admin-article' || route.name === 'admin-article-new') return 'admin-editor';
  return route.path;
});
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
      <router-link to="/categories" :class="{ 'nav-here': $route.name === 'categories' }">分类</router-link>
      <a :href="config.qaUrl" target="_blank" rel="noopener noreferrer">QA-Agent<span class="sr-only">（新窗口）</span></a>
      <router-link
        v-if="isAuthor" to="/admin/articles"
        :class="{ 'nav-here': $route.name === 'admin-articles' || $route.name === 'admin-article'
          || $route.name === 'admin-article-new' || $route.name === 'admin-categories' }"
      >写作</router-link>
      <router-link v-if="signedIn" to="/account" :class="{ 'nav-here': $route.name === 'account' }">账号</router-link>
      <router-link v-else to="/login" :class="{ 'nav-here': $route.name === 'login' }">登录</router-link>
    </nav>
  </header>

  <main id="main" class="shell">
    <!-- 翻页的进出场（见 styles.css「翻页：落纸」）挂在这一层上。router-view 的
         视图是多节点根（首页 = 题记 + 项目 + 列表三段），<transition> 只认单元素，
         所以包一层不画东西的 div；mode="out-in" 是因为两页同时渐变的中间几帧，
         两页的字会叠出重影。 -->
    <transition name="page" mode="out-in">
      <div class="page" :key="pageKey">
        <router-view />
      </div>
    </transition>
  </main>

  <footer class="footer row shell ruled">
    <p class="footer-made">© {{ year }} Anxin</p>
    <div class="footer-main">
      <p>记录，是为了走得更远。</p>
      <a :href="config.siteUrl">anxin-hitsz.com</a>
    </div>
  </footer>
</template>
