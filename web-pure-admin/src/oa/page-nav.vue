<script setup>
import { computed } from 'vue'
import { ElButton } from 'element-plus'

const props = defineProps({
  page: { type: Number, required: true },
  total: { type: Number, required: true },
  pageSize: { type: Number, required: true },
  label: { type: String, required: true },
  disabled: Boolean
})
const emit = defineEmits(['change'])
const pages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
</script>

<template>
  <nav v-if="total" class="oa-pagination" :aria-label="label">
    <ElButton :disabled="disabled || page <= 1" @click="emit('change', page - 1)">上一页</ElButton>
    <span aria-live="polite">第 {{ page }} / {{ pages }} 页 · 共 {{ total }} 条</span>
    <ElButton :disabled="disabled || page >= pages" @click="emit('change', page + 1)">下一页</ElButton>
  </nav>
</template>
