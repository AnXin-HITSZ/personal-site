import { config } from '../config.js';
import { CONTRACT_ERROR, request, requestMultipart } from './client.js';
import { ARTICLE_ID_PATTERN } from './articles.js';
import { isCategoryID } from './categories.js';

/* 写作这一路没有 mock 数据源：假的草稿变不出真的库，存下去的东西下次打开
   就不见了，跑起来只会骗人。理由和 account.js 一样。 */
function requireHttp() {
  if (config.dataSource !== 'http') throw new Error('写作功能没有 mock 数据源，请设置 VITE_DATA_SOURCE=http');
}

export const statusText = Object.freeze({ draft: '草稿', published: '已发布' });

/* 分类的选项不在这儿：分类是一份随写作变的数据，不是一张写死的表，所以它由
   /admin/categories 现取（见 api/categories.js）。这里只管收到的分类 id 像不像话。 */

/* 与服务端 service 里那几个上限同一个数。本地先量一遍只为省一次往返，说了算的
   仍是服务端——它说不行就是不行。 */
export const articleLimits = Object.freeze({
  slugRunes: 120,
  titleRunes: 255,
  summaryRunes: 500,
  bodyRunes: 200000,
  tagRunes: 32,
  tags: 10,
});

const slugPattern = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const runes = value => [...value].length;

/* 一个字段一句话，空串表示这个字段没问题。视图照着键名把话放到对应的框下面。 */
export function articleProblems(input) {
  const slug = String(input.slug ?? '').trim();
  const title = String(input.title ?? '').trim();
  const summary = String(input.summary ?? '').trim();
  const body = String(input.body ?? '').trim();

  return {
    slug: !slugPattern.test(slug) || runes(slug) > articleLimits.slugRunes
      ? '只能用小写字母、数字和连字符，且不能以连字符开头或结尾' : '',
    title: !title ? '标题不能为空'
      : runes(title) > articleLimits.titleRunes ? `标题不能超过 ${articleLimits.titleRunes} 个字符` : '',
    summary: !summary ? '摘要不能为空'
      : runes(summary) > articleLimits.summaryRunes ? `摘要不能超过 ${articleLimits.summaryRunes} 个字符` : '',
    body: !body ? '正文不能为空'
      : runes(body) > articleLimits.bodyRunes ? `正文不能超过 ${articleLimits.bodyRunes} 个字符` : '',
  };
}

function payload(input) {
  return {
    slug: String(input.slug ?? '').trim(),
    title: String(input.title ?? '').trim(),
    summary: String(input.summary ?? '').trim(),
    body: String(input.body ?? ''),
    category: input.category,
    tags: Array.isArray(input.tags) ? [...input.tags] : [],
    status: input.status,
  };
}

function isSummary(a) {
  return Boolean(a) &&
    ARTICLE_ID_PATTERN.test(a.id) && a.id === a.id.toLowerCase() &&
    typeof a.slug === 'string' && typeof a.title === 'string' && typeof a.summary === 'string' &&
    isCategoryID(a.category) &&
    (a.status === 'draft' || a.status === 'published') &&
    Array.isArray(a.tags) && a.tags.every(tag => typeof tag === 'string') &&
    Number.isInteger(a.readingMinutes) && a.readingMinutes > 0 &&
    Number.isFinite(Date.parse(a.createdAt)) && Number.isFinite(Date.parse(a.updatedAt)) &&
    /* 已发布的必然有发布时刻，草稿必然是空的——这是服务端的四种迁移规则落到
       数据上的样子，读的时候照着查一遍，页面才不会拿一个空时刻去格式化。 */
    (a.status === 'published'
      ? typeof a.publishedAt === 'string' && Number.isFinite(Date.parse(a.publishedAt))
      : a.publishedAt === null);
}

function validateList(data, query) {
  const p = data?.pagination;
  if (!Array.isArray(data?.items) || !data.items.every(isSummary) || !p || p.page !== query.page ||
    p.pageSize !== query.pageSize || !Number.isSafeInteger(p.total) || p.total < 0 ||
    p.totalPages !== Math.ceil(p.total / p.pageSize) ||
    data.items.length !== Math.max(0, Math.min(p.pageSize, p.total - (p.page - 1) * p.pageSize))) {
    throw new Error(CONTRACT_ERROR);
  }
  return data;
}

/* bodyRunes 是服务端在保存时按正文数出来的，前端一个数都不算。它缺席时
   （后端那支 migration 还没上）只说明现在还没得显示，不是契约坏了——所以只
   在有值的时候查它的形状，编辑台那一行照旧显示，只是少一段。 */
function validateDetail(data, id) {
  const bodyRunes = data?.bodyRunes;
  const hasRunes = bodyRunes !== undefined;
  if (!isSummary(data) || (id !== undefined && data.id !== id) || typeof data.body !== 'string' ||
    (hasRunes && (!Number.isInteger(bodyRunes) || bodyRunes < 0))) {
    throw new Error(CONTRACT_ERROR);
  }
  return hasRunes ? data : { ...data, bodyRunes: null };
}

function validateUpload(data) {
  if (!data || typeof data.url !== 'string' || typeof data.key !== 'string' ||
    !Number.isSafeInteger(data.bytes) || data.bytes <= 0 || typeof data.contentType !== 'string') {
    throw new Error(CONTRACT_ERROR);
  }
  return data;
}

export async function listAdminArticles({ page = 1, pageSize = 20, status = 'all', category = 'all', q = '' } = {}, options = {}) {
  requireHttp();
  const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
  if (status !== 'all') params.set('status', status);
  if (category !== 'all') params.set('category', category);
  if (q) params.set('q', q);
  return validateList(await request(`/admin/articles?${params}`, options), { page, pageSize });
}

export async function getAdminArticle(id, options = {}) {
  requireHttp();
  return validateDetail(await request(`/admin/articles/${encodeURIComponent(id)}`, options), id);
}

export async function createArticle(input, options = {}) {
  requireHttp();
  const detail = await request('/admin/articles', { method: 'POST', body: payload(input), ...options });
  return validateDetail(detail);
}

export async function updateArticle(id, input, options = {}) {
  requireHttp();
  return validateDetail(
    await request(`/admin/articles/${encodeURIComponent(id)}`, { method: 'PUT', body: payload(input), ...options }),
    id,
  );
}

export async function deleteArticle(id, options = {}) {
  requireHttp();
  return request(`/admin/articles/${encodeURIComponent(id)}`, { method: 'DELETE', ...options });
}

export async function uploadImage(file, options = {}) {
  requireHttp();
  const form = new FormData();
  form.append('file', file);
  return validateUpload(await requestMultipart('/admin/uploads', form, options));
}
