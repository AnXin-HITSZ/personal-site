<script setup>
import { computed, ref } from 'vue';

const props = defineProps({
  id: { type: String, required: true },
  label: { type: String, required: true },
  modelValue: { type: Array, default: () => [] },
  max: { type: Number, default: 10 },
  maxLength: { type: Number, default: 32 },
  error: { type: String, default: '' },
  disabled: Boolean,
});

const emit = defineEmits(['update:modelValue']);

const draft = ref('');
/* 刚撞上重复、或刚删掉一个，提示那行要说一句别的话，说完自己回到规则那句。 */
const note = ref('');
const full = computed(() => props.modelValue.length >= props.max);

const hint = computed(() => {
  if (note.value) return note.value;
  if (full.value) return `到上限了，最多 ${props.max} 个。删掉一个才能换。`;
  return `按 Enter 加一个。最多 ${props.max} 个，每个不超过 ${props.maxLength} 个字符。已加 ${props.modelValue.length} 个。`;
});

const describedBy = computed(() => props.error ? `${props.id}-error` : `${props.id}-hint`);

/* 只按逗号、顿号拆，以及「两边带空格的斜杠」拆。斜杠本身是名字的一部分——
   CI/CD、TCP/IP、LangChain/LangGraph 都得能原样写进去；而公开页上标签正是用
   「空格 斜杠 空格」连起来显示的，所以把那一行整个粘回来，正好还原成原来的
   几个标签，两边是一对互逆的写法。 */
function add(raw) {
  const parts = String(raw).split(/,|，|、|\s+\/\s+/).map(part => part.trim()).filter(Boolean);
  note.value = '';

  const next = [...props.modelValue];
  for (const part of parts) {
    if (next.length >= props.max) break;
    const value = [...part].slice(0, props.maxLength).join('');
    /* 和服务端一样按大小写不敏感去重：Go 和 go 并排显示看不出区别，放进去再被
       服务端悄悄吃掉一个，比当场说一句「已经加过了」更糟。 */
    if (next.some(tag => tag.toLowerCase() === value.toLowerCase())) {
      note.value = `「${value}」已经加过了。`;
      continue;
    }
    next.push(value);
  }

  draft.value = '';
  if (next.length !== props.modelValue.length) emit('update:modelValue', next);
}

function remove(index) {
  note.value = '';
  emit('update:modelValue', props.modelValue.filter((_, at) => at !== index));
}

function keydown(event) {
  if (event.key === 'Enter') {
    /* 这一下必须拦掉，否则敲到一半整页表单就交出去了。 */
    event.preventDefault();
    if (draft.value.trim()) add(draft.value);
  } else if (event.key === 'Backspace' && !draft.value && props.modelValue.length) {
    remove(props.modelValue.length - 1);
  }
}
</script>

<template>
  <div class="field">
    <label class="field-label" :for="id">{{ label }}</label>
    <div class="tags" :class="{ 'tags-full': full }">
      <span v-for="(tag, index) in modelValue" :key="tag" class="tag">
        {{ tag }}<button type="button" :aria-label="`移除标签 ${tag}`" :disabled="disabled" @click="remove(index)">×</button>
      </span>
      <input
        :id="id"
        v-model="draft"
        type="text"
        placeholder="键入后按 Enter"
        autocomplete="off"
        :disabled="full || disabled"
        :aria-describedby="describedBy"
        @keydown="keydown"
        @blur="draft.trim() && add(draft)"
      >
    </div>
    <p v-if="error" :id="`${id}-error`" class="field-error">{{ error }}</p>
    <p v-else :id="`${id}-hint`" class="field-hint">{{ hint }}</p>
  </div>
</template>
