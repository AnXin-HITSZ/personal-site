/* ?next= 是地址栏里的东西，谁都能改。只认站内路径，否则它就是个人人可用的
   跳板：拿我的域名把人送去别处，而地址栏从头到尾显示的都是我。 */
export function safeNext(value, fallback = '/account') {
  if (typeof value !== 'string') return fallback;
  if (!value.startsWith('/')) return fallback;
  // 协议相对地址 //evil.example 也是「以 / 开头」，浏览器会当成外站。
  if (value.startsWith('//') || value.startsWith('/\\')) return fallback;
  return value;
}
