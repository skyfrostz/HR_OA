<script setup>
import { onScopeDispose, shallowRef } from 'vue'
import AttachmentPreview from './attachment-preview.vue'
import { ElAlert, ElButton, ElTag } from 'element-plus'
import { saveAttachment } from './api'
import { useSession } from './session'

defineProps({ files: { type: Array, required: true }, unavailable: Boolean })
const emit = defineEmits(['classify'])
const previewFile = shallowRef(null)
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
        <span class="oa-file-name">{{ file.originalName }} <ElTag size="small" effect="plain" :type="file.classification === 'RESTRICTED' ? 'warning' : 'info'">{{ file.classification === 'RESTRICTED' ? '敏感' : '普通' }}</ElTag></span>
        <div class="oa-actions">
        <ElButton text type="primary" :disabled="pendingId !== null || unavailable" :aria-label="'预览 ' + file.originalName" @click="previewFile = file">预览</ElButton>
        <ElButton v-if="session.account?.user.role === 'HR_ADMIN' && file.version" text :disabled="pendingId !== null || unavailable" :aria-label="'调整访问范围 ' + file.originalName" @click="emit('classify', file)">访问范围</ElButton>
        <ElButton
          text
          type="primary"
          :disabled="pendingId !== null || unavailable"
          :loading="pendingId === file.id"
          :aria-label="'下载 ' + file.originalName"
          @click="download(file)"
        >下载</ElButton>
        </div>
      </li>
    </ul>
    <AttachmentPreview v-if="previewFile" :file="previewFile" @close="previewFile = null" />
  </div>
</template>
