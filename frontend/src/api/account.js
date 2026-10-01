import { config } from '../config.js';
import { CONTRACT_ERROR, request } from './client.js';

/* 会话对象是登录、恢复、刷新三处共用的地基——它形状不对，后面每一页都会拿着
   坏数据往下跑，所以在这一层一次查清。 */
function validateSession(data) {
  const account = data?.account;
  const shaped = account && typeof account.id === 'string' && typeof account.email === 'string' &&
    typeof account.role === 'string' && typeof account.emailVerified === 'boolean' &&
    Number.isFinite(Date.parse(account.createdAt)) && Number.isFinite(Date.parse(data?.expiresAt));
  if (!shaped) throw new Error(CONTRACT_ERROR);
  return data;
}

/* 查完形状只把那串设备递出去：外壳在这里看过一遍就够了，视图不必再看。 */
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
