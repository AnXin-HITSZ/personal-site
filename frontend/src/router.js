import { createRouter, createWebHistory } from 'vue-router';
import { load, session, signedIn } from './session.js';
import ArticleListView from './views/ArticleListView.vue';
import ArticleDetailView from './views/ArticleDetailView.vue';
import LoginView from './views/LoginView.vue';
import RegisterView from './views/RegisterView.vue';
import VerifyEmailView from './views/VerifyEmailView.vue';
import ForgotPasswordView from './views/ForgotPasswordView.vue';
import ResetPasswordView from './views/ResetPasswordView.vue';
import AccountView from './views/AccountView.vue';
import AdminArticleListView from './views/AdminArticleListView.vue';
import AdminArticleEditView from './views/AdminArticleEditView.vue';
import AdminCategoryListView from './views/AdminCategoryListView.vue';
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
    /* new 必须排在 :id 前面，否则「new」会被当成一个 id 去查库。 */
    {
      path: '/admin/articles',
      name: 'admin-articles',
      component: AdminArticleListView,
      meta: { requiresAuth: true, requiresRole: 'admin' },
    },
    {
      path: '/admin/articles/new',
      name: 'admin-article-new',
      component: AdminArticleEditView,
      meta: { requiresAuth: true, requiresRole: 'admin' },
    },
    {
      path: '/admin/articles/:id',
      name: 'admin-article',
      component: AdminArticleEditView,
      meta: { requiresAuth: true, requiresRole: 'admin' },
    },
    {
      path: '/admin/categories',
      name: 'admin-categories',
      component: AdminCategoryListView,
      meta: { requiresAuth: true, requiresRole: 'admin' },
    },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView },
  ],
  scrollBehavior: (to, from, savedPosition) => savedPosition ?? { top: 0 },
});

/* 一次都没问过就问一次；问不到（网络不好）时当作没登录放行不了，但也不能
   因此崩掉，所以无论哪种失败都退回登录页——那一页自己会说清楚是没登录
   还是连不上。带上 next 是为了登录之后直接回到他本来要去的地方。 */
router.beforeEach(async to => {
  if (!to.meta.requiresAuth && !to.meta.requiresRole) return true;

  if (!signedIn.value) {
    try {
      await load();
    } catch {
      // 下面按未登录处理。
    }
  }

  /* 没登录就说没登录。写作这几页对没登录的人等于不存在，所以这里不区分
     「没登录」和「不是作者」以外的任何情况。 */
  if (to.meta.requiresAuth && !signedIn.value) {
    return { name: 'login', query: { next: to.fullPath } };
  }

  /* 角色在 auth 之后查。不是作者就回首页，不解释：这一站只有一个作者，
     页面上的「无权限」四个字对任何看见它的人都没有用。 */
  if (to.meta.requiresRole && session.account?.role !== to.meta.requiresRole) {
    return { name: 'articles' };
  }

  return true;
});
