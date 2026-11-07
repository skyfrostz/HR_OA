<script setup>
import { nextTick, ref, shallowRef, useTemplateRef } from 'vue'
import { ElButton } from 'element-plus'
import { useSession } from '../session'
import { useRemote } from '../use-remote'
import { useVersionedWrite } from '../use-versioned-write'
import RemoteState from '../remote-state.vue'
const emit = defineEmits(['locked'])
const session = useSession()
const listHeading = useTemplateRef('listHeading')
const { result, loading, error: readError, load } = useRemote(signal => session.authorized('/leave-calendar', { signal }))
const days = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']
const draft = ref(null)
const date = shallowRef('')
const working = shallowRef(false)
const validation = shallowRef('')
const feedback = shallowRef('')
const heading = useTemplateRef('heading')
const write = useVersionedWrite()
const { busy, blocked, error } = write
void load()
function begin() { if (!result.value) return; draft.value = JSON.parse(JSON.stringify(result.value)); validation.value = ''; feedback.value = ''; emit('locked', true); void nextTick(() => heading.value?.focus()) }
function close() { draft.value = null; emit('locked', false); void nextTick(() => listHeading.value?.focus()) }
function addException() {
  if (!date.value || draft.value.exceptions.some(item => item.date === date.value)) { validation.value = '请选择尚未设置例外的日期。'; return }
  if (draft.value.exceptions.length >= 1000) { validation.value = '日期例外最多 1000 个。'; return }
  draft.value.exceptions.push({ date: date.value, working: working.value })
  draft.value.exceptions.sort((a,b) => a.date.localeCompare(b.date))
  date.value = ''; validation.value = ''
}
async function save() {
  if (!draft.value.workingWeekdays.length) { validation.value = '请至少选择一个周工作日。'; return }
  const saved = await write.send('/leave-calendar', 'PUT', { version: draft.value.version, workingWeekdays: draft.value.workingWeekdays, exceptions: draft.value.exceptions })
  if (saved) { close(); feedback.value = '工作日历已保存；已有申请保留原计算记录。'; await load() }
}
async function reconcile() { await write.reconcile(load, () => { close(); feedback.value = '已重新读取日历。请核对当前版本与日期例外。' }) }
</script>
<template>
  <section aria-labelledby="calendar-heading">
    <header class="oa-section-head"><div><h2 id="calendar-heading" ref="listHeading" tabindex="-1">工作日历</h2><p class="oa-muted">日期例外优先于周规则；未内置法定节假日。</p></div><ElButton :disabled="Boolean(draft) || loading || !result" @click="begin">编辑工作日历</ElButton></header>
    <RemoteState :loading="loading" :error="readError" @retry="load" />
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <template v-if="result && !draft"><p class="oa-status">版本 {{ result.version }} · 周工作日：{{ result.workingWeekdays.map(day => days[day - 1]).join('、') }}</p><ul class="oa-settings-list"><li v-for="entry in result.exceptions" :key="entry.date"><time :datetime="entry.date">{{ entry.date }}</time><span>{{ entry.working ? '工作日' : '休息日' }}</span></li></ul><p v-if="!result.exceptions.length" class="oa-muted">暂无日期例外。</p></template>
    <form v-if="draft" class="oa-form oa-inline-form" @submit.prevent="save">
      <h3 ref="heading" tabindex="-1">编辑工作日历 · 版本 {{ draft.version }}</h3>
      <fieldset :disabled="busy || Boolean(blocked)"><legend>每周工作日</legend><div class="oa-weekdays"><label v-for="(day, index) in days" :key="day"><input v-model="draft.workingWeekdays" type="checkbox" :value="index + 1">{{ day }}</label></div></fieldset>
      <div class="oa-fields"><label>例外日期<input v-model="date" type="date" :disabled="busy || Boolean(blocked)"></label><label>该日安排<select v-model="working" :disabled="busy || Boolean(blocked)"><option :value="false">休息日</option><option :value="true">工作日</option></select></label></div>
      <ElButton :disabled="busy || Boolean(blocked)" @click="addException">添加日期例外</ElButton>
      <ul class="oa-settings-list"><li v-for="entry in draft.exceptions" :key="entry.date"><span>{{ entry.date }} · {{ entry.working ? '工作日' : '休息日' }}</span><ElButton :disabled="busy || Boolean(blocked)" :aria-label="'删除日期例外 ' + entry.date" @click="draft.exceptions = draft.exceptions.filter(item => item.date !== entry.date)">删除</ElButton></li></ul>
      <p v-if="validation || error" role="alert">{{ validation || error }}</p>
      <div class="oa-actions"><ElButton v-if="blocked" :loading="busy" @click="reconcile">重新读取并核对</ElButton><ElButton v-else type="primary" native-type="submit" :loading="busy">保存工作日历</ElButton><ElButton :disabled="busy || Boolean(blocked)" @click="close">取消</ElButton></div>
    </form>
  </section>
</template>
