<script setup>
import { ElAlert, ElButton } from 'element-plus'

defineProps({
  loading: Boolean,
  error: { type: String, default: '' },
  empty: Boolean,
  emptyText: { type: String, default: '暂无记录。' }
})
const emit = defineEmits(['retry'])
</script>

<template>
  <p v-if="loading" role="status" class="oa-status">正在加载…</p>
  <div v-else-if="error" class="oa-error">
    <ElAlert :title="error" type="error" :closable="false" show-icon />
    <ElButton @click="emit('retry')">重新加载</ElButton>
  </div>
  <p v-else-if="empty" class="oa-status">{{ emptyText }}</p>
</template>
