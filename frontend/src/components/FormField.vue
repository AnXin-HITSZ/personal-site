<script setup>
import { computed } from 'vue';

const props = defineProps({
  id: { type: String, required: true },
  label: { type: String, required: true },
  type: { type: String, default: 'text' },
  autocomplete: { type: String, default: 'off' },
  modelValue: { type: String, default: '' },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  disabled: Boolean,
});

defineEmits(['update:modelValue']);

/* 有错就说错，没错才说提示。两个都挂在 aria-describedby 上会让读屏
   把「至少 12 个字符」和「口令过短」一起念出来，而后者已经够了。 */
const describedBy = computed(() => {
  if (props.error) return `${props.id}-error`;
  return props.hint ? `${props.id}-hint` : undefined;
});
</script>

<template>
  <div class="field">
    <label class="field-label" :for="id">{{ label }}</label>
    <input
      :id="id"
      class="field-input"
      :type="type"
      :autocomplete="autocomplete"
      :value="modelValue"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
      @input="$emit('update:modelValue', $event.target.value)"
    >
    <p v-if="error" :id="`${id}-error`" class="field-error">{{ error }}</p>
    <p v-else-if="hint" :id="`${id}-hint`" class="field-hint">{{ hint }}</p>
  </div>
</template>
