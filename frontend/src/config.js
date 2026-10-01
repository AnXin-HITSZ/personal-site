// Vite replaces these values at build time; never put backend secrets in VITE_*.
const env = import.meta.env ?? {};
/* mock 只允许出现在开发构建里。生产构建要是打成了 mock，页面会拿示例文章当真的显示，
   而且没人会察觉——所以让它在构建时直接失败，而不是等到线上看见假数据。 */
const dataSource = env.VITE_DATA_SOURCE || 'http';
if (!['mock', 'http'].includes(dataSource)) throw new Error('VITE_DATA_SOURCE 必须为 mock 或 http');
if (env.PROD && dataSource !== 'http') throw new Error('生产构建必须使用真实 HTTP API');

/* 这两个对外的地址写死在这里，不走环境变量：它们不随部署的机器变，而 canonical 与项目
   入口都指着它们，配错一次就是给搜索引擎递错地址。 */
export const config = Object.freeze({
  siteUrl: 'https://anxin-hitsz.com',
  qaUrl: 'https://qa.anxin-hitsz.com',
  dataSource,
  apiBaseUrl: env.VITE_API_BASE_URL || '/api/v1',
  // 8 秒：普通接口该比这快得多，而网关 15 秒就会掐断——先由我们自己收场，
  // 界面才说得出是「超时」，而不是笼统的连接失败。
  requestTimeoutMs: 8000,
  // 注册、重发验证信、找回口令这三个接口是同步发信的：用户在那一屏等着 SMTP
  // 连上、认证、投递完。服务端给整次发信留了 10 秒，HTTP 写超时是 15 秒，
  // 这里必须比 15 秒长，否则会出现「前端已经超时、服务端其实办成了」——
  // 用户以为没注册上，再点一次，撞上的却是限速。
  mailTimeoutMs: 20000,
  // 上传是一张图从浏览器到 OSS 的整段。服务端给 PutObject 留了 10 秒，网关 15 秒
  // 会掐断连接，这里取中间：服务端先有机会回一句真的失败原因，又赶在网关之前收场。
  uploadTimeoutMs: 14000,
});
