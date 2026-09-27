import test from 'node:test';
import assert from 'node:assert/strict';
import { ApiError, codes, describeFailure, fetchSession, listSessions, login, register, resetPassword, retryAfterText, revokeSession } from '../src/api/account.js';
import { config } from '../src/config.js';

const sessionBody = {
  account: { id: 'u1', email: 'reader@example.com', role: 'member', emailVerified: true, createdAt: '2026-09-27T10:00:00Z' },
  expiresAt: '2026-10-27T10:00:00Z',
};

function json(body, status = 200, headers = {}) {
  return new Response(JSON.stringify(body), { status, headers });
}

function capture(result = null, status = 200) {
  const seen = {};
  return {
    seen,
    fetchImpl: async (url, options) => {
      Object.assign(seen, { url, options });
      return result === null ? new Response(null, { status }) : json(result, status);
    },
  };
}

test('register trims the email and carries the CSRF header', async () => {
  const { seen, fetchImpl } = capture({ message: '如果这个邮箱可以注册，验证邮件已经发出' }, 202);
  await register({ email: '  reader@example.com  ', password: 'correct-horse-battery' }, { fetchImpl });

  assert.equal(seen.url, '/api/v1/auth/register');
  assert.equal(seen.options.method, 'POST');
  assert.equal(seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
  assert.equal(seen.options.headers['Content-Type'], 'application/json');
  assert.equal(seen.options.credentials, 'same-origin');
  assert.deepEqual(JSON.parse(seen.options.body), { email: 'reader@example.com', password: 'correct-horse-battery' });
});

test('reads carry no CSRF header and no body', async () => {
  const { seen, fetchImpl } = capture(sessionBody);
  await fetchSession({ fetchImpl });

  assert.equal(seen.url, '/api/v1/account/session');
  assert.equal(seen.options.method, 'GET');
  assert.equal(seen.options.headers['X-Requested-With'], undefined);
  assert.equal(seen.options.headers['Content-Type'], undefined);
  assert.equal(seen.options.body, undefined);
});

test('a malformed session payload is rejected rather than trusted', async () => {
  await assert.rejects(fetchSession({ fetchImpl: async () => json({ account: { id: 'u1' } }) }), /数据格式/);
  await assert.rejects(fetchSession({ fetchImpl: async () => json({ ...sessionBody, expiresAt: 'soon' }) }), /数据格式/);
  await assert.rejects(fetchSession({ fetchImpl: async () => new Response('{') }), SyntaxError);
});

test('device lists are validated entry by entry', async () => {
  const device = { id: 's1', userAgent: 'Chrome 140 · Windows 11', ip: '127.0.0.1', createdAt: '2026-09-27T10:00:00Z', expiresAt: '2026-10-27T10:00:00Z', current: true };
  const devices = await listSessions({ fetchImpl: async () => json({ sessions: [device] }) });
  assert.equal(devices[0].ip, '127.0.0.1');

  await assert.rejects(listSessions({ fetchImpl: async () => json({ sessions: [{ ...device, current: 'yes' }] }) }), /数据格式/);
  await assert.rejects(listSessions({ fetchImpl: async () => json({}) }), /数据格式/);
});

test('revokeSession encodes the id and treats 204 as success', async () => {
  const { seen, fetchImpl } = capture(null, 204);
  assert.equal(await revokeSession('s/1', { fetchImpl }), null);

  assert.equal(seen.url, '/api/v1/account/sessions/s%2F1');
  assert.equal(seen.options.method, 'DELETE');
  assert.equal(seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
});

test('an error body becomes an ApiError with its code, field and Retry-After', async () => {
  const throttled = await login({ email: 'reader@example.com', password: 'x' }, {
    fetchImpl: async () => json({ error: { code: 'TOO_MANY_REQUESTS', message: '登录尝试过于频繁，请稍后再试' } }, 429, { 'Retry-After': '480' }),
  }).catch(cause => cause);

  assert.ok(throttled instanceof ApiError);
  assert.equal(throttled.code, codes.tooManyRequests);
  assert.equal(throttled.status, 429);
  assert.equal(throttled.retryAfterSeconds, 480);

  const field = await register({ email: 'reader@example.com', password: 'short' }, {
    fetchImpl: async () => json({ error: { code: 'INVALID_ARGUMENT', message: '口令过短：至少 12 个字符', field: 'password' } }, 400),
  }).catch(cause => cause);

  const failure = describeFailure(field);
  assert.equal(failure.code, codes.invalidArgument);
  assert.equal(failure.field, 'password');
  assert.equal(failure.message, '口令过短：至少 12 个字符');
  assert.equal(failure.offline, false);
});

test('a response without an error body falls back instead of reading undefined', async () => {
  const cause = await resetPassword({ token: 't', password: 'x' }, {
    fetchImpl: async () => new Response('<html>502 Bad Gateway</html>', { status: 502 }),
  }).catch(error => error);

  assert.ok(cause instanceof ApiError);
  assert.equal(cause.code, codes.transport);
  assert.equal(cause.status, 502);
  assert.equal(describeFailure(cause).message, '');
});

test('offline failures are told apart from rejected input', () => {
  assert.equal(describeFailure(new TypeError('fetch failed')).offline, true);

  const timeout = new Error('timed out');
  timeout.name = 'TimeoutError';
  assert.equal(describeFailure(timeout).offline, true);

  assert.equal(describeFailure(new ApiError({ code: codes.invalidCredentials, message: '邮箱或口令不正确' })).offline, false);
});

test('rate-limit wording scales from seconds to minutes', () => {
  assert.equal(retryAfterText(0), '');
  assert.equal(retryAfterText(45), '45 秒后再试');
  assert.equal(retryAfterText(60), '1 分钟后再试');
  assert.equal(retryAfterText(480), '8 分钟后再试');
});

/* 服务端给整次发信留了 10 秒，HTTP 写超时 15 秒。客户端如果比它短，就会出现
   「前端已经超时、服务端其实办成了」：用户以为没注册上，再点一次撞上的是限速。 */
test('the mail timeout outlasts the server write timeout', () => {
  assert.ok(config.mailTimeoutMs > 15000);
  assert.ok(config.mailTimeoutMs > config.requestTimeoutMs);
});
