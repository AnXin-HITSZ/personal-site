<script setup>
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { changePassword, listSessions, logout, revokeSession } from '../api/account.js';
import { codes, describeFailure } from '../api/client.js';
import { commonText, passwordProblem, roleText } from '../copy.js';
import { clear, session } from '../session.js';
import { formatDate, formatDateTime } from '../format.js';
import { setMetadata } from '../metadata.js';
import FormField from '../components/FormField.vue';
import NoticeBanner from '../components/NoticeBanner.vue';

const route = useRoute();
const router = useRouter();

const currentPassword = ref('');
const password = ref('');
const problem = ref({ current: '', password: '' });
const changed = ref('');
const failing = ref(null);
const busy = ref(false);

const devices = ref([]);
const devicesState = ref('loading');
const revoking = ref('');
const revokeError = ref('');

const account = computed(() => session.account);

const currentError = computed(() => problem.value.current || serverFieldError('currentPassword'));
const passwordError = computed(() => problem.value.password || serverFieldError('password'));

function serverFieldError(name) {
  const cause = failing.value;
  if (cause?.code !== codes.invalidArgument || cause.field !== name) return '';
  return cause.message;
}

/* 自己在服务端被踢掉时把本地状态一并清干净，否则报头上会留着一个不成立的
   「账号」，点进去又是一次 401。 */
async function toLogin() {
  clear();
  await router.replace({ name: 'login', query: { next: route.fullPath } });
}

async function refreshDevices() {
  try {
    devices.value = await listSessions();
    devicesState.value = 'ready';
  } catch (cause) {
    if (cause?.code === codes.unauthorized) return toLogin();
    devicesState.value = 'failed';
  }
}

async function submit() {
  failing.value = null;
  changed.value = '';

  problem.value = {
    current: currentPassword.value ? '' : '请填当前口令',
    password: passwordProblem(password.value) ||
      (password.value === currentPassword.value ? '新口令和现在这个一样' : ''),
  };
  if (problem.value.current || problem.value.password) return;

  busy.value = true;
  try {
    const result = await changePassword({ currentPassword: currentPassword.value, password: password.value });
    changed.value = result?.message || '口令已更新';
    currentPassword.value = '';
    password.value = '';
    /* 其它设备已经被服务端作废了，这张表得重新拉一次才和事实一致。 */
    await refreshDevices();
  } catch (cause) {
    if (cause?.code === codes.unauthorized) return toLogin();
    failing.value = describeFailure(cause);
  } finally {
    busy.value = false;
  }
}

/* 只把那一行从表里去掉，不重新拉整张表——重新拉的话，一旦网络不好，
   人刚从表上认出来的那几台会跟着一起消失。 */
async function revoke(id) {
  revoking.value = id;
  revokeError.value = '';
  try {
    await revokeSession(id);
    devices.value = devices.value.filter(device => device.id !== id);
  } catch (cause) {
    if (cause?.code === codes.unauthorized) return toLogin();
    revokeError.value = commonText(describeFailure(cause));
  } finally {
    revoking.value = '';
  }
}

async function signOut() {
  busy.value = true;
  try {
    await logout();
  } catch {
    /* 退不干净也得退：服务端那边的会话有没有删掉，不该拦住人离开这一页。 */
  } finally {
    busy.value = false;
    await toLogin();
  }
}

onMounted(() => {
  setMetadata({ title: '账号 · Anxin', path: route.fullPath });
  refreshDevices();
});
</script>

<template>
  <section class="piece row ruled">
    <div class="facts">
      <p><span class="key">改口令</span><br>改完其它设备全部登出，当前这台留着</p>
    </div>
    <div class="piece-main">
      <h1 class="piece-title">账号</h1>

      <NoticeBanner v-if="changed" tone="done" :title="changed" />

      <dl v-if="account" class="identity">
        <div>
          <dt>邮箱</dt>
          <dd>{{ account.email }} <span v-if="account.emailVerified" class="badge">已验证</span></dd>
        </div>
        <div><dt>身份</dt><dd>{{ roleText(account.role) }}</dd></div>
        <div><dt>注册于</dt><dd class="date">{{ formatDate(account.createdAt) }}</dd></div>
      </dl>

      <div class="section-head">
        <h2 class="section-title">改口令</h2>
        <p class="section-note">改完其它设备会全部登出，这一台留着。</p>
      </div>

      <NoticeBanner v-if="failing && failing.code !== codes.invalidArgument" title="口令没有改成">
        {{ commonText(failing) }}
      </NoticeBanner>

      <form class="form" novalidate @submit.prevent="submit">
        <FormField
          id="account-current" v-model="currentPassword" label="当前口令" type="password"
          autocomplete="current-password" :error="currentError" :disabled="busy"
        />
        <FormField
          id="account-pass" v-model="password" label="新口令" type="password" autocomplete="new-password"
          hint="至少 12 个字符，且不能和现在这个一样" :error="passwordError" :disabled="busy"
        />
        <div class="form-actions">
          <button class="primary" type="submit" :disabled="busy">{{ busy ? '保存中…' : '保存新口令' }}</button>
          <button class="pager-btn" type="button" :disabled="busy" @click="signOut">退出登录</button>
        </div>
      </form>
    </div>
  </section>

  <section class="section row ruled">
    <p class="facts">{{ devicesState === 'ready' ? `${devices.length} 台` : '' }}</p>
    <div class="main">
      <h2 class="section-title">登录设备</h2>
      <p class="section-note">认不出哪一台，就在那一行上把它登出。改动口令也会全部登出。</p>

      <NoticeBanner v-if="devicesState === 'failed'" title="设备列表没读出来">
        刷新一下这一页再试。
      </NoticeBanner>
      <p v-else-if="devicesState === 'loading'" class="field-hint stall-note">正在读取设备列表…</p>
      <NoticeBanner v-else-if="revokeError" title="这一台没有登出">{{ revokeError }}</NoticeBanner>
    </div>
  </section>

  <div v-if="devicesState === 'ready'" class="devices">
    <article v-for="device in devices" :key="device.id" class="device row ruled">
      <div class="facts">
        <p v-if="device.current" class="mark">本机</p>
        <p><time class="date">{{ formatDateTime(device.createdAt) }}</time></p>
        <p>{{ device.ip }}</p>
      </div>
      <div class="device-main">
        <p class="device-ua">{{ device.userAgent }}</p>
        <p class="device-meta">{{ formatDate(device.expiresAt) }} 过期</p>
        <div v-if="!device.current" class="device-actions">
          <button class="pager-btn" type="button" :disabled="revoking === device.id" @click="revoke(device.id)">
            {{ revoking === device.id ? '正在登出…' : '在这台设备上登出' }}
          </button>
        </div>
      </div>
    </article>
  </div>
</template>
