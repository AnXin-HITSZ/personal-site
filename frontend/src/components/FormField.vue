<script setup>
import { computed } from 'vue';

const props = defineProps({
  id: { type: String, required: true },
  label: { type: String, required: true },
  type: { type: String, default: 'text' },
  autocomplete: { type: String, default: 'off' },
  placeholder: { type: String, default: '' },
  /* 这一格的字号要按它在别处的体量给（标题输入框要长得像印出来的标题），
     input 在组件里面，外面的 class 落不到它身上，所以另开一个口子。 */
  controlClass: { type: String, default: '' },
  modelValue: { type: String, default: '' },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  /* 要出错时也留着提示的，只有地址里那个名字：它的提示说的不是「怎么填对」，
     而是「改这一下不会弄坏别的东西」——撞名那一瞬间正是人最需要听到这句的时候。 */
  keepHint: Boolean,
  disabled: Boolean,
});

defineEmits(['update:modelValue']);

/* 有错就说错，没错才说提示。两个都挂在 aria-describedby 上会让读屏
   把「至少 12 个字符」和「口令过短」一起念出来，而后者已经够了。 */
const describedBy = computed(() => {
  const ids = [];
  if (props.error) ids.push(`${props.id}-error`);
  if (props.hint && (!props.error || props.keepHint)) ids.push(`${props.id}-hint`);
  return ids.length ? ids.join(' ') : undefined;
});
</script>

<template>
  <div class="field">
    <label class="field-label" :for="id">{{ label }}</label>
    <input
      :id="id"
      class="field-input"
      :class="controlClass"
      :type="type"
      :autocomplete="autocomplete"
      :placeholder="placeholder"
      :value="modelValue"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
      @input="$emit('update:modelValue', $event.target.value)"
    >
    <p v-if="error" :id="`${id}-error`" class="field-error">{{ error }}</p>
    <p v-if="hint && (!error || keepHint)" :id="`${id}-hint`" class="field-hint">{{ hint }}</p>
  </div>
</template>
