import { createRouter, createWebHistory } from 'vue-router';
import ArticleListView from './views/ArticleListView.vue';
import ArticleDetailView from './views/ArticleDetailView.vue';
import NotFoundView from './views/NotFoundView.vue';

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'articles', component: ArticleListView },
    { path: '/articles/:slug', name: 'article', component: ArticleDetailView },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView },
  ],
  scrollBehavior: (to, from, savedPosition) => savedPosition ?? { top: 0 },
});
