<script setup>
import { computed, nextTick, reactive, shallowRef, useTemplateRef } from 'vue'
import { ElButton } from 'element-plus'
import { useSession } from '../session'
import { useRemote } from '../use-remote'
import { useVersionedWrite } from '../use-versioned-write'
import RemoteState from '../remote-state.vue'
const emit = defineEmits(['locked'])
const session = useSession()
const listHeading = useTemplateRef('listHeading')
const { result, loading, error: readError, load } = useRemote(signal => session.authorized('/leave-types', { signal }))
const editor = shallowRef(null)
const form = reactive({ name: '', usesBalance: false, enabled: true })
const feedback = shallowRef('')
const validation = shallowRef('')
const title = useTemplateRef('title')
const write = useVersionedWrite()
const { busy, blocked, error } = write
const disabled = computed(() => busy.value || Boolean(blocked.value))
void load()
function begin(row) {
  if (disabled.value) return
  editor.value = row
  Object.assign(form, row === 'new' ? { name: '', usesBalance: false, enabled: true } : row || {})
  validation.value = ''; error.value = ''
  emit('locked', Boolean(row))
  void nextTick(() => row ? title.value?.focus() : listHeading.value?.focus())
}
async function save() {
  const name = form.name.trim()
  if (!name || [...name].length > 60) { validation.value = '请填写 1–60 字的类型名称。'; return }
  const data = { name, usesBalance: form.usesBalance, enabled: form.enabled }
  if (editor.value !== 'new') data.version = editor.value.version
  const saved = await write.send('/leave-types' + (editor.value === 'new' ? '' : '/' + editor.value.id), editor.value === 'new' ? 'POST' : 'PUT', data)
  if (saved) { begin(null); feedback.value = '请假类型已保存，仅影响以后提交的申请。'; await load() }
}
async function reconcile() { await write.reconcile(load, () => { begin(null); feedback.value = '已重新读取。请核对类型列表和版本，确认原操作是否已经完成。' }) }
</script>
<template>
  <section aria-labelledby="leave-types-heading">
    <header class="oa-section-head"><div><h2 id="leave-types-heading" ref="listHeading" tabindex="-1">请假类型</h2><p class="oa-muted">扣余额的类型共用年度年假额度；不为各假种单独分配额度。</p></div><ElButton :disabled="Boolean(editor) || loading" @click="begin('new')">新增类型</ElButton></header>
    <RemoteState :loading="loading" :error="readError" @retry="load" />
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <form v-if="editor" class="oa-form oa-inline-form" @submit.prevent="save">
      <h3 ref="title" tabindex="-1">{{ editor === 'new' ? '新增请假类型' : '编辑请假类型' }}</h3>
      <div class="oa-fields">
        <label>类型名称<input v-model="form.name" required :disabled="disabled"></label>
        <label>余额处理<select v-model="form.usesBalance" :disabled="disabled"><option :value="true">扣余额（共用年度年假额度）</option><option :value="false">不扣余额（共用年度年假额度）</option></select></label>
        <label>使用状态<select v-model="form.enabled" :disabled="disabled"><option :value="true">启用</option><option :value="false">停用</option></select></label>
      </div>
      <p v-if="validation || error" role="alert">{{ validation || error }}</p>
      <div class="oa-actions"><ElButton v-if="blocked" :loading="busy" @click="reconcile">重新读取并核对</ElButton><ElButton v-else type="primary" native-type="submit" :loading="busy">保存类型</ElButton><ElButton :disabled="disabled" @click="begin(null)">取消</ElButton></div>
    </form>
    <ul v-if="result" class="oa-settings-list"><li v-for="row in result.list" :key="row.id"><div><strong>{{ row.name }}</strong><p class="oa-muted">{{ row.usesBalance ? '扣年度余额' : '不扣年度余额' }} · {{ row.enabled ? '已启用' : '已停用' }} · 版本 {{ row.version }}</p></div><ElButton :disabled="Boolean(editor) || loading" @click="begin(row)">编辑{{ row.name }}</ElButton></li></ul>
  </section>
</template>
