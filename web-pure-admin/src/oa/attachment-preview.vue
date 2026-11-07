<script setup>
import { defineAsyncComponent, onScopeDispose, shallowRef } from 'vue'
import { ElAlert, ElButton, ElDialog } from 'element-plus'
import { useSession } from './session'
const VuePdfEmbed = defineAsyncComponent(() => import('vue-pdf-embed'))
const props = defineProps({ file: { type: Object, required: true } })
const emit = defineEmits(['close'])
const session = useSession()
const controller = new AbortController()
const imageUrl = shallowRef('')
const source = shallowRef(null)
const loading = shallowRef(true)
const error = shallowRef('')
const page = shallowRef(1)
const pageCount = shallowRef(0)
const renderedPages = shallowRef([])
let documentProxy
function release() {
  if (imageUrl.value) URL.revokeObjectURL(imageUrl.value)
  imageUrl.value = ''; source.value = null
  const doc = documentProxy; documentProxy = undefined
  void doc?.destroy().catch(() => {})
}
onScopeDispose(() => { controller.abort(); release() })
function fail() { error.value = '无法显示此附件，可关闭预览后下载原件核对。'; loading.value = false; release() }
async function showPage(number) {
  loading.value = true
  renderedPages.value = []
  try {
    const pdfPage = await documentProxy.getPage(number)
    if (controller.signal.aborted) return
    const viewport = pdfPage.getViewport({ scale: 1 })
    const { width, height } = viewport
    if (![width, height].every(value => Number.isFinite(value) && value >= 10 && value <= 14400) || width * height > 16_777_216 || height / width > 8 || width / height > 8) throw new Error('Page exceeds preview limits')
    page.value = number; renderedPages.value = [number]
  } catch { if (!controller.signal.aborted) fail() }
}
function loaded(doc) {
  if (controller.signal.aborted) { void doc.destroy(); return }
  documentProxy = doc
  if (doc.numPages > 1000) { fail(); return }
  pageCount.value = doc.numPages
  void showPage(1)
}
async function load() {
  try {
    const blob = await session.authorized('/files/' + props.file.id + '/preview', { binary: true, signal: controller.signal })
    if (controller.signal.aborted) return
    if (['image/png', 'image/jpeg'].includes(blob.type)) imageUrl.value = URL.createObjectURL(blob)
    else if (blob.type === 'application/pdf') {
      const data = new Uint8Array(await blob.arrayBuffer())
      if (controller.signal.aborted) return
      // Render bytes only; no document URLs, annotation links, embedded forms or scripting.
      source.value = { data,
        cMapUrl: new URL('./pure-assets/pdf-cmaps/', location.href).href,
        cMapPacked: true,
        standardFontDataUrl: new URL('./pure-assets/pdf-fonts/', location.href).href,
        isEvalSupported: false, enableXfa: false, useWorkerFetch: false, disableAutoFetch: true, stopAtErrors: true, maxImageSize: 16_777_216, verbosity: 0 }
    } else throw new Error('Unsupported preview')
  } catch (cause) {
    if (!controller.signal.aborted) { error.value = cause.status ? cause.message : '无法显示此附件，可关闭预览后下载原件核对。'; loading.value = false }
  }
}
void load()
</script>
<template>
  <ElDialog :model-value="true" top="4vh" :title="'附件预览 · ' + file.originalName" width="min(58rem, calc(100vw - 2rem))" class="oa-attachment-dialog" destroy-on-close @update:model-value="!$event && emit('close')">
    <div class="oa-pure">
      <p class="oa-muted">预览关闭后清除当前内容。PDF 以页面图像展示；如需检索或辅助阅读，可下载原件。</p>
      <ElAlert v-if="error" class="oa-inline-alert" :title="error" type="warning" :closable="false" show-icon />
      <p v-if="loading" class="oa-status" role="status">正在读取附件…</p>
      <div v-if="source && pageCount" class="oa-pagination" aria-label="PDF 分页">
        <ElButton :disabled="page <= 1 || loading" @click="showPage(page - 1)">上一页</ElButton><span>第 {{ page }} / {{ pageCount }} 页</span><ElButton :disabled="page >= pageCount || loading" @click="showPage(page + 1)">下一页</ElButton>
      </div>
      <div v-if="source || imageUrl" class="oa-attachment-preview">
        <img v-if="imageUrl" :src="imageUrl" :alt="file.originalName" @load="loading = false" @error="fail">
        <VuePdfEmbed v-else :source="source" :page="renderedPages" :width="900" :annotation-layer="false" :text-layer="false" @loaded="loaded" @rendered="renderedPages.length && (loading = false)" @loading-failed="fail" @rendering-failed="fail" @password-requested="fail" />
      </div>
    </div>
    <template #footer><ElButton @click="emit('close')">关闭预览</ElButton></template>
  </ElDialog>
</template>
