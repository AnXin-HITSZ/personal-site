import { createRouter, createWebHistory } from 'vue-router';
import { load, signedIn } from './session.js';
import ArticleListView from './views/ArticleListView.vue';
import ArticleDetailView from './views/ArticleDetailView.vue';
import LoginView from './views/LoginView.vue';
import RegisterView from './views/RegisterView.vue';
import VerifyEmailView from './views/VerifyEmailView.vue';
import ForgotPasswordView from './views/ForgotPasswordView.vue';
import ResetPasswordView from './views/ResetPasswordView.vue';
import AccountView from './views/AccountView.vue';
import NotFoundView from './views/NotFoundView.vue';

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'articles', component: ArticleListView },
    { path: '/articles/:id/:slug?', name: 'article', component: ArticleDetailView },
    { path: '/login', name: 'login', component: LoginView },
    { path: '/register', name: 'register', component: RegisterView },
    { path: '/verify-email', name: 'verify-email', component: VerifyEmailView },
    { path: '/forgot-password', name: 'forgot-password', component: ForgotPasswordView },
    { path: '/reset-password', name: 'reset-password', component: ResetPasswordView },
    { path: '/account', name: 'account', component: AccountView, meta: { requiresAuth: true } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView },
  ],
  scrollBehavior: (to, from, savedPosition) => savedPosition ?? { top: 0 },
});

/* 一次都没问过就问一次；问不到（网络不好）时当作没登录放行不了，但也不能
   因此崩掉，所以无论哪种失败都退回登录页——那一页自己会说清楚是没登录
   还是连不上。带上 next 是为了登录之后直接回到他本来要去的地方。 */
router.beforeEach(async to => {
  if (!to.meta.requiresAuth || signedIn.value) return true;

  try {
    await load();
  } catch {
    // 下面按未登录处理。
  }

  if (signedIn.value) return true;
  return { name: 'login', query: { next: to.fullPath } };
});
