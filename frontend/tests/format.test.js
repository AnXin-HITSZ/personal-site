import test from 'node:test';
import assert from 'node:assert/strict';
import { formatDate, formatDateTime, formatMonthDay } from '../src/format.js';

/* 写作台那一栏是「改于 9 月 28 日」，不是「改于 9/28」——zh-CN 只给月日时排出来的
   正是后者，所以这一格必须自己拼。这条一旦退回 Intl 的默认写法，列表和编辑台会同时
   变成数值日期，而那时候没人会想到是这里。 */
test('the writing desk date is spelled out, not written as numbers', () => {
  assert.equal(formatMonthDay('2026-09-28T02:00:00Z'), '9 月 28 日');
  assert.equal(formatMonthDay('2026-01-05T02:00:00Z'), '1 月 5 日');
  assert.equal(formatMonthDay('2026-12-31T02:00:00Z'), '12 月 31 日');
});

/* 全站按北京时间说日子。UTC 的 16:00 已是第二天，按 UTC 排会少一天。 */
test('dates are read in Beijing time, not UTC', () => {
  assert.equal(formatMonthDay('2026-09-27T16:30:00Z'), '9 月 28 日');
  assert.equal(formatDate('2026-09-27T16:30:00Z'), '2026.09.28');
  assert.equal(formatDateTime('2026-09-27T16:30:00Z'), '2026.09.28 00:30');
});

test('the dotted forms keep their shape', () => {
  assert.equal(formatDate('2026-09-28T02:00:00Z'), '2026.09.28');
  assert.equal(formatDateTime('2026-09-28T02:00:00Z'), '2026.09.28 10:00');
});
