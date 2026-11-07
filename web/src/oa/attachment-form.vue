<script setup>
import { shallowRef, onMounted, useTemplateRef } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { useSession } from './session'

const props = defineProps({
  contractId: { type: Number, required: true },
  refresh: { type: Function, required: true }
})
const emit = defineEmits(['uploaded', 'cancel'])
const session = useSession()
const heading = useTemplateRef('heading')
onMounted(() => heading.value.focus())
const selectedFile = shallowRef(null)
const busy = shallowRef(false)
const requiresRefresh = shallowRef(false)
const error = shallowRef('')

function selectFile(event) {
  error.value = ''
  const file = event.target.files[0]
  selectedFile.value = null
  if (!file) return
  if (!/\.(pdf|jpe?g|png)$/i.test(file.name)) {
    error.value = '请选择 PDF、JPEG 或 PNG 文件。'
  } else if (file.size > 10 * 1024 * 1024) {
    error.value = '文件超过 10 MiB 上传限制。'
  } else if (!file.size) {
    error.value = '请选择非空文件。'
  } else {
    selectedFile.value = file
  }
  if (!selectedFile.value) event.target.value = ''
}

async function submit() {
  if (busy.value || requiresRefresh.value || !selectedFile.value || !session.canWrite) return
  busy.value = true
  error.value = ''
  try {
    await session.authorized('/contracts/' + props.contractId + '/files', {
      method: 'POST', file: selectedFile.value
    })
    emit('uploaded')
  } catch (cause) {
    requiresRefresh.value = !cause.status || cause.status >= 500
    error.value = requiresRefresh.value
      ? '上传结果未知。请刷新合同并核对附件列表，再决定是否重传。'
      : cause.message
  } finally {
    busy.value = false
  }
}

async function refresh() {
  if (busy.value) return
  busy.value = true
  try {
    if (await props.refresh()) {
      requiresRefresh.value = false
      selectedFile.value = null
      error.value = '已刷新。请核对附件列表，如需重传请重新选择文件。'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="oa-inline-form" aria-labelledby="attachment-form-heading">
    <h4 id="attachment-form-heading" ref="heading" tabindex="-1">添加合同附件</h4>
    <p id="oa-attachment-hint" class="oa-muted">支持 PDF、JPEG、PNG，单个文件最多 10 MiB。</p>
    <ElAlert v-if="error" :title="error" type="warning" :closable="false" show-icon />
    <form class="oa-form" @submit.prevent="submit">
      <label v-if="!requiresRefresh" for="oa-attachment-input">选择文件</label>
      <input
        v-if="!requiresRefresh"
        id="oa-attachment-input"
        type="file"
        accept=".pdf,.jpeg,.jpg,.png"
        aria-describedby="oa-attachment-hint"
        :disabled="busy"
        @change="selectFile"
      >
      <div class="oa-actions">
        <ElButton v-if="requiresRefresh" :loading="busy" @click="refresh">刷新并核对附件</ElButton>
        <ElButton
          v-else
          type="primary"
          native-type="submit"
          :loading="busy"
          :disabled="!selectedFile"
        >上传附件</ElButton>
        <ElButton :disabled="busy || requiresRefresh" @click="emit('cancel')">取消</ElButton>
      </div>
    </form>
  </section>
</template>
