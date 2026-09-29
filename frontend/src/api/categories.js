import { config } from '../config.js';
import { CONTRACT_ERROR, request } from './client.js';
import { mockCategories } from '../mocks/articles.js';

/* 分类 id 出现在两个地方：库里的 categories.id，和筛选地址的 ?category= 那一段。
   形状只管到「像个 id」——某一段是不是真的存在，由服务端说了算（筛出来是空列表）。
   内置那四个是短代号，新加的是 8 位随机码，所以长度是 1 到 16，不是 8。
   要和服务端的 service.IsCategoryID 一起改。 */
export const CATEGORY_ID_PATTERN = /^[0-9a-z]{1,16}$/;

/* 「不筛」在地址里也是一个值，但它不是分类，所以不在任何一份列表里。 */
export const ALL = 'all';

/* 与服务端 service 里那两个上限同一个数。本地先量一遍只为省一次往返，说了算的
   仍是服务端——它说不行就是不行。 */
export const categoryNameRunes = 16;
export const maxCategories = 20;

export function isCategoryID(value) {
  return typeof value === 'string' && CATEGORY_ID_PATTERN.test(value);
}

/* 地址里那一段是不是一个分类的 id：形状对就行，存不存在不归前端判。 */
export function isCategoryRef(value) {
  return isCategoryID(value) || value === ALL;
}

/* 名字不进任何地址，所以除了长度没有别的字符要求——去掉首尾空格之后不为空就行。
   撞名不在这里查：它由库上的唯一索引判，而且大小写不敏感，前端再比一遍只会比出
   第二套规则。 */
export function categoryNameProblem(name) {
  const trimmed = String(name ?? '').trim();
  if (!trimmed) return '名字不能为空';
  if ([...trimmed].length > categoryNameRunes) return `名字不能超过 ${categoryNameRunes} 个字`;
  return '';
}

const runes = value => [...value].length;

function isRef(item) {
  return Boolean(item) && isCategoryID(item.id) && typeof item.name === 'string' && item.name !== '';
}

function isAdminRef(item) {
  return isRef(item) &&
    Number.isSafeInteger(item.articleCount) && item.articleCount >= 0 &&
    Number.isSafeInteger(item.draftCount) && item.draftCount >= 0;
}

function validateList(data, each) {
  if (!Array.isArray(data?.items) || !data.items.every(each)) throw new Error(CONTRACT_ERROR);
  return data;
}

function validateOne(data) {
  if (!isAdminRef(data)) throw new Error(CONTRACT_ERROR);
  return data;
}

function requireHttp() {
  if (config.dataSource !== 'http') throw new Error('写作功能没有 mock 数据源，请设置 VITE_DATA_SOURCE=http');
}

/* 读者那一行：只列有已发布文章的分类，所以它是一个随写作变的列表，不是一张配置。
   读不到时这一整行不出现——文章照常显示各自的分类名（名字跟着文章一起回来）。 */
export async function listCategories({ signal, source = config.dataSource, fetchImpl = fetch } = {}) {
  if (source === 'mock') {
    await new Promise(resolve => setTimeout(resolve, 120));
    signal?.throwIfAborted();
    return { items: mockCategories.map(item => ({ ...item })) };
  }
  if (source !== 'http') throw new Error('未知的数据源配置');
  return validateList(await request('/categories', { signal, fetchImpl }), isRef);
}

export async function listAdminCategories(options = {}) {
  requireHttp();
  return validateList(await request('/admin/categories', options), isAdminRef);
}

export async function createCategory(name, options = {}) {
  requireHttp();
  return validateOne(await request('/admin/categories', { method: 'POST', body: { name }, ...options }));
}

export async function renameCategory(id, name, options = {}) {
  requireHttp();
  return validateOne(await request(`/admin/categories/${encodeURIComponent(id)}`, { method: 'PATCH', body: { name }, ...options }));
}

/* 顺序是整份提交的：上移、下移把整张表的新顺序一次发上去，服务端一个事务写下来。
   两次单独的交换请求中间断一次，顺序就停在只换了一半的地方。 */
export async function reorderCategories(ids, options = {}) {
  requireHttp();
  return request('/admin/categories/order', { method: 'PUT', body: { ids }, ...options });
}

/* moveTo 为空表示「这个分类下面本来就没有文章」。有文章又不给去处，服务端会回
   409——这不是前端拦的，外键也拦着，前端只是把话说在前面。 */
export async function deleteCategory(id, moveTo = '', options = {}) {
  requireHttp();
  const query = moveTo ? `?moveTo=${encodeURIComponent(moveTo)}` : '';
  return request(`/admin/categories/${encodeURIComponent(id)}${query}`, { method: 'DELETE', ...options });
}

export { runes };
