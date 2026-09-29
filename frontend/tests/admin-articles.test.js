import test from 'node:test';
import assert from 'node:assert/strict';
import {
  articleLimits, articleProblems, categoryOptions, createArticle, deleteArticle, getAdminArticle,
  listAdminArticles, statusText, updateArticle, uploadImage,
} from '../src/api/admin-articles.js';
import { insertAtCursor, altFromFileName, imageMarkdown, shortenURL } from '../src/insert.js';
import { uploadText } from '../src/copy.js';
import { codes } from '../src/api/client.js';

function json(body, status = 200) {
  return new Response(JSON.stringify(body), { status });
}

function capture(result, status = 200) {
  const seen = {};
  return {
    seen,
    fetchImpl: async (url, options) => {
      Object.assign(seen, { url, options });
      return result === null ? new Response(null, { status }) : json(result, status);
    },
  };
}

/* 一份形状正确的摘要，各条测试在它上面改一处再断言被拒。 */
function summary(overrides = {}) {
  return {
    id: 'abcd2345',
    slug: 'go-api-first-step',
    title: '从零写一个 Go API',
    summary: '把路由、中间件和错误处理串成一条线。',
    category: 'backend',
    tags: ['go', 'api'],
    status: 'published',
    publishedAt: '2026-09-20T02:00:00Z',
    readingMinutes: 6,
    createdAt: '2026-09-19T02:00:00Z',
    updatedAt: '2026-09-20T02:00:00Z',
    ...overrides,
  };
}

const detail = (overrides = {}) => ({ ...summary(), body: '# 标题\n\n正文', ...overrides });

const listBody = (query, overrides = {}) => ({
  items: [summary()],
  pagination: { page: query.page, pageSize: query.pageSize, total: 1, totalPages: 1 },
  ...overrides,
});

test('the category picker offers the names the public site already uses', () => {
  assert.deepEqual(categoryOptions, [
    { value: 'backend', label: '后端开发' },
    { value: 'frontend', label: '前端实践' },
    { value: 'ai', label: 'AI 探索' },
    { value: 'notes', label: '学习随笔' },
  ]);
  assert.equal(categoryOptions.some(option => option.value === 'all'), false);
  assert.deepEqual(statusText, { draft: '草稿', published: '已发布' });
});

test('local validation names one field at a time and agrees with the server limits', () => {
  const good = { slug: 'go-api', title: '标题', summary: '摘要', body: '正文' };
  assert.deepEqual(articleProblems(good), { slug: '', title: '', summary: '', body: '' });

  assert.match(articleProblems({ ...good, slug: 'Go API' }).slug, /小写字母/);
  assert.match(articleProblems({ ...good, slug: '-go' }).slug, /小写字母/);
  assert.match(articleProblems({ ...good, slug: '' }).slug, /小写字母/);
  assert.match(articleProblems({ ...good, title: '   ' }).title, /标题/);
  assert.match(articleProblems({ ...good, summary: '' }).summary, /摘要/);
  assert.match(articleProblems({ ...good, body: ' ' }).body, /正文/);

  // 按字符数（rune）量，不按字节：一个汉字就是 1。
  assert.equal(articleProblems({ ...good, title: '字'.repeat(articleLimits.titleRunes) }).title, '');
  assert.match(articleProblems({ ...good, title: '字'.repeat(articleLimits.titleRunes + 1) }).title, /255/);
  assert.equal(articleProblems({ ...good, summary: '字'.repeat(articleLimits.summaryRunes) }).summary, '');
  assert.match(articleProblems({ ...good, summary: '字'.repeat(articleLimits.summaryRunes + 1) }).summary, /500/);
  assert.equal(articleProblems({ ...good, body: '字'.repeat(articleLimits.bodyRunes) }).body, '');
  assert.match(articleProblems({ ...good, body: '字'.repeat(articleLimits.bodyRunes + 1) }).body, /200000/);
});

test('the list request omits the filters that mean "everything"', async () => {
  const query = { page: 1, pageSize: 10 };
  const { seen, fetchImpl } = capture(listBody(query));
  const result = await listAdminArticles(query, { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/articles?page=1&pageSize=10');
  assert.equal(seen.options.method, 'GET');
  assert.equal(seen.options.headers['X-Requested-With'], undefined);
  assert.equal(result.items.length, 1);
});

test('draft and category filters are carried, and a keyword is encoded', async () => {
  const query = { page: 2, pageSize: 10, status: 'draft', category: 'notes', q: '读书 笔记' };
  const { seen, fetchImpl } = capture({ items: [], pagination: { page: 2, pageSize: 10, total: 10, totalPages: 1 } });
  await listAdminArticles(query, { fetchImpl });

  // URLSearchParams 把空格写成 +；Gin 读回来是同一个空格。
  assert.equal(
    seen.url,
    `/api/v1/admin/articles?page=2&pageSize=10&status=draft&category=notes&q=${encodeURIComponent('读书 笔记').replace('%20', '+')}`,
  );
});

/* 草稿没有发布时刻，这是正常数据，不是违规——公开接口那一族 DTO 在这里不能套用。 */
test('a draft with a null publishedAt is a valid row', async () => {
  const query = { page: 1, pageSize: 10 };
  const draft = summary({ status: 'draft', publishedAt: null });
  const result = await listAdminArticles(query, { fetchImpl: async () => json(listBody(query, { items: [draft] })) });
  assert.equal(result.items[0].status, 'draft');
});

test('a row whose shape drifted is rejected rather than rendered', async () => {
  const query = { page: 1, pageSize: 10 };
  const broken = [
    summary({ publishedAt: null }),                       // 已发布却没有发布时刻
    summary({ status: 'draft' }),                         // 草稿却带着发布时刻
    summary({ category: 'all' }),                         // all 是「不筛」，不是分类
    summary({ tags: 'go' }),                              // 标签必须是数组
    summary({ readingMinutes: 0 }),                       // 时长下限是 1
    summary({ id: 'ABCD2345' }),                          // id 只有小写
    summary({ id: 'short' }),
    summary({ createdAt: 'invalid' }),
  ];
  for (const row of broken) {
    const cause = await listAdminArticles(query, {
      fetchImpl: async () => json(listBody(query, { items: [row] })),
    }).catch(error => error);
    assert.match(cause.message, /数据格式/, `${JSON.stringify(row)} 应当被拒`);
  }
});

test('pagination has to agree with the rows that came with it', async () => {
  const query = { page: 1, pageSize: 10 };
  const cases = [
    { items: [summary()], pagination: { page: 2, pageSize: 10, total: 1, totalPages: 1 } },
    { items: [summary()], pagination: { page: 1, pageSize: 20, total: 1, totalPages: 1 } },
    { items: [summary()], pagination: { page: 1, pageSize: 10, total: 1, totalPages: 3 } },
    { items: [], pagination: { page: 1, pageSize: 10, total: 1, totalPages: 1 } },
    { pagination: { page: 1, pageSize: 10, total: 1, totalPages: 1 } },
  ];
  for (const body of cases) {
    await assert.rejects(listAdminArticles(query, { fetchImpl: async () => json(body) }), /数据格式/);
  }
});

test('the detail read checks the id it asked for', async () => {
  const { seen, fetchImpl } = capture(detail());
  const article = await getAdminArticle('abcd2345', { fetchImpl });
  assert.equal(seen.url, '/api/v1/admin/articles/abcd2345');
  assert.equal(article.body, '# 标题\n\n正文');

  const other = await getAdminArticle('abcd2345', { fetchImpl: async () => json(detail({ id: 'zzzz9999' })) })
    .catch(error => error);
  assert.match(other.message, /数据格式/);
});

/* bodyRunes 还不在后端。缺了它只该少显示一段字，不该把整篇判成坏的；
   但它一旦出现，形状就得对——半个数字比没有数字更糟。 */
test('bodyRunes is tolerated while absent and checked once it appears', async () => {
  const tolerated = await getAdminArticle('abcd2345', { fetchImpl: async () => json(detail()) });
  assert.equal(tolerated.bodyRunes, null);

  const present = await getAdminArticle('abcd2345', { fetchImpl: async () => json(detail({ bodyRunes: 2140 })) });
  assert.equal(present.bodyRunes, 2140);

  for (const value of [1.5, -1, '2140', null]) {
    await assert.rejects(
      getAdminArticle('abcd2345', { fetchImpl: async () => json(detail({ bodyRunes: value })) }),
      /数据格式/,
    );
  }
});

test('create posts the whole draft and trims what the server will trim', async () => {
  const input = {
    slug: '  go-api  ', title: '  标题  ', summary: '  摘要  ', body: '正文',
    category: 'backend', tags: ['go'], status: 'draft',
  };
  const { seen, fetchImpl } = capture(detail({ status: 'draft', publishedAt: null, slug: 'go-api' }));
  await createArticle(input, { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/articles');
  assert.equal(seen.options.method, 'POST');
  assert.equal(seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
  assert.equal(seen.options.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(seen.options.body), {
    slug: 'go-api', title: '标题', summary: '摘要', body: '正文',
    category: 'backend', tags: ['go'], status: 'draft',
  });
});

test('update targets the id and a missing article comes back as an error', async () => {
  const { seen, fetchImpl } = capture(detail());
  await updateArticle('abcd2345', { slug: 'go-api', title: 't', summary: 's', body: 'b', category: 'ai', tags: [], status: 'published' }, { fetchImpl });
  assert.equal(seen.url, '/api/v1/admin/articles/abcd2345');
  assert.equal(seen.options.method, 'PUT');

  const cause = await updateArticle('zzzz9999', {}, {
    fetchImpl: async () => json({ error: { code: 'NOT_FOUND', message: '文章不存在' } }, 404),
  }).catch(error => error);
  assert.equal(cause.code, codes.notFound);
});

test('a taken slug arrives as CONFLICT and points at the slug field', async () => {
  const cause = await createArticle({}, {
    fetchImpl: async () => json({
      error: { code: 'CONFLICT', message: '这个 slug 已经被别的文章用了', field: 'slug' },
    }, 409),
  }).catch(error => error);

  assert.equal(cause.code, codes.conflict);
  assert.equal(cause.field, 'slug');
});

test('delete treats 204 as success and encodes the id', async () => {
  const { seen, fetchImpl } = capture(null, 204);
  assert.equal(await deleteArticle('abcd2345', { fetchImpl }), null);
  assert.equal(seen.url, '/api/v1/admin/articles/abcd2345');
  assert.equal(seen.options.method, 'DELETE');
});

test('the upload posts a form and never sets its own Content-Type', async () => {
  const uploaded = { url: 'https://cdn.example.com/articles/ab/abcd.png', key: 'articles/ab/abcd.png', bytes: 1024, contentType: 'image/png' };
  const { seen, fetchImpl } = capture(uploaded, 201);
  const result = await uploadImage(new Blob([new Uint8Array([1])], { type: 'image/png' }), { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/uploads');
  assert.equal(seen.options.method, 'POST');
  assert.ok(seen.options.body instanceof FormData);
  assert.equal(seen.options.body.get('file').type, 'image/png');
  // 写死 boundary 会让服务端解不出表单，所以这个头必须不在。
  assert.equal(seen.options.headers['Content-Type'], undefined);
  assert.equal(seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
  assert.equal(result.key, 'articles/ab/abcd.png');
});

test('a malformed upload response is rejected', async () => {
  const cases = [
    { url: 'x', key: 'y', bytes: 0, contentType: 'image/png' },
    { url: 'x', key: 'y', bytes: 1.5, contentType: 'image/png' },
    { url: 'x', key: 'y', bytes: 1, contentType: 7 },
    { url: 'x', key: 'y', bytes: 1 },
  ];
  for (const body of cases) {
    await assert.rejects(uploadImage(new Blob([]), { fetchImpl: async () => json(body, 201) }), /数据格式/);
  }
});

test('the upload failure wording says what to do next', () => {
  assert.match(uploadText({ code: codes.payloadTooLarge, offline: false }), /4 MB/);
  assert.match(uploadText({ code: codes.invalidArgument, offline: false }), /SVG/);
  assert.match(uploadText({ code: codes.serviceUnavailable, offline: false }), /OSS_/);
  assert.match(uploadText({ code: codes.internal, offline: false }), /服务暂时不可用/);
  assert.match(uploadText({ offline: true }), /连接不上服务器/);
});

/* 文本域要用真的：setRangeText 是浏览器给的，替身测不出撤销栈那件事。 */
function fakeArea(value, start = value.length, end = start) {
  return {
    value, selectionStart: start, selectionEnd: end, focused: false,
    setRangeText(text, from, to) {
      this.value = this.value.slice(0, from) + text + this.value.slice(to);
      this.selectionStart = this.selectionEnd = from + text.length;
    },
    focus() { this.focused = true; },
  };
}

test('an image lands at the cursor and leaves it after the markdown', () => {
  const area = fakeArea('开头结尾', 2, 2);
  assert.equal(insertAtCursor(area, '![图](u)'), '开头![图](u)结尾');
  assert.equal(area.selectionStart, 2 + '![图](u)'.length);
  assert.equal(area.focused, true);
});

test('a selection is replaced, and a missing textarea is not a crash', () => {
  assert.equal(insertAtCursor(fakeArea('abcdef', 1, 4), 'X'), 'aXef');
  assert.equal(insertAtCursor(fakeArea('尾巴'), 'X'), '尾巴X');
  assert.equal(insertAtCursor(null, 'X'), '');
});

test('alt text comes from the file name and a long url is shortened', () => {
  assert.equal(altFromFileName('屏幕截图 2026-09-28.png'), '屏幕截图 2026-09-28');
  assert.equal(altFromFileName('no-extension'), 'no-extension');
  assert.equal(altFromFileName(''), '');
  assert.equal(imageMarkdown('u', 'a'), '![a](u)');

  const url = `https://cdn.example.com/${'a'.repeat(120)}.png`;
  const short = shortenURL(url);
  assert.ok(short.length < url.length);
  assert.match(short, /…/);
  assert.equal(shortenURL('https://cdn.example.com/a.png'), 'https://cdn.example.com/a.png');
});
