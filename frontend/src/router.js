import { createRouter, createWebHistory } from 'vue-router';
import { load, session, signedIn } from './session.js';
import ArticleListView from './views/ArticleListView.vue';
import ArticleDetailView from './views/ArticleDetailView.vue';
import ProjectQaAgentView from './views/ProjectQaAgentView.vue';
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

/* 地址是真实的路径（createWebHistory），不是 #/ 那一种：文章要能被搜索引擎收录、被别处
   直接粘链接。页面路由本身 Go 看不到——nginx 用 try_files 兜到 index.html，由前端接管。 */

/* 旧页淡出用多久，回页首就压后多久（styles.css「翻页：落纸」里那个 .15s）。
   matchMedia 每次读 .matches 拿的都是当下的设置，改系统开关不用刷新。 */
const pageLeaveMs = 150;
const reduceMotion = matchMedia('(prefers-reduced-motion: reduce)');

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'articles', component: ArticleListView },
    // slug 段可选，也不参与查询：老地址、手抄漏一段的地址都落得到同一篇上，
    // 拿到数据之后再让详情页把地址栏换成规范写法。
    { path: '/articles/:id/:slug?', name: 'article', component: ArticleDetailView },
    /* 项目详情页。报头那条「QA-Agent」仍旧直接去应用，这一页是站内的另一处：
       首页那一栏点「了解项目」落到这儿。正文由主人自己写（见 ProjectQaAgentView）。 */
    { path: '/projects/qa-agent', name: 'project-qa-agent', component: ProjectQaAgentView },
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
  /* 后退/前进回到浏览器记着的那个位置，其余情况一律回页首——换一页却停在半截最让人迷路。
     回页首压后到旧页淡完之后：翻页那 0.15s 里旧页还看得见（styles.css「翻页：落纸」），
     立刻跳的话它会在原地「唰」地滚回顶上再消失。这两个数是同一个数，改要一起改。
     少动的人那里没有过渡，也就不等。 */
  scrollBehavior: (to, from, savedPosition) =>
    new Promise(resolve => {
      setTimeout(() => resolve(savedPosition ?? { top: 0 }), reduceMotion.matches ? 0 : pageLeaveMs);
    }),
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
