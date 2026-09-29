<script setup>
import { computed } from 'vue';

const props = defineProps({
  id: { type: String, required: true },
  label: { type: String, required: true },
  modelValue: { type: String, required: true },
  options: { type: Array, required: true },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  disabled: Boolean,
});

defineEmits(['update:modelValue']);

const describedBy = computed(() => {
  if (props.error) return `${props.id}-error`;
  return props.hint ? `${props.id}-hint` : undefined;
});
</script>

<template>
  <div class="field">
    <label class="field-label" :for="id">{{ label }}</label>
    <!-- 外面这一层只是为了放那颗自己画的角标：select 自己画不下伪元素。 -->
    <span class="select">
      <select
        :id="id"
        :value="modelValue"
        :disabled="disabled"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="describedBy"
        @change="$emit('update:modelValue', $event.target.value)"
      >
        <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
    </span>
    <p v-if="error" :id="`${id}-error`" class="field-error">{{ error }}</p>
    <p v-else-if="hint" :id="`${id}-hint`" class="field-hint">{{ hint }}</p>
  </div>
</template>
