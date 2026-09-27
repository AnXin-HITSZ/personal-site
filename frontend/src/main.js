import { createApp } from 'vue';
import App from './App.vue';
import { router } from './router.js';
import { load } from './session.js';
import './styles.css';

/* 先问一次登录态，但不等它：首屏不该为了报头上那一个字多等一个来回。
   路由守卫在需要它的页面（账号页）会自己再等一次。 */
load().catch(() => {});

createApp(App).use(router).mount('#app');
