import test from 'node:test';
import assert from 'node:assert/strict';
import { listArticles, mockList, normalizeQuery } from '../frontend/src/api/articles.js';

test('default list and pagination retain filtered totals', () => {
  const first = mockList();
  assert.equal(first.items.length, 6);
  assert.equal(first.items[0].id, 'a009');
  assert.deepEqual(first.pagination, { page: 1, pageSize: 6, total: 9, totalPages: 2 });
  assert.deepEqual(mockList({ page: 2 }).items.map(a => a.id), ['a003', 'a002', 'a001']);
  assert.deepEqual(mockList({ page: 3 }), { items: [], pagination: { page: 3, pageSize: 6, total: 9, totalPages: 2 } });
  assert.equal(mockList({ pageSize: 1 }).pagination.totalPages, 9);
});

test('keyword matching trims whitespace, ignores case, and combines with category', () => {
  assert.deepEqual(mockList({ category: 'backend' }).items.map(a => a.id), ['a009', 'a005', 'a002']);
  assert.deepEqual(mockList({ q: ' GO ', category: 'backend' }).items.map(a => a.id), ['a009']);
  assert.equal(mockList({ q: 'GO', category: 'frontend' }).pagination.total, 0);
  assert.equal(mockList({ q: 'JavaScript' }).pagination.total, 0); // tags are not searchable
  assert.equal(mockList({ q: '   ' }).pagination.total, 9);
  assert.deepEqual(mockList({ q: 'not-found' }), { items: [], pagination: { page: 1, pageSize: 6, total: 0, totalPages: 0 } });
});

test('invalid client parameters fail before making a request', () => {
  for (const query of [{ page: 0 }, { page: 1.5 }, { page: 1000001 }, { pageSize: 0 }, { pageSize: 51 }, { category: 'invalid' }, { q: '字'.repeat(101) }]) {
    assert.throws(() => normalizeQuery(query));
  }
  assert.equal([...normalizeQuery({ q: '😀'.repeat(100) }).q].length, 100);
});

test('HTTP adapter serializes contract parameters and validates the result', async () => {
  const query = { page: 1, pageSize: 6, q: ' Go ', category: 'backend' };
  const result = await listArticles(query, { source: 'http', fetchImpl: async (url, options) => {
    assert.equal(url, '/api/v1/articles?page=1&pageSize=6&q=Go&category=backend');
    assert.equal(options.headers.Accept, 'application/json');
    assert.ok(options.signal instanceof AbortSignal);
    return new Response(JSON.stringify(mockList(query)), { status: 200 });
  } });
  assert.equal(result.items[0].id, 'a009');
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
