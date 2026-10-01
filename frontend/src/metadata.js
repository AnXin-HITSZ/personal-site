import { config } from './config.js';

// 与 index.html 里的静态值保持一致：那里是首屏的初值，这里是路由切换后回填的值。
export const defaultMetadata = Object.freeze({
  title: 'Anxin · 记录与探索',
  description: 'Anxin 的个人网站，记录开发实践与学习笔记，探索 Go、前端与 AI。',
});

/* canonical 由页面自己维护。index.html 里那份是首屏爬到的静态值，路由一切换就不对了，
   所以这里找得到就改、找不到就建一个。 */
function canonicalLink() {
  let link = document.querySelector('link[rel="canonical"]');
  if (!link) {
    link = document.createElement('link');
    link.rel = 'canonical';
    document.head.append(link);
  }
  return link;
}

/* 每个视图在拿到数据之后调一次。path 要用服务端返回的 id 与 slug 拼，不能用
   route.fullPath——地址里那段 slug 可能是旧的，原样写进 canonical 等于把旧地址
   当成正版告诉搜索引擎。 */
export function setMetadata({ title, description, path = '/' }) {
  if (title) document.title = title;
  if (description) document.querySelector('meta[name="description"]')?.setAttribute('content', description);
  canonicalLink().href = `${config.siteUrl.replace(/\/$/, '')}${path}`;
}
