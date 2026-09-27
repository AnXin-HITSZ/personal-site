<script setup>
import { computed } from 'vue';

const props = defineProps({
  title: { type: String, default: '' },
  tone: { type: String, default: 'alert' },
  live: { type: Boolean, default: true },
});

/* 出事了要让读屏立刻念出来（role=alert），办完了等着它念到就行（role=status）——
   前者打断，后者不打断。 */
const role = computed(() => {
  if (!props.live) return undefined;
  return props.tone === 'done' ? 'status' : 'alert';
});
</script>

<template>
  <div class="banner" :class="{ 'banner-done': tone === 'done' }" :role="role">
    <h3 v-if="title">{{ title }}</h3>
    <p v-if="$slots.default"><slot /></p>
    <div v-if="$slots.actions" class="banner-actions"><slot name="actions" /></div>
  </div>
</template>
