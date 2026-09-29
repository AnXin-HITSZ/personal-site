import { onBeforeUnmount, ref, unref, watch } from 'vue';
import { onBeforeRouteLeave, useRouter } from 'vue-router';

/* 离开一页有两条路，机制不一样，两处都要写：
   站内跳转走路由守卫，返回 false 就地拦下，不跳；关标签页、刷新、直接改地址栏
   这三条路由根本管不着，只能挂 beforeunload 交给浏览器自己那一句。
   被拦下之后真的要走，得由人来点一下「放弃改动」——这一下由 discard() 收尾。 */
export function useUnsavedChanges(isDirty) {
  const router = useRouter();
  const blocked = ref(false);
  let pending = '';

  function beforeUnload(event) {
    event.preventDefault();
    /* 现代浏览器不看这句话，只认 preventDefault；旧的一批要有 returnValue 才弹。 */
    event.returnValue = '';
    return '';
  }

  /* 常挂着这个监听，浏览器会在关标签页时无差别地问一句——哪怕什么都没改。
     所以只在真有改动的那段时间里挂着。 */
  watch(() => unref(isDirty), dirty => {
    if (dirty) window.addEventListener('beforeunload', beforeUnload);
    else window.removeEventListener('beforeunload', beforeUnload);
  }, { immediate: true });

  onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));

  onBeforeRouteLeave(to => {
    if (!unref(isDirty)) return true;
    pending = to.fullPath;
    blocked.value = true;
    return false;
  });

  async function discard() {
    const to = pending;
    pending = '';
    blocked.value = false;
    if (to) await router.push(to);
  }

  /* 保存成功之后拦下的那一次就不算数了：人本来就还在这一页，没打算走。 */
  function release() {
    pending = '';
    blocked.value = false;
  }

  return { blocked, discard, release };
}
