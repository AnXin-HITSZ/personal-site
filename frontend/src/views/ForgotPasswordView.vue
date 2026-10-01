<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { requestPasswordReset } from '../api/account.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText, throttleText } from '../copy.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import NoticeBanner from '../components/NoticeBanner.vue';

const route = useRoute();

const email = ref('');
const failure = ref(null);
const busy = ref(false);
const sent = ref(false);

const throttled = computed(() => failure.value?.code === codes.tooManyRequests);
const emailError = computed(() =>
  failure.value?.code === codes.invalidArgument && failure.value.field === 'email' ? failure.value.message : '');

const banner = computed(() => {
  const cause = failure.value;
  if (!cause || cause.code === codes.invalidArgument) return null;
  if (cause.code === codes.tooManyRequests) {
    return { title: '申请太频繁', text: throttleText(cause, '这个邮箱的申请次数太多了，请稍后再试。') };
  }
  return { title: '没能发出重置邮件', text: commonText(cause) };
});

async function submit() {
  busy.value = true;
  failure.value = null;
  try {
    await requestPasswordReset(email.value);
    sent.value = true;
  } catch (cause) {
    failure.value = describeFailure(cause);
  } finally {
    busy.value = false;
  }
}

onMounted(() => setMetadata({ title: '重置口令 · Anxin', path: route.fullPath }));
</script>

<template>
  <section v-if="!sent" class="piece row ruled">
    <div class="facts">
      <p><span class="key">有效期</span><br>重置链接 30 分钟</p>
    </div>
    <div class="piece-main">
      <h1 class="form-title">重置口令</h1>
      <p class="form-lede">填注册时用的邮箱，我们会寄一条重设口令的链接过去。</p>

      <NoticeBanner v-if="banner" :title="banner.title">{{ banner.text }}</NoticeBanner>

      <form class="form" novalidate @submit.prevent="submit">
        <FormField
          id="forgot-mail" v-model="email" label="邮箱" type="email" autocomplete="email"
          :error="emailError" :disabled="busy || throttled"
        />
        <div class="form-actions">
          <button class="primary" type="submit" :disabled="busy || throttled">{{ busy ? '发送中…' : '发送重置链接' }}</button>
        </div>
      </form>

      <p class="form-alt"><span>想起来了？</span><router-link to="/login">去登录</router-link></p>
    </div>
  </section>

  <section v-else class="piece row ruled">
    <div class="facts">
      <p><span class="key">有效期</span><br>30 分钟，过了得重新申请</p>
      <p><span class="key">改完之后</span><br>所有设备都会登出，需要重新登录</p>
    </div>
    <div class="piece-main">
      <h1 class="form-title">去邮箱看看</h1>
      <!-- 「如果有账号」和注册那屏是同一个口径：这个邮箱在不在库里，服务端不回答。 -->
      <p class="form-lede">如果 {{ email }} 有账号，重置口令的链接已经发出，30 分钟内有效。</p>
      <!-- 这里没有「重发」：再要一条和第一次申请是同一个动作，回上一屏重填即可。 -->
      <div class="form-actions">
        <router-link class="pager-btn" to="/login">回登录</router-link>
      </div>
    </div>
  </section>
</template>
