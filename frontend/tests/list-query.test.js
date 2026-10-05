import test from 'node:test';
import assert from 'node:assert/strict';
import { ALL } from '../src/api/categories.js';
import { maxListPage, maxSearchRunes } from '../src/api/articles.js';
import { readListQuery, writeListQuery } from '../src/list-query.js';
import { mockCategoryIDs } from '../src/mocks/articles.js';

/* 一份形状正确的地址参数，各条测试在它上面改一处再断言结果。 */
const raw = () => ({ page: '3', q: ' 分页 ', category: mockCategoryIDs.backend });

test('an address without filters reads as the default state', () => {
  assert.deepEqual(readListQuery(), { page: 1, q: '', category: ALL });
  assert.deepEqual(readListQuery({}), { page: 1, q: '', category: ALL });
  // 地址上带的都是字符串；没带的键是 undefined，不是缺失。
  assert.deepEqual(readListQuery({ page: undefined, q: undefined, category: undefined }), { page: 1, q: '', category: ALL });
});

test('a filter address reads back as the same state it was written from', () => {
  assert.deepEqual(readListQuery(raw()), { page: 3, q: '分页', category: mockCategoryIDs.backend });
  // all 是「不筛」的记号，读回来和没带一样。
  assert.deepEqual(readListQuery({ category: ALL }), { page: 1, q: '', category: ALL });
});

test('foreign values in the address fall back to the default, never travel on', () => {
  /* 地址栏谁都能改。坏值在这里退回默认值——往下传的话 normalizeQuery 会当场抛，
     读者看到的是一整页「格式不符合约定」，而错的只是地址里的一段。 */
  for (const page of ['0', '-1', '1.5', 'abc', '', '999999999', ['2', '3'], 'NaN']) {
    assert.equal(readListQuery({ ...raw(), page }).page, 1, `page=${page}`);
  }
  assert.equal(readListQuery({ ...raw(), page: String(maxListPage) }).page, maxListPage);
  for (const category of ['backend', 'Backend', 'backend-1', 'X'.repeat(8), ['all'], 7]) {
    assert.equal(readListQuery({ ...raw(), category }).category, ALL, `category=${category}`);
  }
  for (const q of [['一', '二'], 7, null]) {
    assert.equal(readListQuery({ ...raw(), q }).q, '', `q=${q}`);
  }
  // 长度按字符数（一个汉字算一个），与搜索框和服务端的量法同一把尺子。
  assert.equal(readListQuery({ ...raw(), q: '字'.repeat(maxSearchRunes) }).q, '字'.repeat(maxSearchRunes));
  assert.equal(readListQuery({ ...raw(), q: '字'.repeat(maxSearchRunes + 1) }).q, '');
});

test('the default state writes a clean address', () => {
  assert.deepEqual(writeListQuery({ page: 1, q: '', category: ALL }), {});
  assert.deepEqual(writeListQuery({ page: 2, q: '分页', category: mockCategoryIDs.backend }), { page: '2', q: '分页', category: mockCategoryIDs.backend });
});

test('reading what was written gives the same state back', () => {
  for (const state of [
    { page: 1, q: '', category: ALL },
    { page: 4, q: '检索', category: mockCategoryIDs.ai },
    { page: 2, q: '', category: mockCategoryIDs.notes },
    { page: 1, q: 'Go', category: ALL },
  ]) {
    assert.deepEqual(readListQuery(writeListQuery(state)), state, JSON.stringify(state));
  }
});
