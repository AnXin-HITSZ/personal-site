// Vite replaces these values at build time; never put backend secrets in VITE_*.
const env = import.meta.env ?? {};
const dataSource = env.VITE_DATA_SOURCE || 'http';
if (!['mock', 'http'].includes(dataSource)) throw new Error('VITE_DATA_SOURCE 必须为 mock 或 http');
if (env.PROD && dataSource !== 'http') throw new Error('生产构建必须使用真实 HTTP API');

export const config = Object.freeze({
  siteUrl: 'https://anxin-hitsz.com',
  qaUrl: 'https://qa.anxin-hitsz.com',
  dataSource,
  apiBaseUrl: env.VITE_API_BASE_URL || '/api/v1',
  requestTimeoutMs: 8000,
});
