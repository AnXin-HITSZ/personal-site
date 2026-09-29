import { codes, retryAfterText } from './api/client.js';

/* 服务端的 role 是给机器看的（member / admin），页面上要给人看。
   注册一律产生 member，所以「作者」这一档页面上暂时见不到，留着是为了
   某天真的多出一个作者时不至于把 admin 三个字母印在页面上。 */
const roles = Object.freeze({ member: '读者', admin: '作者' });

export const GENERIC = '服务暂时不可用，请稍后再试。';
export const OFFLINE = '连接不上服务器，请检查网络后重试。';

/* 与服务端 auth.MinPasswordRunes / auth.MaxPasswordBytes 对齐。本地先量一遍
   只是为了省一次往返和一次限速计数；说了算的仍是服务端，它说不行就是不行。
   长度按字符数（Go 的 rune），上限按字节（UTF-8）——这两个单位不一样，
   因为「至少 12 个字符」是给中文用户看的，「最多 72 字节」是 bcrypt 的边界。 */
export const passwordRule = Object.freeze({ minRunes: 12, maxBytes: 72 });

export function passwordProblem(value, confirm) {
  if ([...value].length < passwordRule.minRunes) return `口令过短：至少 ${passwordRule.minRunes} 个字符`;
  if (new TextEncoder().encode(value).length > passwordRule.maxBytes) {
    return `口令过长：最多 ${passwordRule.maxBytes} 字节，一个汉字算 3 个`;
  }
  if (confirm !== undefined && value !== confirm) return '两次输入的口令不一致';
  return '';
}

export function roleText(role) {
  return roles[role] ?? role;
}

/* 429 的文案要说得出「多久之后」。服务端在限速器的窗口上给 Retry-After，
   给不出来的时候退回各页自己的说法，不能让人对着「请稍后再试」猜。 */
export function throttleText(failure, fallback) {
  const wait = retryAfterText(failure.retryAfterSeconds);
  return wait ? `尝试次数过多，请 ${wait}。` : fallback;
}

/* 剩下的失败只剩三种可能：没连上、服务端出了事、中间有别的东西坏了。
   对用户来说它们要做的事一样——等一会儿再试，所以用同一句话。 */
export function commonText(failure) {
  return failure.offline ? OFFLINE : GENERIC;
}

/* 插图那四种结果各自说下一步做什么，位置一样，只有话不一样——这样眼睛不用去找。
   「存储没配」说的是下一步，不是「请联系站长」：这一页只有作者进得来，他当然知道
   是说给谁听的。 */
export function uploadText(failure) {
  if (failure.offline) return OFFLINE;
  switch (failure.code) {
    case codes.payloadTooLarge:
      return '图片不能超过 4 MB。先压一下再传，截图存成 PNG 通常最大。';
    case codes.invalidArgument:
      return '这个文件不是图片。只收 JPEG、PNG、GIF 和 WebP——SVG 里面能藏脚本，而图是公开放的，不能收。';
    case codes.serviceUnavailable:
      return '图片存储未配置，无法上传。服务器的 backend/.env 里补上 OSS_ 那六个键，重启后端再来。';
    default:
      return GENERIC;
  }
}
