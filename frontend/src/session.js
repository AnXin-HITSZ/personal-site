import { computed, reactive, readonly } from 'vue';
import { fetchSession } from './api/account.js';
import { ApiError, codes } from './api/client.js';

/* 全站唯一的登录态。报头那一项、路由守卫、账号页都读它，谁也不各自去问。
   status 的三态是有意的：unknown 是「还没问过」，ready 是「问清楚了」。
   分不开这两件事，就没法在第一次问失败之后知道该不该再问。 */
const state = reactive({ account: null, expiresAt: '', status: 'unknown' });
let pending = null;

export const session = readonly(state);
export const signedIn = computed(() => state.account !== null);
/* 「写作」那一栏和公开分类页里那枚「编辑分类」都读它：作者专属的东西只在作者眼前
   出现，对别人连一个字都不该有——同一个判断也不该有第二份。 */
export const isAuthor = computed(() => state.account?.role === 'admin');

export function apply(view) {
  state.account = view.account;
  state.expiresAt = view.expiresAt;
  state.status = 'ready';
}

export function clear() {
  state.account = null;
  state.expiresAt = '';
  state.status = 'ready';
}

/* 只在状态不明时问一次；同时发起的多个调用共用同一个请求。
   401 是「确实没登录」，其它失败是「问不到」——后者保留 unknown，下次导航
   再问。一次网络抖动不该把已登录的人显示成未登录。 */
export function load({ force = false } = {}) {
  if (pending) return pending;
  if (state.status !== 'unknown' && !force) return Promise.resolve(state.account);

  pending = fetchSession()
    .then(view => {
      apply(view);
      return state.account;
    })
    .catch(cause => {
      if (cause instanceof ApiError && cause.code === codes.unauthorized) {
        clear();
        return null;
      }
      throw cause;
    })
    .finally(() => {
      pending = null;
    });

  return pending;
}
