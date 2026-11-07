<script setup>
import { onScopeDispose, shallowRef } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { saveAttachment } from './api'
import { useSession } from './session'

defineProps({ files: { type: Array, required: true } })
const session = useSession()
const pendingId = shallowRef(null)
const error = shallowRef('')
const controller = new AbortController()

onScopeDispose(() => controller.abort())

async function download(file) {
  if (pendingId.value !== null) return
  pendingId.value = file.id
  error.value = ''
  try {
    const blob = await session.authorized('/files/' + file.id + '/download', {
      binary: true,
      signal: controller.signal
    })
    saveAttachment(blob, file.originalName)
  } catch (cause) {
    if (!controller.signal.aborted) error.value = cause.message
  } finally {
    pendingId.value = null
  }
}
</script>

<template>
  <div class="oa-files">
    <h4>合同附件</h4>
    <p v-if="!files.length" class="oa-muted">暂无附件。</p>
    <ElAlert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <ul v-if="files.length" class="oa-file-list">
      <li v-for="file in files" :key="file.id">
        <span class="oa-file-name">{{ file.originalName }}</span>
        <ElButton
          text
          type="primary"
          :disabled="pendingId !== null"
          :loading="pendingId === file.id"
          :aria-label="'下载 ' + file.originalName"
          @click="download(file)"
        >下载</ElButton>
      </li>
    </ul>
  </div>
</template>
