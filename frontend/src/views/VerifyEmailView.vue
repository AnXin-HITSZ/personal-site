<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { codes, describeFailure, resendVerification, verifyEmail } from '../api/account.js';
import { commonText, throttleText } from '../copy.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import NoticeBanner from '../components/NoticeBanner.vue';

const route = useRoute();

const state = ref('pending');
const failure = ref(null);
const email = ref('');
const resendState = ref('');
const busy = ref(false);

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''));

/* 令牌是一次性的。失效的两种情形（过期、已经用过）服务端分得比这里细，
   但对用户来说要做的事一样，所以合并成一屏。 */
async function verify(value) {
  try {
    await verifyEmail(value);
    state.value = 'done';
  } catch (cause) {
    failure.value = describeFailure(cause);
    state.value = cause?.code === codes.invalidToken || cause?.code === codes.notFound ? 'invalid' : 'broken';
  }
}

async function resend() {
  if (busy.value || !email.value) return;
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

onMounted(() => {
  setMetadata({ title: '验证邮箱 · Anxin', path: route.fullPath });
  if (token.value) verify(token.value);
  else state.value = 'missing';
});
</script>

<template>
  <section v-if="state === 'pending'" class="piece row ruled">
    <div class="piece-main">
      <h1 class="form-title">正在验证邮箱</h1>
      <p class="form-lede">稍等，在跟服务端核对这个链接。</p>
      <p class="field-hint stall-note">如果这一步停着不动，<router-link class="link-quiet" to="/login">去登录页重新要一封</router-link>。</p>
    </div>
  </section>

  <section v-else-if="state === 'done'" class="piece row ruled">
    <div class="facts">
      <p><span class="key">会话</span><br>登录后 30 天内免登录</p>
    </div>
    <div class="piece-main">
      <h1 class="form-title">邮箱验证完成</h1>
      <p class="form-lede">这个邮箱已经确认是你的。现在可以登录了。</p>
      <div class="form-actions">
        <router-link class="primary" to="/login">去登录</router-link>
        <router-link class="link-quiet" to="/">回文章列表</router-link>
      </div>
    </div>
  </section>

  <section v-else-if="state === 'invalid'" class="piece row ruled">
    <div class="piece-main">
      <h1 class="form-title">这个链接不能用了</h1>
      <p class="form-lede">它可能超过 24 小时了，也可能在这之前已经用过一次。重新要一封就行。</p>

      <form class="form" novalidate @submit.prevent="resend">
        <FormField id="verify-mail" v-model="email" label="邮箱" type="email" autocomplete="email" :disabled="busy" />
        <div class="form-actions">
          <button class="primary" type="submit" :disabled="busy">{{ busy ? '发送中…' : '重新发送验证邮件' }}</button>
        </div>
      </form>

      <p v-if="resendState" class="field-hint stall-note">{{ resendState }}</p>
      <p class="form-alt"><span>已经验证过了？</span><router-link to="/login">直接登录</router-link></p>
    </div>
  </section>

  <section v-else-if="state === 'broken'" class="piece row ruled">
    <div class="piece-main">
      <h1 class="form-title">没能核对这个链接</h1>
      <NoticeBanner :title="failure?.offline ? '连接不上服务器' : '校验没有完成'">
        {{ commonText(failure ?? { offline: false }) }}
      </NoticeBanner>
      <div class="form-actions">
        <router-link class="primary" to="/login">去登录</router-link>
      </div>
    </div>
  </section>

  <section v-else class="piece row ruled">
    <div class="piece-main">
      <h1 class="form-title">这一页需要从邮件里打开</h1>
      <p class="form-lede">验证链接带着一串一次性的令牌，直接输网址是进不来的。去收件箱里找那封信，或者重新要一封。</p>
      <div class="form-actions">
        <router-link class="primary" to="/login">去登录</router-link>
        <router-link class="link-quiet" to="/">回文章列表</router-link>
      </div>
    </div>
  </section>
</template>
