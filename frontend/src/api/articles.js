import { config } from '../config.js';
import { articles } from '../mocks/articles.js';

export const categories = { all: '全部', backend: '后端开发', frontend: '前端实践', ai: 'AI 探索', notes: '学习随笔' };

const CONTRACT_ERROR = '服务返回的数据格式不符合约定，请检查接口契约';

export class ArticleNotFoundError extends Error {
  constructor() {
    super('文章不存在');
    this.name = 'ArticleNotFoundError';
  }
}

export function normalizeQuery({ page = 1, pageSize = 6, q = '', category = 'all' } = {}) {
  if (!Number.isInteger(page) || page < 1 || page > 1000000 || !Number.isInteger(pageSize) || pageSize < 1 || pageSize > 50 || typeof q !== 'string' || [...q.trim()].length > 100 || !Object.hasOwn(categories, category)) {
    throw new Error('查询参数不符合接口约定');
  }
  return { page, pageSize, q: q.trim(), category };
}

export function mockList(query = {}) {
  const { page, pageSize, q, category } = normalizeQuery(query);
  const keyword = q.toLowerCase();
  const filtered = articles.filter(a => (category === 'all' || a.category === category) && (a.title.toLowerCase().includes(keyword) || a.summary.toLowerCase().includes(keyword)))
    .sort((a, b) => b.publishedAt.localeCompare(a.publishedAt) || a.id.localeCompare(b.id));
  return { items: filtered.slice((page - 1) * pageSize, page * pageSize).map(withheldBody), pagination: { page, pageSize, total: filtered.length, totalPages: Math.ceil(filtered.length / pageSize) } };
}

export function mockDetail(slug) {
  const article = articles.find(a => a.slug === slug);
  return article ? { ...article } : null;
}

function withheldBody({ body, ...summary }) {
  return summary;
}

function validateResponse(data, query) {
  const p = data?.pagination;
  if (!Array.isArray(data?.items) || !p || p.page !== query.page || p.pageSize !== query.pageSize || !Number.isSafeInteger(p.total) || p.total < 0 || p.totalPages !== Math.ceil(p.total / p.pageSize) || data.items.length !== Math.max(0, Math.min(p.pageSize, p.total - (p.page - 1) * p.pageSize)) || !data.items.every(a =>
    a && ['id', 'slug', 'title', 'summary', 'publishedAt'].every(key => typeof a[key] === 'string') && Number.isFinite(Date.parse(a.publishedAt)) && a.category !== 'all' && Object.hasOwn(categories, a.category) && Array.isArray(a.tags) && a.tags.every(t => typeof t === 'string') && Number.isInteger(a.readingMinutes) && a.readingMinutes > 0)) {
    throw new Error(CONTRACT_ERROR);
  }
  return data;
}

function validateDetail(data, slug) {
  if (!data || data.slug !== slug || !['id', 'slug', 'title', 'summary', 'body', 'publishedAt'].every(key => typeof data[key] === 'string') || !Number.isFinite(Date.parse(data.publishedAt)) || data.category === 'all' || !Object.hasOwn(categories, data.category) || !Array.isArray(data.tags) || !data.tags.every(t => typeof t === 'string') || !Number.isInteger(data.readingMinutes) || data.readingMinutes <= 0) {
    throw new Error(CONTRACT_ERROR);
  }
  return data;
}

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

export async function getArticle(slug, { signal, source = config.dataSource, fetchImpl = fetch } = {}) {
  if (typeof slug !== 'string' || !slug.trim()) throw new Error('文章标识不符合接口约定');
  const normalized = slug.trim();
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
