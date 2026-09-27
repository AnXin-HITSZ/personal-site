import { config } from './config.js';

// 与 index.html 里的静态值保持一致：那里是首屏的初值，这里是路由切换后回填的值。
export const defaultMetadata = Object.freeze({
  title: 'Anxin · 记录与探索',
  description: 'Anxin 的个人网站，记录开发实践与学习笔记，探索 Go、前端与 AI。',
});

function canonicalLink() {
  let link = document.querySelector('link[rel="canonical"]');
  if (!link) {
    link = document.createElement('link');
    link.rel = 'canonical';
    document.head.append(link);
  }
  return link;
}

export function setMetadata({ title, description, path = '/' }) {
  if (title) document.title = title;
  if (description) document.querySelector('meta[name="description"]')?.setAttribute('content', description);
  canonicalLink().href = `${config.siteUrl.replace(/\/$/, '')}${path}`;
}
