<script setup>
import { ElAlert, ElButton, ElSkeleton, ElSkeletonItem } from 'element-plus'

defineProps({
  loading: Boolean,
  error: { type: String, default: '' },
  empty: Boolean,
  emptyText: { type: String, default: '暂无记录。' }
})
const emit = defineEmits(['retry'])
</script>

<template>
  <div v-if="loading" role="status" class="oa-status oa-skeleton" aria-label="正在加载">
    <ElSkeleton :animated="false">
      <template #template><ElSkeletonItem variant="text" style="width: 45%" /><ElSkeletonItem variant="text" style="width: 72%" /></template>
    </ElSkeleton>
  </div>
  <div v-else-if="error" class="oa-error">
    <ElAlert :title="error" type="error" :closable="false" show-icon />
    <ElButton @click="emit('retry')">重新加载</ElButton>
  </div>
  <p v-else-if="empty" class="oa-status">{{ emptyText }}</p>
</template>
