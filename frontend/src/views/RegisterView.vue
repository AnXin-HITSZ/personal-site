<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { register, resendVerification } from '../api/account.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText, passwordProblem, throttleText } from '../copy.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import NoticeBanner from '../components/NoticeBanner.vue';

const route = useRoute();

const email = ref('');
const password = ref('');
const confirm = ref('');
const failure = ref(null);
const busy = ref(false);
const sent = ref(false);
const resendState = ref('');

const throttled = computed(() => failure.value?.code === codes.tooManyRequests);
const localProblem = ref({ password: '', confirm: '' });

const emailError = computed(() => (failure.value?.field === 'email' ? failure.value.message : ''));
const passwordError = computed(() => {
  const fromServer = failure.value?.code === codes.invalidArgument && failure.value.field === 'password';
  return fromServer ? failure.value.message : localProblem.value.password;
});
const confirmError = computed(() => localProblem.value.confirm);

const banner = computed(() => {
  const cause = failure.value;
  if (!cause) return null;
  if (cause.code === codes.invalidArgument) return null;
  if (cause.code === codes.tooManyRequests) {
    return { title: '注册太频繁', text: throttleText(cause, '这个网络的注册次数用完了，请稍后再试。') };
  }
  return { title: '注册没有完成', text: commonText(cause) };
});

/* 两个口令框本地先对一遍。对不上的话不该发出去——发出去要么被服务端打回来，
   要么更糟：注册成了，但注册的人以为自己输的是另一个口令。 */
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
    await register({ email: email.value, password: password.value });
    sent.value = true;
  } catch (cause) {
    failure.value = describeFailure(cause);
  } finally {
    busy.value = false;
  }
}

async function resend() {
  if (busy.value) return;
  busy.value = true;
  resendState.value = '';
  try {
    await resendVerification(email.value);
    resendState.value = '验证邮件已经发出，同一邮箱每小时最多 3 封。';
  } catch (cause) {
    const described = describeFailure(cause);
    resendState.value = described.code === codes.tooManyRequests
      ? throttleText(described, '重发次数太多，请稍后再试。')
      : commonText(described);
  } finally {
    busy.value = false;
  }
}

onMounted(() => setMetadata({ title: '注册 · Anxin', path: route.fullPath }));
</script>

<template>
  <section v-if="!sent" class="piece row ruled">
    <div class="facts">
      <p><span class="key">下一步</span><br>会收到一封验证信，点里面的链接才算注册完</p>
      <p><span class="key">有效期</span><br>链接 24 小时</p>
    </div>
    <div class="piece-main">
      <h1 class="form-title">注册</h1>

      <NoticeBanner v-if="banner" :title="banner.title">{{ banner.text }}</NoticeBanner>

      <form class="form" novalidate @submit.prevent="submit">
        <FormField
          id="register-mail" v-model="email" label="邮箱" type="email" autocomplete="email"
          hint="验证信会寄到这里" :error="emailError" :disabled="busy || throttled"
        />
        <FormField
          id="register-pass" v-model="password" label="口令" type="password" autocomplete="new-password"
          hint="至少 12 个字符" :error="passwordError" :disabled="busy || throttled"
        />
        <FormField
          id="register-pass2" v-model="confirm" label="再输一遍口令" type="password" autocomplete="new-password"
          :error="confirmError" :disabled="busy || throttled"
        />
        <div class="form-actions">
          <button class="primary" type="submit" :disabled="busy || throttled">{{ busy ? '提交中…' : '注册' }}</button>
        </div>
      </form>

      <p class="form-alt"><span>已经有账号？</span><router-link to="/login">登录</router-link></p>
    </div>
  </section>

  <section v-else class="piece row ruled">
    <div class="facts">
      <p><span class="key">接下来</span><br>点开信里的链接，验证就完成了，然后回来登录</p>
      <p><span class="key">有效期</span><br>链接 24 小时</p>
    </div>
    <div class="piece-main">
      <h1 class="form-title">去邮箱看看</h1>
      <p class="form-lede">如果 {{ email }} 可以注册，验证信已经发出。点开里面的链接，注册就算完成。</p>

      <NoticeBanner tone="done" :live="false">
        没收到？先翻一下垃圾邮件，再确认地址写对了。都不是的话，可以再要一封——同一个邮箱每小时最多 3 封。
      </NoticeBanner>

      <div class="form-actions">
        <button class="pager-btn" type="button" :disabled="busy" @click="resend">重新发送验证邮件</button>
        <router-link class="link-quiet" to="/login">去登录</router-link>
      </div>
      <p v-if="resendState" class="field-hint stall-note">{{ resendState }}</p>
    </div>
  </section>
</template>
