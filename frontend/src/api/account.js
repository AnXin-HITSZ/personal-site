import { config } from '../config.js';

const CONTRACT_ERROR = '服务返回的数据格式不符合约定，请检查接口契约';

/* 服务端的错误码。前端按码决定文案，不按 message —— 码是契约，
   消息是可以改的措辞。唯一的例外见 describeFailure。 */
export const codes = Object.freeze({
  invalidArgument: 'INVALID_ARGUMENT',
  unauthorized: 'UNAUTHORIZED',
  forbidden: 'FORBIDDEN',
  notFound: 'NOT_FOUND',
  internal: 'INTERNAL_ERROR',
  invalidCredentials: 'INVALID_CREDENTIALS',
  emailNotVerified: 'EMAIL_NOT_VERIFIED',
  accountDisabled: 'ACCOUNT_DISABLED',
  invalidToken: 'INVALID_TOKEN',
  tooManyRequests: 'TOO_MANY_REQUESTS',
  // 不是服务端给的：响应不是 JSON，或者没有 error 体。走到这里说明中间有东西坏了。
  transport: 'TRANSPORT_ERROR',
});

export class ApiError extends Error {
  constructor({ code, message, field = '', status = 0, retryAfterSeconds = 0 }) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.field = field;
    this.status = status;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

/* 429 上的秒数。服务端只发秒数形式（HTTP-date 那种写法它不用），
   读不出来就当 0，由文案退回到「请稍后再试」。 */
function readRetryAfter(value) {
  const seconds = Number.parseInt(value ?? '', 10);
  return Number.isSafeInteger(seconds) && seconds > 0 ? seconds : 0;
}

async function toApiError(response) {
  let body = null;
  try {
    body = await response.json();
  } catch {
    // 不是 JSON 就没有可用的错误体，下面走兜底。
  }

  const error = body?.error;
  if (typeof error?.code !== 'string' || typeof error?.message !== 'string') {
    return new ApiError({ code: codes.transport, message: `请求失败（HTTP ${response.status}）`, status: response.status });
  }

  return new ApiError({
    code: error.code,
    message: error.message,
    field: typeof error.field === 'string' ? error.field : '',
    status: response.status,
    retryAfterSeconds: readRetryAfter(response.headers.get('Retry-After')),
  });
}

async function request(path, { method = 'GET', body, timeoutMs = config.requestTimeoutMs, signal, fetchImpl = fetch } = {}) {
  const headers = { Accept: 'application/json' };
  // 改状态的方法都要带这个头。跨站请求发不出自定义头，服务端拿它当第一层
  // CSRF 防线，缺了直接 403——那是个和「没登录」长得完全不一样的失败。
  if (method !== 'GET') headers['X-Requested-With'] = 'XMLHttpRequest';
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  const timeout = AbortSignal.timeout(timeoutMs);
  const response = await fetchImpl(`${config.apiBaseUrl.replace(/\/$/, '')}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    // 会话是不透明的 cookie，跟请求自动走。同源时这是浏览器的默认值，
    // 写出来是因为整条鉴权都押在它上面，不该靠默认值。
    credentials: 'same-origin',
    signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
  });

  if (response.status === 204) return null;
  if (!response.ok) throw await toApiError(response);
  return response.json();
}

function validateSession(data) {
  const account = data?.account;
  const shaped = account && typeof account.id === 'string' && typeof account.email === 'string' &&
    typeof account.role === 'string' && typeof account.emailVerified === 'boolean' &&
    Number.isFinite(Date.parse(account.createdAt)) && Number.isFinite(Date.parse(data?.expiresAt));
  if (!shaped) throw new Error(CONTRACT_ERROR);
  return data;
}

function validateDevices(data) {
  const shaped = Array.isArray(data?.sessions) && data.sessions.every(session =>
    session && typeof session.id === 'string' && typeof session.userAgent === 'string' &&
    typeof session.ip === 'string' && typeof session.current === 'boolean' &&
    Number.isFinite(Date.parse(session.createdAt)) && Number.isFinite(Date.parse(session.expiresAt)));
  if (!shaped) throw new Error(CONTRACT_ERROR);
  return data.sessions;
}

/* 账号没有 mock 数据源：一个假的会话变不出真的 cookie，跑起来只会骗人。
   VITE_DATA_SOURCE=mock 在这里明确失败，而不是安静地什么都不做。 */
function requireHttp() {
  if (config.dataSource !== 'http') throw new Error('账号功能没有 mock 数据源，请设置 VITE_DATA_SOURCE=http');
}

/* 邮箱一律去掉首尾空白再发。服务端也会归一化，但在这一层做掉的额外好处是：
   限速器按邮箱计数，前后多个空格不该算成另一个账号。 */
function mail(value) {
  return String(value ?? '').trim();
}

export async function register({ email, password }, options = {}) {
  requireHttp();
  return request('/auth/register', {
    method: 'POST', body: { email: mail(email), password }, timeoutMs: config.mailTimeoutMs, ...options,
  });
}

export async function verifyEmail(token, options = {}) {
  requireHttp();
  return request('/auth/verify-email', { method: 'POST', body: { token }, ...options });
}

export async function resendVerification(email, options = {}) {
  requireHttp();
  return request('/auth/resend-verification', {
    method: 'POST', body: { email: mail(email) }, timeoutMs: config.mailTimeoutMs, ...options,
  });
}

export async function login({ email, password }, options = {}) {
  requireHttp();
  return validateSession(await request('/auth/login', { method: 'POST', body: { email: mail(email), password }, ...options }));
}

export async function logout(options = {}) {
  requireHttp();
  return request('/auth/logout', { method: 'POST', ...options });
}

export async function requestPasswordReset(email, options = {}) {
  requireHttp();
  return request('/auth/forgot-password', {
    method: 'POST', body: { email: mail(email) }, timeoutMs: config.mailTimeoutMs, ...options,
  });
}

export async function resetPassword({ token, password }, options = {}) {
  requireHttp();
  return request('/auth/reset-password', { method: 'POST', body: { token, password }, ...options });
}

export async function fetchSession(options = {}) {
  requireHttp();
  return validateSession(await request('/account/session', options));
}

export async function changePassword({ currentPassword, password }, options = {}) {
  requireHttp();
  return request('/account/password', { method: 'POST', body: { currentPassword, password }, ...options });
}

export async function listSessions(options = {}) {
  requireHttp();
  return validateDevices(await request('/account/sessions', options));
}

export async function revokeSession(sessionID, options = {}) {
  requireHttp();
  return request(`/account/sessions/${encodeURIComponent(sessionID)}`, { method: 'DELETE', ...options });
}

/* 把任何一次失败收成一种形状，视图不必各写一遍 instanceof 判断。
   offline 为 true 表示请求根本没到服务端，文案要说「没连上」而不是
   「你填错了」——这两件事让人做的动作完全不同。

   只有一个地方用服务端给的 message 而不是自己按码写文案：INVALID_ARGUMENT。
   它带着一个 field，内容是具体的、这里枚举不完的（「口令过短：至少 12 个字符」），
   换一种说法只会更差。 */
export function describeFailure(cause) {
  if (cause instanceof ApiError) {
    return {
      code: cause.code,
      field: cause.field,
      message: cause.code === codes.invalidArgument ? cause.message : '',
      retryAfterSeconds: cause.retryAfterSeconds,
      offline: false,
    };
  }
  return {
    code: '',
    field: '',
    message: '',
    retryAfterSeconds: 0,
    offline: cause instanceof TypeError || cause?.name === 'TimeoutError',
  };
}

/* 限速文案里那个「N 分钟后再试」。服务端给的是秒，小额度下按秒说更好用。 */
export function retryAfterText(seconds) {
  if (!seconds) return '';
  if (seconds < 60) return `${seconds} 秒后再试`;
  return `${Math.ceil(seconds / 60)} 分钟后再试`;
}
