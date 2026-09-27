<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { codes, describeFailure, resetPassword } from '../api/account.js';
import { commonText, passwordProblem } from '../copy.js';
import { clear } from '../session.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import NoticeBanner from '../components/NoticeBanner.vue';

const route = useRoute();

const state = ref('form');
const password = ref('');
const confirm = ref('');
const localProblem = ref({ password: '', confirm: '' });
const failure = ref(null);
const busy = ref(false);

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''));

const passwordError = computed(() => {
  const fromServer = failure.value?.code === codes.invalidArgument && failure.value.field === 'password';
  return fromServer ? failure.value.message : localProblem.value.password;
});
const confirmError = computed(() => localProblem.value.confirm);

const banner = computed(() => {
  const cause = failure.value;
  if (!cause || cause.code === codes.invalidArgument) return null;
  return { title: '口令没有改成', text: commonText(cause) };
});

function localCheck() {
  localProblem.value = {
    password: passwordProblem(password.value),
    confirm: password.value === confirm.value ? '' : '两次输入的口令不一致',
  };
  return Boolean(localProblem.value.password || localProblem.value.confirm);
}

async function submit() {
  failure.value = null;
  if (localCheck()) return;

  busy.value = true;
  try {
    await resetPassword({ token: token.value, password: password.value });
    /* 服务端已经把包括这一台在内的所有会话作废了。本地这份状态如果还留着，
       报头上就会挂着一个已经不成立的「账号」。 */
    clear();
    state.value = 'done';
  } catch (cause) {
    failure.value = describeFailure(cause);
    if (cause?.code === codes.invalidToken || cause?.code === codes.notFound) state.value = 'invalid';
  } finally {
    busy.value = false;
  }
}

onMounted(() => {
  setMetadata({ title: '设置新口令 · Anxin', path: route.fullPath });
  if (!token.value) state.value = 'missing';
});
</script>

<template>
  <section v-if="state === 'form'" class="piece row ruled">
    <div class="facts">
      <p><span class="key">有效期</span><br>这条链接 30 分钟</p>
      <p><span class="key">改完之后</span><br>所有设备都要重新登录</p>
    </div>
    <div class="piece-main">
      <h1 class="form-title">设置新口令</h1>
      <p class="form-lede">设一个新口令。设完之后所有设备都要重新登录。</p>

      <NoticeBanner v-if="banner" :title="banner.title">{{ banner.text }}</NoticeBanner>

      <form class="form" novalidate @submit.prevent="submit">
        <FormField
          id="reset-pass" v-model="password" label="新口令" type="password" autocomplete="new-password"
          hint="至少 12 个字符" :error="passwordError" :disabled="busy"
        />
        <FormField
          id="reset-pass2" v-model="confirm" label="再输一遍新口令" type="password" autocomplete="new-password"
          :error="confirmError" :disabled="busy"
        />
        <div class="form-actions">
          <button class="primary" type="submit" :disabled="busy">{{ busy ? '保存中…' : '保存新口令' }}</button>
        </div>
      </form>
    </div>
  </section>

  <section v-else-if="state === 'done'" class="piece row ruled">
    <div class="piece-main">
      <h1 class="form-title">口令已更新</h1>
      <p class="form-lede">所有设备上的登录都已经作废，包括之前在这台机器上的。用新口令重新登录即可。</p>
      <div class="form-actions">
        <router-link class="primary" to="/login">去登录</router-link>
      </div>
    </div>
  </section>

  <section v-else-if="state === 'invalid'" class="piece row ruled">
    <div class="piece-main">
      <h1 class="form-title">这个链接不能用了</h1>
      <p class="form-lede">它可能超过 30 分钟了，也可能已经用过。重新申请一条就行，口令没有被改。</p>
      <div class="form-actions">
        <router-link class="primary" to="/forgot-password">重新申请</router-link>
        <router-link class="link-quiet" to="/login">去登录</router-link>
      </div>
    </div>
  </section>

  <section v-else class="piece row ruled">
    <div class="piece-main">
      <h1 class="form-title">这一页需要从邮件里打开</h1>
      <p class="form-lede">重置链接带着一串一次性的令牌，直接输网址是进不来的。去收件箱里找那封信，或者重新申请一条。</p>
      <div class="form-actions">
        <router-link class="primary" to="/forgot-password">重新申请</router-link>
        <router-link class="link-quiet" to="/login">去登录</router-link>
      </div>
    </div>
  </section>
</template>
