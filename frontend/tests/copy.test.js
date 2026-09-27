import test from 'node:test';
import assert from 'node:assert/strict';
import { commonText, passwordProblem, passwordRule, roleText, throttleText } from '../src/copy.js';

test('roles are shown in words, and an unknown one is not swallowed', () => {
  assert.equal(roleText('member'), '读者');
  assert.equal(roleText('admin'), '作者');
  assert.equal(roleText('editor'), 'editor');
});

/* 长度按字符数（服务端 auth.MinPasswordRunes），上限按字节（auth.MaxPasswordBytes）。
   两个单位不一样，中文口令最容易踩到：12 个汉字是 36 字节，远在 72 以下，
   所以「过短」和「过长」不会同时成立，这两条各管各的。 */
test('the password rule counts characters for the floor and bytes for the ceiling', () => {
  assert.equal(passwordProblem('a'.repeat(11)), '口令过短：至少 12 个字符');
  assert.equal(passwordProblem('a'.repeat(12)), '');
  assert.equal(passwordProblem('口令'.repeat(6)), '');
  assert.equal(passwordProblem('a'.repeat(73)), '口令过长：最多 72 字节，一个汉字算 3 个');
  assert.match(passwordProblem('密'.repeat(25)), /口令过长/);
});

test('the confirmation is only compared when one was asked for', () => {
  assert.equal(passwordProblem('correct-horse-battery', 'correct-horse-battery'), '');
  assert.equal(passwordProblem('correct-horse-battery', 'correct-horse-batteyr'), '两次输入的口令不一致');
  assert.equal(passwordProblem('correct-horse-battery'), '');
});

test('throttling says how long when the server said, and falls back when it did not', () => {
  const fallback = '这个网络的注册次数用完了，请稍后再试。';
  assert.equal(throttleText({ retryAfterSeconds: 480 }, fallback), '尝试次数过多，请 8 分钟后再试。');
  assert.equal(throttleText({ retryAfterSeconds: 45 }, fallback), '尝试次数过多，请 45 秒后再试。');
  assert.equal(throttleText({ retryAfterSeconds: 0 }, fallback), fallback);
});

test('an unreachable server is not reported as bad input', () => {
  assert.match(commonText({ offline: true }), /连接不上服务器/);
  assert.match(commonText({ offline: false }), /服务暂时不可用/);
});

test('the local floor matches the one the server enforces', () => {
  assert.equal(passwordRule.minRunes, 12);
  assert.equal(passwordRule.maxBytes, 72);
});
