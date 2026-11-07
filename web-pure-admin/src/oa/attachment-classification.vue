<script setup>
import { onMounted, shallowRef, useTemplateRef } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { useVersionedWrite } from './use-versioned-write'
const props = defineProps({ file: { type: Object, required: true }, refresh: { type: Function, required: true } })
const emit = defineEmits(['saved', 'cancel', 'reconciled'])
const heading = useTemplateRef('heading')
onMounted(() => heading.value?.focus())
const classification = shallowRef(props.file.classification || 'STANDARD')
const reason = shallowRef('')
const { busy, blocked, error, send, reconcile } = useVersionedWrite()
async function submit() {
  if (!reason.value.trim()) return
  const result = await send('/files/' + props.file.id + '/classification', 'PUT', { classification: classification.value, version: props.file.version, reason: reason.value.trim() })
  if (result) emit('saved')
}
</script>
<template>
  <section class="oa-inline-form" aria-labelledby="oa-classification-heading">
    <h3 id="oa-classification-heading" ref="heading" tabindex="-1">调整附件访问范围</h3><p class="oa-muted">{{ file.originalName }}</p>
    <p class="oa-muted">含工资等敏感原文的附件应设为敏感。普通附件可由获准查看该合同的人事查阅员访问。</p>
    <ElAlert v-if="error" class="oa-inline-alert" :title="error" type="warning" :closable="false" />
    <form class="oa-form" @submit.prevent="submit">
      <div class="oa-fields"><label for="oa-file-classification">访问范围<select id="oa-file-classification" v-model="classification" :disabled="busy || Boolean(blocked)"><option value="RESTRICTED">敏感 · 管理员与合同所属员工</option><option value="STANDARD">普通 · 按合同权限查看</option></select></label>
      <label for="oa-file-reason">调整原因<input id="oa-file-reason" v-model="reason" required maxlength="500" :disabled="busy || Boolean(blocked)"></label></div>
      <div class="oa-actions"><ElButton v-if="blocked" :loading="busy" @click="reconcile(refresh, () => emit('reconciled'))">重新读取并核对</ElButton><ElButton v-else type="primary" native-type="submit" :loading="busy">保存访问范围</ElButton><ElButton :disabled="busy || Boolean(blocked)" @click="emit('cancel')">取消</ElButton></div>
    </form>
  </section>
</template>
