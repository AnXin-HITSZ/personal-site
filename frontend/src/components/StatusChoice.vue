<script setup>
import { computed } from 'vue';

const props = defineProps({
  id: { type: String, required: true },
  modelValue: { type: String, required: true },
  hint: { type: String, default: '' },
  disabled: Boolean,
});

defineEmits(['update:modelValue']);

/* 状态只有两个值，摊成两段并排，不用下拉：下拉把没选中的那个藏起来，可「草稿」
   在这一站是个正经名字，别处都在拿它说事，藏起来等于给它换个叫法。滑块跟着选中的
   那个走，全是 CSS——这里就是两个 radio。 */
const choices = [
  { value: 'draft', label: '草稿' },
  { value: 'published', label: '已发布' },
];

/* 这一项没有「填错」这回事，提示一直显示，所以它无条件挂在 aria-describedby 上。 */
const describedBy = computed(() => `${props.id}-hint`);
</script>

<template>
  <div class="field">
    <span :id="`${id}-label`" class="field-label">状态</span>
    <div class="status-row">
      <div class="segmented" role="radiogroup" :aria-labelledby="`${id}-label`" :aria-describedby="describedBy">
        <template v-for="choice in choices" :key="choice.value">
          <!-- 圆圈藏在视觉之外、label 显示出来：看上去是分段控件，语义上仍是 radio，
               方向键、读屏和浏览器自带的行为一样都不少。 -->
          <input
            :id="`${id}-${choice.value}`" class="sr-only" type="radio" :name="id"
            :value="choice.value" :checked="modelValue === choice.value" :disabled="disabled"
            @change="$emit('update:modelValue', choice.value)"
          >
          <label :for="`${id}-${choice.value}`">{{ choice.label }}</label>
        </template>
      </div>
      <p :id="`${id}-hint`" class="field-hint">{{ hint }}</p>
    </div>
  </div>
</template>
