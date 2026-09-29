import test from 'node:test';
import assert from 'node:assert/strict';
import { getArticle, listArticles, mockDetail, mockList, normalizeQuery } from '../src/api/articles.js';
import { articles } from '../src/mocks/articles.js';

/* 8 位 base32 的 id 直接写进断言没法读，所以按 slug 取。格式本身另有一条测试钉着，
   取出来的值不会悄悄变成不合法的东西。 */
const idOf = slug => articles.find(a => a.slug === slug).id;

/* mock 必须是真契约的替身：服务端只发 8 位小写 Crockford base32，mock 也得是，
   否则 VITE_DATA_SOURCE=mock 跑出来的「通过」说明不了任何事。 */
test('every mock id is shaped like the one the server hands out', () => {
  for (const article of articles) assert.match(article.id, /^[0-9a-z]{8}$/);
  assert.equal(new Set(articles.map(a => a.id)).size, articles.length);
});

test('default list and pagination retain filtered totals', () => {
  const first = mockList();
  assert.equal(first.items.length, 6);
  assert.equal(first.items[0].id, idOf('go-api-first-step'));
  assert.deepEqual(first.pagination, { page: 1, pageSize: 6, total: 9, totalPages: 2 });
  assert.deepEqual(mockList({ page: 2 }).items.map(a => a.id), ['retrieval-notes', 'go-errors', 'hello-notebook'].map(idOf));
  assert.deepEqual(mockList({ page: 3 }), { items: [], pagination: { page: 3, pageSize: 6, total: 9, totalPages: 2 } });
  assert.equal(mockList({ pageSize: 1 }).pagination.totalPages, 9);
});

test('keyword matching trims whitespace, ignores case, and combines with category', () => {
  assert.deepEqual(mockList({ category: 'backend' }).items.map(a => a.id), ['go-api-first-step', 'pagination-basics', 'go-errors'].map(idOf));
  assert.deepEqual(mockList({ q: ' GO ', category: 'backend' }).items.map(a => a.id), [idOf('go-api-first-step')]);
  assert.equal(mockList({ q: 'GO', category: 'frontend' }).pagination.total, 0);
  assert.equal(mockList({ q: 'JavaScript' }).pagination.total, 0); // tags are not searchable
  assert.equal(mockList({ q: '   ' }).pagination.total, 9);
  assert.deepEqual(mockList({ q: 'not-found' }), { items: [], pagination: { page: 1, pageSize: 6, total: 0, totalPages: 0 } });
});

test('invalid client parameters fail before making a request', () => {
  for (const query of [{ page: 0 }, { page: 1.5 }, { page: 1000001 }, { pageSize: 0 }, { pageSize: 51 }, { category: 'Backend' }, { category: 'backend!' }, { category: '' }, { category: 'x'.repeat(17) }, { q: '字'.repeat(101) }]) {
    assert.throws(() => normalizeQuery(query));
  }
  assert.equal([...normalizeQuery({ q: '😀'.repeat(100) }).q].length, 100);
  /* 形状对就行，存不存在不归前端判：服务端对不存在的分类回一个空列表，那是一个说得
     通的答案（「这个分类下面还没有文章」），不是请求写错了。 */
  assert.equal(normalizeQuery({ category: 'zzzz9999' }).category, 'zzzz9999');
});

test('HTTP adapter serializes contract parameters and validates the result', async () => {
  const query = { page: 1, pageSize: 6, q: ' Go ', category: 'backend' };
  const result = await listArticles(query, { source: 'http', fetchImpl: async (url, options) => {
    assert.equal(url, '/api/v1/articles?page=1&pageSize=6&q=Go&category=backend');
    assert.equal(options.headers.Accept, 'application/json');
    assert.ok(options.signal instanceof AbortSignal);
    return new Response(JSON.stringify(mockList(query)), { status: 200 });
  } });
  assert.equal(result.items[0].id, idOf('go-api-first-step'));
  await listArticles({}, { source: 'http', fetchImpl: async url => {
    assert.equal(url, '/api/v1/articles?page=1&pageSize=6');
    return new Response(JSON.stringify(mockList()));
  } });
});

test('HTTP failures and malformed responses never fall back to mock data', async () => {
  await assert.rejects(listArticles({}, { source: 'http', fetchImpl: async () => new Response('{}', { status: 500 }) }), /HTTP 500/);
  await assert.rejects(listArticles({}, { source: 'http', fetchImpl: async () => new Response('{') }), SyntaxError);
  await assert.rejects(listArticles({}, { source: 'http', fetchImpl: async () => new Response(JSON.stringify({ items: [], pagination: { page: 1, pageSize: 6, total: 9, totalPages: 2 } })) }), /数据格式/);
  const invalid = mockList();
  invalid.items = [{ ...invalid.items[0], publishedAt: 'invalid' }, ...invalid.items.slice(1)];
  await assert.rejects(listArticles({}, { source: 'http', fetchImpl: async () => new Response(JSON.stringify(invalid)) }), /数据格式/);
  await assert.rejects(listArticles({}, { source: 'http', fetchImpl: async () => { throw new TypeError('network failed'); } }), /network failed/);
});

test('cancelled mock request rejects so stale results cannot win', async () => {
  const controller = new AbortController();
  const request = listArticles({}, { source: 'mock', signal: controller.signal });
  controller.abort();
  await assert.rejects(request, { name: 'AbortError' });
});

test('mock detail is keyed by id and carries the body the list withholds', async () => {
  assert.equal(mockList().items[0].body, undefined);
  assert.ok(mockDetail(idOf('go-api-first-step')).body.includes('## '));
  assert.equal(mockDetail('does-not-exist'), null);
  // slug 不再是键，传 slug 进来就查不到——这正是换 id 要钉住的地方。
  assert.equal(mockDetail('go-api-first-step'), null);
  await assert.rejects(getArticle('00000000', { source: 'mock' }), { name: 'ArticleNotFoundError' });
});

test('HTTP detail requests one id and validates the result', async () => {
  const detail = mockDetail(idOf('go-api-first-step'));
  const result = await getArticle(` ${detail.id} `, { source: 'http', fetchImpl: async (url, options) => {
    assert.equal(url, `/api/v1/articles/${detail.id}`);
    assert.equal(options.headers.Accept, 'application/json');
    assert.ok(options.signal instanceof AbortSignal);
    return new Response(JSON.stringify(detail), { status: 200 });
  } });
  assert.equal(result.id, detail.id);
  assert.equal(result.body, detail.body);
});

test('detail failures separate missing articles from broken responses', async () => {
  const detail = mockDetail(idOf('go-api-first-step'));
  const respond = body => getArticle(detail.id, { source: 'http', fetchImpl: async () => new Response(JSON.stringify(body)) });

  await assert.rejects(getArticle(detail.id, { source: 'http', fetchImpl: async () => new Response('{}', { status: 404 }) }), { name: 'ArticleNotFoundError' });
  await assert.rejects(getArticle(detail.id, { source: 'http', fetchImpl: async () => new Response('{}', { status: 500 }) }), /HTTP 500/);
  await assert.rejects(getArticle(detail.id, { source: 'http', fetchImpl: async () => new Response('{') }), SyntaxError);
  await assert.rejects(respond({ ...detail, body: 42 }), /数据格式/);
});

/* slug 和地址里给的那一段对不上是正常的：查库只按 id，详情页会把地址换成规范的那
   一份。所以这一条必须过，而 id 对不上才是契约违规。 */
test('a differing slug is accepted, a differing id is not', async () => {
  const detail = mockDetail(idOf('go-api-first-step'));
  const respond = body => getArticle(detail.id, { source: 'http', fetchImpl: async () => new Response(JSON.stringify(body)) });

  assert.equal((await respond({ ...detail, slug: 'renamed-since' })).slug, 'renamed-since');
  await assert.rejects(respond({ ...detail, id: '00000000' }), /数据格式/);
});

/* 地址里不是 id 的那些值不必跑一趟网络：服务端对不合法的 :id 也是直接 404，两边落到
   同一句话。老的 /articles/<slug> 链接走的正是这条路。 */
test('a malformed id is reported as missing without asking the server', async () => {
  for (const bad of ['go-errors', 'abc', '', '   ', 'ABCDEFGH', '0000000', '000000000']) {
    let called = false;
    await assert.rejects(
      getArticle(bad, { source: 'http', fetchImpl: async () => { called = true; return new Response('{}'); } }),
      { name: 'ArticleNotFoundError' },
    );
    assert.equal(called, false, `${JSON.stringify(bad)} 不该发出请求`);
  }
});

test('cancelled detail request rejects so stale results cannot win', async () => {
  const controller = new AbortController();
  const request = getArticle(idOf('go-api-first-step'), { source: 'mock', signal: controller.signal });
  controller.abort();
  await assert.rejects(request, { name: 'AbortError' });
});

/* 分类名跟着文章一起回来，不从筛选器那份列表里现查：读者那一页的分类那行读不出来
   时，条目上的落款照旧要写得出。所以名字缺席是契约坏了，不是「少了一段」。 */
test('every mock article carries its category name', async () => {
  for (const article of articles) assert.equal(typeof article.categoryName, 'string');
  assert.equal(mockList().items[0].categoryName, '后端开发');

  const row = mockList().items[0];
  const respond = withName => listArticles({ page: 1, pageSize: 1 }, {
    source: 'http',
    fetchImpl: async () => new Response(JSON.stringify({
      items: [{ ...row, categoryName: withName }],
      pagination: { page: 1, pageSize: 1, total: 1, totalPages: 1 },
    })),
  });
  for (const broken of [undefined, '', 7]) {
    await assert.rejects(respond(broken), /数据格式/);
  }
  assert.equal((await respond('后端开发')).items[0].categoryName, '后端开发');
});
