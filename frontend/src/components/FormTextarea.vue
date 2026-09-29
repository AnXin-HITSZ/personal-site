<script setup>
import { computed } from 'vue';

const props = defineProps({
  id: { type: String, required: true },
  label: { type: String, required: true },
  modelValue: { type: String, default: '' },
  rows: { type: Number, default: 3 },
  placeholder: { type: String, default: '' },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  disabled: Boolean,
});

defineEmits(['update:modelValue']);

/* 有错就说错，没错才说提示——理由同 FormField：两个都挂上去，读屏会把
   「写清楚这篇解决什么问题」和「摘要不能为空」一起念出来。 */
const describedBy = computed(() => {
  if (props.error) return `${props.id}-error`;
  return props.hint ? `${props.id}-hint` : undefined;
});
</script>

<template>
  <div class="field">
    <label class="field-label" :for="id">{{ label }}</label>
    <textarea
      :id="id"
      class="field-area"
      :rows="rows"
      :placeholder="placeholder"
      :value="modelValue"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
      @input="$emit('update:modelValue', $event.target.value)"
    ></textarea>
    <p v-if="error" :id="`${id}-error`" class="field-error">{{ error }}</p>
    <p v-else-if="hint" :id="`${id}-hint`" class="field-hint">{{ hint }}</p>
  </div>
</template>
