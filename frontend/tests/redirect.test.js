import test from 'node:test';
import assert from 'node:assert/strict';
import { safeNext } from '../src/redirect.js';

test('a same-site path is kept', () => {
  assert.equal(safeNext('/account'), '/account');
  assert.equal(safeNext('/account?tab=devices'), '/account?tab=devices');
  assert.equal(safeNext('/'), '/');
});

/* 这两条是同一个洞的两副面孔：都「以 / 开头」，但浏览器读出来是外站。
   // 是协议相对地址，\ 会被浏览器当成 / 处理。 */
test('protocol-relative and backslash forms do not leave the site', () => {
  assert.equal(safeNext('//evil.example'), '/account');
  assert.equal(safeNext('/\\evil.example'), '/account');
  assert.equal(safeNext('/\\/evil.example'), '/account');
});

test('absolute urls and non-strings fall back', () => {
  assert.equal(safeNext('https://evil.example'), '/account');
  assert.equal(safeNext('account'), '/account');
  assert.equal(safeNext(undefined), '/account');
  assert.equal(safeNext(['/a', '/b']), '/account');
});
