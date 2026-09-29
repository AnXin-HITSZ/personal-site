import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL, CATEGORY_ID_PATTERN, categoryNameProblem, createCategory, deleteCategory,
  isCategoryID, isCategoryRef, listAdminCategories, listCategories, renameCategory, reorderCategories,
} from '../src/api/categories.js';
import { ARTICLE_ID_PATTERN } from '../src/api/articles.js';
import { articles as mockArticles, mockCategories } from '../src/mocks/articles.js';

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

/* 一行形状正确的分类，各条测试在它上面改一处再断言被拒。 */
function row(overrides = {}) {
  return { id: 'backend', name: '后端开发', articleCount: 4, draftCount: 1, ...overrides };
}

const listBody = items => ({ items });

test('every article id is also a legal category id, and not the other way round', () => {
  /* 两边用的是同一个字母表，只是分类 id 没有固定的长度：内置那四个是短代号，
     新加的和文章一样是 8 位随机码。所以约束的方向只有一个——文章 id 全都落得进
     分类 id 那一套里，反过来不成立。 */
  for (const article of mockArticles) {
    assert.match(article.id, ARTICLE_ID_PATTERN);
    assert.ok(isCategoryID(article.id), article.id);
  }
  assert.equal(ARTICLE_ID_PATTERN.test('backend'), false);
  assert.ok(isCategoryID('backend'));

  for (const good of ['a', 'backend', 'zzzz9999', '0'.repeat(16)]) assert.ok(isCategoryID(good), good);
  for (const bad of ['', 'Backend', 'backend-1', 'x'.repeat(17), '分类', 7, null]) assert.equal(isCategoryID(bad), false, String(bad));

  /* all 是个合法的形状，所以「像不像一个分类 id」答不了「这一段是不是分类」。地址参数
     因此单独有一条规则：all 和不筛是一回事，两者都是允许的取值。 */
  assert.equal(isCategoryID(ALL), true);
  assert.equal(isCategoryRef(ALL), true);
  assert.equal(isCategoryRef('zzzz9999'), true);
  assert.equal(isCategoryRef('Backend'), false);
});

test('a name is only required to be non-empty and short enough', () => {
  assert.equal(categoryNameProblem('读书笔记'), '');
  assert.equal(categoryNameProblem('  读书笔记  '), '', '首尾空格由服务端去掉，这里也照那个规矩量');
  assert.equal(categoryNameProblem('字'.repeat(16)), '');
  assert.match(categoryNameProblem('字'.repeat(17)), /16/);
  assert.match(categoryNameProblem(''), /不能为空/);
  assert.match(categoryNameProblem('   '), /不能为空/);
  assert.match(categoryNameProblem(null), /不能为空/);
  // 名字不进任何地址，所以除了长度没有别的字符要求。撞名不在这里查——那是唯一索引
  // 的事，而且它大小写不敏感，前端再比一遍只会比出第二套规则。
  assert.equal(categoryNameProblem('AI 探索！'), '');
});

test('the reader list reads the published categories and shapes the mock the same way', async () => {
  const mocked = await listCategories({ source: 'mock' });
  assert.deepEqual(mocked.items, mockCategories);
  assert.deepEqual(mocked.items.map(item => item.id), ['backend', 'frontend', 'ai', 'notes']);
  // 拿到的必须是副本：调用方改了它，下一次读出来的不该跟着变。
  mocked.items[0].name = '改了';
  assert.equal((await listCategories({ source: 'mock' })).items[0].name, '后端开发');

  const { seen, fetchImpl } = capture(listBody([{ id: 'backend', name: '后端开发' }]));
  const result = await listCategories({ source: 'http', fetchImpl });
  assert.equal(seen.url, '/api/v1/categories');
  assert.equal(seen.options.headers['X-Requested-With'], undefined, '读者那一行没有登录态，不该带写请求的头');
  assert.equal(result.items[0].name, '后端开发');
});

test('a reader row without a usable name is rejected', async () => {
  for (const broken of [{ id: 'backend' }, { name: '后端开发' }, { id: 'Backend', name: 'x' }, { id: 'backend', name: '' }, null]) {
    await assert.rejects(
      listCategories({ source: 'http', fetchImpl: async () => json(listBody([broken])) }),
      /数据格式/,
      JSON.stringify(broken),
    );
  }
  // 一个分类都没有是说得通的（还没写过文章），不是格式坏了。
  const empty = await listCategories({ source: 'http', fetchImpl: async () => json({ items: [] }) });
  assert.deepEqual(empty.items, []);
});

test('the writing list carries the two counts and refuses half of them', async () => {
  const { seen, fetchImpl } = capture(listBody([row()]));
  const result = await listAdminCategories({ fetchImpl });
  assert.equal(seen.url, '/api/v1/admin/categories');
  assert.equal(result.items[0].articleCount, 4);

  for (const broken of [{ ...row(), articleCount: undefined }, { ...row(), draftCount: -1 }, { ...row(), articleCount: 1.5 }]) {
    await assert.rejects(
      listAdminCategories({ fetchImpl: async () => json(listBody([broken])) }),
      /数据格式/,
      JSON.stringify(broken),
    );
  }
  /* 草稿数是「其中」——它比总数大只可能是服务端算错了，但这一条客户端不查：多一个
     前端规则就多一处和服务端对不上的地方。 */
  const odd = await listAdminCategories({ fetchImpl: async () => json(listBody([{ ...row(), draftCount: 9 }])) });
  assert.equal(odd.items[0].draftCount, 9);
});

test('creating and renaming post the name and read the row back', async () => {
  const created = capture(row({ id: 'zzzz9999', name: '读书笔记', articleCount: 0, draftCount: 0 }), 201);
  const made = await createCategory('  读书笔记  ', { fetchImpl: created.fetchImpl });
  assert.equal(created.seen.url, '/api/v1/admin/categories');
  assert.equal(created.seen.options.method, 'POST');
  assert.equal(created.seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
  // 首尾空格留给服务端去（它自己会去），前端不先剪一次——两处各剪一次就是两条规则。
  assert.deepEqual(JSON.parse(created.seen.options.body), { name: '  读书笔记  ' });
  assert.equal(made.id, 'zzzz9999');

  const renamed = capture(row({ name: '服务端' }));
  const after = await renameCategory('backend', '服务端', { fetchImpl: renamed.fetchImpl });
  assert.equal(renamed.seen.url, '/api/v1/admin/categories/backend');
  assert.equal(renamed.seen.options.method, 'PATCH');
  assert.equal(after.name, '服务端');
});

test('the order is submitted whole, and only the ids go up', async () => {
  const { seen, fetchImpl } = capture(null, 204);
  await reorderCategories(['frontend', 'backend', 'notes'], { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/categories/order');
  assert.equal(seen.options.method, 'PUT');
  assert.deepEqual(JSON.parse(seen.options.body), { ids: ['frontend', 'backend', 'notes'] });
});

test('the destination rides in the query, and an empty one is left out', async () => {
  const moved = capture(null, 204);
  await deleteCategory('backend', 'frontend', { fetchImpl: moved.fetchImpl });
  assert.equal(moved.seen.url, '/api/v1/admin/categories/backend?moveTo=frontend');
  assert.equal(moved.seen.options.method, 'DELETE');

  /* 空分类不问去处。带一个空的 moveTo 上去，服务端读到的仍是空串，但地址里多一段
     没有意义的参数，看起来像「有人选了一个空分类」。 */
  const empty = capture(null, 204);
  await deleteCategory('读书笔记', '', { fetchImpl: empty.fetchImpl });
  assert.equal(empty.seen.url, '/api/v1/admin/categories/%E8%AF%BB%E4%B9%A6%E7%AC%94%E8%AE%B0');
});

test('the reader list is the only one with a mock source', async () => {
  /* mock 模式在这儿跑得通，写作那五个方法一律要求 http——假的草稿变不出真的库，
     存下去的东西下次打开就不见了。config 是模块加载时定死的，测试里翻不动它，
     所以这一条只钉住读者那一支在两种数据源下都答得上来。 */
  const mocked = await listCategories({ source: 'mock' });
  assert.equal(mocked.items.length, mockCategories.length);
  const overHttp = await listCategories({ source: 'http', fetchImpl: async () => json(listBody(mockCategories)) });
  assert.deepEqual(overHttp.items, mockCategories);
});
