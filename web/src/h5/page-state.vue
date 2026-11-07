<script setup>
import { Button, Loading } from 'vant'

defineProps({
  loading: Boolean,
  error: { type: String, default: '' },
  empty: { type: String, default: '' }
})
const emit = defineEmits(['retry'])
</script>

<template>
  <div v-if="loading" class="h5-card h5-state" role="status">
    <Loading size="20px">正在加载…</Loading>
  </div>
  <div v-else-if="error" class="h5-card h5-state">
    <p class="h5-message h5-message-error" role="alert">{{ error }}</p>
    <Button size="small" plain type="primary" @click="emit('retry')">重新加载</Button>
  </div>
  <p v-else-if="empty" class="h5-card h5-state h5-muted">{{ empty }}</p>
</template>
