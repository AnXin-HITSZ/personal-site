import test from 'node:test';
import assert from 'node:assert/strict';
import { ApiError, codes, describeFailure, request, requestMultipart } from '../src/api/client.js';
import { config } from '../src/config.js';

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

function uploadForm() {
  const form = new FormData();
  form.append('file', new Blob([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], { type: 'image/png' }), 'photo.png');
  return form;
}

test('an upload carries the CSRF header but never its own Content-Type', async () => {
  const { seen, fetchImpl } = capture({ url: 'https://cdn.example.com/a.png' }, 201);
  await requestMultipart('/admin/uploads', uploadForm(), { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/uploads');
  assert.equal(seen.options.method, 'POST');
  assert.equal(seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
  assert.equal(seen.options.credentials, 'same-origin');
  // 这一条是整段 multipart 的成败所在：boundary 由浏览器生成，手写 Content-Type
  // 会把 boundary 丢掉，服务端就解不出表单。所以这里断言的是「没有这个头」。
  assert.equal(seen.options.headers['Content-Type'], undefined);
  // 表单要原样交给 fetch，不能被序列化成 JSON。
  assert.ok(seen.options.body instanceof FormData);
});

test('a JSON request keeps the Content-Type the upload must not set', async () => {
  const { seen, fetchImpl } = capture({ ok: true });
  await request('/admin/articles', { method: 'POST', body: { title: '标题' }, fetchImpl });

  assert.equal(seen.options.headers['Content-Type'], 'application/json');
  assert.equal(seen.options.body, '{"title":"标题"}');
});

test('an upload gets the same error conversion as any other request', async () => {
  const cause = await requestMultipart('/admin/uploads', uploadForm(), {
    fetchImpl: async () => json({ error: { code: 'SERVICE_UNAVAILABLE', message: '图片存储未配置，无法上传' } }, 503),
  }).catch(error => error);

  assert.ok(cause instanceof ApiError);
  assert.equal(cause.code, codes.serviceUnavailable);
  assert.equal(cause.status, 503);

  const failure = describeFailure(cause);
  assert.equal(failure.code, codes.serviceUnavailable);
  assert.equal(failure.offline, false);
  // 只有 INVALID_ARGUMENT 会把服务端的措辞透出来，其余由视图按码写。
  assert.equal(failure.message, '');
});

test('an upload carries a deadline instead of hanging forever', async () => {
  const hanging = (_url, options) => new Promise((_, reject) => {
    options.signal.addEventListener('abort', () => reject(options.signal.reason));
  });

  await assert.rejects(
    requestMultipart('/admin/uploads', uploadForm(), { timeoutMs: 20, fetchImpl: hanging }),
    cause => cause.name === 'TimeoutError',
  );
});

/* 三个上限必须层层套住：服务端给 PutObject 10 秒，网关 15 秒掐断，前端要落在
   两者中间——短于服务端就没机会读到真正的失败原因，长于网关则先被判超时。
   15 秒与 10 秒分别抄自 nginx 模板的 proxy_read_timeout 与 uploads.go 的
   putTimeout，改了那边记得回来改这里。 */
test('the upload deadline sits between the server deadline and the gateway', () => {
  assert.ok(config.uploadTimeoutMs > 10000, `上传超时应大于服务端的 10 秒，实际 ${config.uploadTimeoutMs}`);
  assert.ok(config.uploadTimeoutMs < 15000, `上传超时应小于网关的 15 秒，实际 ${config.uploadTimeoutMs}`);
  assert.ok(config.requestTimeoutMs < config.uploadTimeoutMs, '普通请求的超时应短于上传');
});
