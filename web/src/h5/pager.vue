<script setup>
import { computed } from 'vue'
import { Button } from 'vant'

const props = defineProps({
  page: { type: Number, required: true },
  total: { type: Number, required: true },
  pageSize: { type: Number, required: true },
  disabled: Boolean
})
const emit = defineEmits(['change'])
const pages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
</script>

<template>
  <nav v-if="pages > 1 || page > 1" class="h5-pager" aria-label="列表分页">
    <Button size="small" plain :disabled="disabled || page <= 1" @click="emit('change', page - 1)">上一页</Button>
    <span>第 {{ page }} / {{ pages }} 页</span>
    <Button size="small" plain :disabled="disabled || page >= pages" @click="emit('change', page + 1)">下一页</Button>
  </nav>
</template>
