import { ALL, isCategoryRef } from './api/categories.js';
import { maxListPage, maxSearchRunes } from './api/articles.js';

/* 列表页的筛选住在地址里：分享出去的链接带着筛选，后退键一步步退回上一次筛选。
   这两个函数就是那扇门——read 把 ?page= / ?q= / ?category= 读成一份筛选状态，
   write 把筛选状态写回地址。列表页只认这两进两出，不自己解析地址。 */

/* 读的时候每一段都要过一遍和接口同一套的量法。地址是外人能改的：坏值退回默认值，
   不往下传——否则 normalizeQuery 会当场抛，读者看到的是一整页「格式不符合约定」。 */
export function readListQuery(raw = {}) {
  const page = Number(raw.page ?? 1);
  const category = typeof raw.category === 'string' && isCategoryRef(raw.category) ? raw.category : ALL;
  const q = typeof raw.q === 'string' ? raw.q.trim() : '';
  return {
    page: Number.isInteger(page) && page >= 1 && page <= maxListPage ? page : 1,
    q: [...q].length <= maxSearchRunes ? q : '',
    category,
  };
}

/* 默认值不写进地址：一个没筛过的列表就该是一条干净的地址。键一个个加，不摆
   { page: undefined } 那种——「没有这个键」和「键是 undefined」是两回事，别指望
   中间层替我们把后者吃掉。 */
export function writeListQuery({ page, q, category }) {
  const query = {};
  if (page > 1) query.page = String(page);
  if (q) query.q = q;
  if (category !== ALL) query.category = category;
  return query;
}
