<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { login, resendVerification } from '../api/account.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText, throttleText } from '../copy.js';
import { safeNext } from '../redirect.js';
import { apply } from '../session.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import NoticeBanner from '../components/NoticeBanner.vue';

const route = useRoute();
const router = useRouter();

const email = ref('');
const password = ref('');
const failure = ref(null);
const busy = ref(false);
const resendState = ref('');

/* 429 之后能做的只有等：服务端按邮箱计数，越点窗口越长，所以限速期间把整张表单
   都停掉，只留「重置口令」那条路——它不经过口令校验，不受这次计数拖累。 */
const throttled = computed(() => failure.value?.code === codes.tooManyRequests);

/* 服务端有意不说是邮箱还是口令不对（说了就能拿来枚举注册过的邮箱），前端也照这个
   口径：只把「口令不对」挂在口令框下面，邮箱框留白——写在邮箱框上等于替服务端
   指认邮箱错了。 */
const passwordError = computed(() =>
  failure.value?.code === codes.invalidCredentials ? '口令不对' : fieldError('password'));
const emailError = computed(() => (failure.value?.code === codes.invalidCredentials ? '' : fieldError('email')));

function fieldError(name) {
  return failure.value?.field === name ? failure.value.message : '';
}

/* 每个错误码一句人话。长话走横幅，字段级的话留给框下面的小字；invalidArgument
   没有横幅，因为那时候该说的已经挂在具体那个框上了，再说一遍是重复。 */
const banner = computed(() => {
  const cause = failure.value;
  if (!cause) return null;

  switch (cause.code) {
    case codes.invalidCredentials:
      return { title: '邮箱或口令不正确', text: '两者之一不对，请重试。' };
    case codes.emailNotVerified:
      return { title: '邮箱尚未验证', text: `${email.value} 的验证还没完成，先点开那封信里的链接。` };
    case codes.accountDisabled:
      return { title: '这个账号已经停用', text: '需要恢复的话，请通过站点上的联系方式说明。' };
    case codes.tooManyRequests:
      return { title: '登录尝试过于频繁', text: throttleText(cause, '这个邮箱的尝试太多了，请稍后再试。') };
    case codes.invalidArgument:
      return null;
    default:
      return { title: '登录失败', text: commonText(cause) };
  }
});

/* 未验证的重发要单独一处：用户在这一屏上最想做的事就是再要一封信。
   用的是表单里的邮箱，所以先把失败原因标出来更重要——邮箱填错就白发了。 */
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

/* 登录成功后回到当初想去的地方：路由守卫把人送来时会把原地址放进 ?next=，
   safeNext 只放行站内路径，别的一律回首页（理由在 redirect.js）。 */
async function submit() {
  busy.value = true;
  failure.value = null;
  resendState.value = '';
  try {
    apply(await login({ email: email.value, password: password.value }));
    await router.replace(safeNext(route.query.next));
  } catch (cause) {
    failure.value = describeFailure(cause);
  } finally {
    busy.value = false;
  }
}

onMounted(() => setMetadata({ title: '登录 · Anxin', path: route.fullPath }));
</script>

<template>
  <section class="piece row ruled">
    <div class="facts">
      <p><span class="key">会话</span><br>登录后 30 天内免登录，可以随时在账号页撤销</p>
      <p><span class="key">邮箱</span><br>没验证过的邮箱登不进来</p>
    </div>
    <div class="piece-main">
      <h1 class="form-title">登录</h1>
      <p class="form-lede">用注册时的邮箱和口令进来。</p>

      <NoticeBanner v-if="banner" :title="banner.title">
        {{ banner.text }}
        <template v-if="failure.code === codes.emailNotVerified" #actions>
          <button class="pager-btn" type="button" :disabled="busy" @click="resend">重新发送验证邮件</button>
          <span class="field-hint">{{ resendState || '没收到？翻一下垃圾邮件。' }}</span>
        </template>
      </NoticeBanner>

      <form class="form" novalidate @submit.prevent="submit">
        <FormField
          id="login-mail" v-model="email" label="邮箱" type="email" autocomplete="email"
          :error="emailError" :disabled="busy || throttled"
        />
        <FormField
          id="login-pass" v-model="password" label="口令" type="password" autocomplete="current-password"
          :error="passwordError" :disabled="busy || throttled"
        />
        <div class="form-actions">
          <button class="primary" type="submit" :disabled="busy || throttled">{{ busy ? '登录中…' : '登录' }}</button>
          <router-link v-if="!throttled" class="link-quiet" to="/forgot-password">忘记口令</router-link>
          <span v-else class="field-hint">等待期间可以先去<router-link class="link-quiet" to="/forgot-password">重置口令</router-link></span>
        </div>
      </form>

      <p class="form-alt"><span>还没有账号？</span><router-link to="/register">注册</router-link></p>
    </div>
  </section>
</template>
