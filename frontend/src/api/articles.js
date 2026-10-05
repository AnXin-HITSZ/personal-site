import { config } from '../config.js';
import { articles } from '../mocks/articles.js';
import { isCategoryID, isCategoryRef } from './categories.js';

const CONTRACT_ERROR = '服务返回的数据格式不符合约定，请检查接口契约';

/* 分类名和正文一样是给人看的字，不是配置里那几个固定的词——所以它必须跟着文章
   一起回来。这一行在 /categories 读不到的时候照样要显示，名字缺席就是契约坏了。 */
function hasCategoryName(a) {
  return isCategoryID(a.category) && typeof a.categoryName === 'string' && a.categoryName !== '';
}

export class ArticleNotFoundError extends Error {
  constructor() {
    super('文章不存在');
    this.name = 'ArticleNotFoundError';
  }
}

/* 8 位 Crockford base32 小写。服务端对 :id 的校验就是这一条，不合法的地址它直接
   回 404；前端用同一条规则，mock 与 HTTP 两种模式下「地址不对」才会落到同一处。
   两边要一起改。 */
export const ARTICLE_ID_PATTERN = /^[0-9a-z]{8}$/;

/* 页码和搜索词的上限。列表页的地址栏（list-query.js）和这里各量一遍：那边把坏值
   退回默认值，这里对不上就抛——量的是同一个数，改要一起改。 */
export const maxListPage = 1000000;
export const maxSearchRunes = 100;

/* 参数先在这里量一遍，不合约定就当场抛：省掉一次注定 400 的往返，也让越界在开发时就响，
   而不是等到线上出现一个空列表。这几个上限与服务端 handler 里的那几条是一致的。 */
export function normalizeQuery({ page = 1, pageSize = 6, q = '', category = 'all' } = {}) {
  if (!Number.isInteger(page) || page < 1 || page > maxListPage || !Number.isInteger(pageSize) || pageSize < 1 || pageSize > 50 || typeof q !== 'string' || [...q.trim()].length > maxSearchRunes || !isCategoryRef(category)) {
    throw new Error('查询参数不符合接口约定');
  }
  return { page, pageSize, q: q.trim(), category };
}

/* 排序要跟服务端一致：先按发布时间倒序，同一时刻再按 id——少了这第二把钥匙，
   同一秒发的两篇在两条实现里的先后就是随机的，翻页会跳行。 */
export function mockList(query = {}) {
  const { page, pageSize, q, category } = normalizeQuery(query);
  const keyword = q.toLowerCase();
  const filtered = articles.filter(a => (category === 'all' || a.category === category) && (a.title.toLowerCase().includes(keyword) || a.summary.toLowerCase().includes(keyword)))
    .sort((a, b) => b.publishedAt.localeCompare(a.publishedAt) || a.id.localeCompare(b.id));
  return { items: filtered.slice((page - 1) * pageSize, page * pageSize).map(withheldBody), pagination: { page, pageSize, total: filtered.length, totalPages: Math.ceil(filtered.length / pageSize) } };
}

export function mockDetail(id) {
  const article = articles.find(a => a.id === id);
  return article ? { ...article } : null;
}

/* 列表不带正文——服务端也不带（正文只在详情里读），mock 跟着照做，
   免得本地开发时页面悄悄依赖了线上永远不会给的字段。 */
function withheldBody({ body, ...summary }) {
  return summary;
}

/* 回来的东西要逐项对回契约，连分页算术都对一遍：对不上就抛 CONTRACT_ERROR。
   宁可当场看见一句「格式不符合约定」，也不要让半截数据流进页面——
   那种坏法在界面上只是一个 undefined，回头很难查到是哪一层丢的。 */
function validateResponse(data, query) {
  const p = data?.pagination;
  if (!Array.isArray(data?.items) || !p || p.page !== query.page || p.pageSize !== query.pageSize || !Number.isSafeInteger(p.total) || p.total < 0 || p.totalPages !== Math.ceil(p.total / p.pageSize) || data.items.length !== Math.max(0, Math.min(p.pageSize, p.total - (p.page - 1) * p.pageSize)) || !data.items.every(a =>
    a && ARTICLE_ID_PATTERN.test(a.id) && ['id', 'slug', 'title', 'summary', 'publishedAt'].every(key => typeof a[key] === 'string') && Number.isFinite(Date.parse(a.publishedAt)) && hasCategoryName(a) && Array.isArray(a.tags) && a.tags.every(t => typeof t === 'string') && Number.isInteger(a.readingMinutes) && a.readingMinutes > 0)) {
    throw new Error(CONTRACT_ERROR);
  }
  return data;
}

/* 地址里那段 slug 只是给人看的，查库只按 id。所以这里比的是 id——slug 可以随时改，
   它和服务端返回值不一致是正常的，由详情页把地址归位。 */
function validateDetail(data, id) {
  if (!data || data.id !== id || !['id', 'slug', 'title', 'summary', 'body', 'publishedAt'].every(key => typeof data[key] === 'string') || !Number.isFinite(Date.parse(data.publishedAt)) || !hasCategoryName(data) || !Array.isArray(data.tags) || !data.tags.every(t => typeof t === 'string') || !Number.isInteger(data.readingMinutes) || data.readingMinutes <= 0) {
    throw new Error(CONTRACT_ERROR);
  }
  return data;
}

/* mock 与真接口在这同一支函数里分叉，共用上面那套参数校验和响应校验——
   契约只有一份，改的时候不会只改一边。那 180 毫秒是假的等待，留着它，
   加载态与骨架屏在本地才看得见。 */
export async function listArticles(query = {}, { signal, source = config.dataSource, fetchImpl = fetch } = {}) {
  const normalized = normalizeQuery(query);
  if (source === 'mock') {
    await new Promise(resolve => setTimeout(resolve, 180));
    signal?.throwIfAborted();
    return mockList(normalized);
  }
  if (source !== 'http') throw new Error('未知的数据源配置');
  const params = new URLSearchParams({ page: String(normalized.page), pageSize: String(normalized.pageSize) });
  if (normalized.q) params.set('q', normalized.q);
  if (normalized.category !== 'all') params.set('category', normalized.category);
  const timeout = AbortSignal.timeout(config.requestTimeoutMs);
  const response = await fetchImpl(`${config.apiBaseUrl.replace(/\/$/, '')}/articles?${params}`, { signal: signal ? AbortSignal.any([signal, timeout]) : timeout, headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`文章暂时无法加载（HTTP ${response.status}）`);
  return validateResponse(await response.json(), normalized);
}

export async function getArticle(id, { signal, source = config.dataSource, fetchImpl = fetch } = {}) {
  // 地址里那段指向不存在的资源，不是请求参数写错了，所以按「不存在」处理：
  // 既要走这条路的人看到的是同一句话，也不必为一个必然 404 的地址跑一趟网络。
  const normalized = typeof id === 'string' ? id.trim() : '';
  if (!ARTICLE_ID_PATTERN.test(normalized)) throw new ArticleNotFoundError();

  if (source === 'mock') {
    await new Promise(resolve => setTimeout(resolve, 180));
    signal?.throwIfAborted();
    const found = mockDetail(normalized);
    if (!found) throw new ArticleNotFoundError();
    return found;
  }
  if (source !== 'http') throw new Error('未知的数据源配置');
  const timeout = AbortSignal.timeout(config.requestTimeoutMs);
  const response = await fetchImpl(`${config.apiBaseUrl.replace(/\/$/, '')}/articles/${encodeURIComponent(normalized)}`, { signal: signal ? AbortSignal.any([signal, timeout]) : timeout, headers: { Accept: 'application/json' } });
  if (response.status === 404) throw new ArticleNotFoundError();
  if (!response.ok) throw new Error(`文章暂时无法加载（HTTP ${response.status}）`);
  return validateDetail(await response.json(), normalized);
}
