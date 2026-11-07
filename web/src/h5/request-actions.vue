<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from 'vue'
import { Button } from 'vant'
import { actionLabels, approvalError } from './approval'
import { useIntent } from './use-intent'
const props = defineProps({ requestId: { type: Number, required: true }, operations: { type: Array, default: () => [] }, unavailable: Boolean })
const emit = defineEmits(['locked', 'changed', 'invalidated'])
const action = shallowRef('')
const reason = shallowRef('')
const validation = shallowRef('')
const heading = useTemplateRef('heading')
const controls = useTemplateRef('controls')
const mutation = useIntent(approvalError)
const { busy, retryable, error, failure } = mutation
const locked = computed(() => busy.value || retryable.value)
watch(locked, value => emit('locked', value), { flush: 'sync' })
function begin(value) {
  if (locked.value || props.unavailable) return
  action.value = value
  reason.value = ''
  validation.value = ''
  error.value = ''
  void nextTick(() => value ? heading.value?.focus() : controls.value?.querySelector('button')?.focus())
}
function finish(result) {
  if (result) { const kind = action.value; action.value = ''; emit('changed', kind) }
  else if (failure.value?.status === 403 || ['INVALID_STATE', 'STALE_RECORD', 'NOT_FOUND'].includes(failure.value?.code)) emit('invalidated', error.value)
}
async function submit() {
  if (locked.value || !props.operations.includes(action.value)) return
  const note = reason.value.trim()
  if ((action.value !== 'REMIND' && !note) || [...note].length > 500) { validation.value = '请填写操作原因，最多 500 字。'; return }
  finish(await mutation.submit('/requests/' + props.requestId + '/actions', { action: action.value, reason: note }))
}
async function retry() { finish(await mutation.retry()) }
</script>
<template>
  <section v-if="operations.length" class="h5-block h5-card" aria-label="申请操作">
    <div v-if="!action" ref="controls" class="h5-actions">
      <Button v-for="kind in operations" :key="kind" :disabled="unavailable" plain @click="begin(kind)">{{ actionLabels[kind] }}</Button>
    </div>
    <form v-else class="h5-form" autocomplete="off" @submit.prevent="submit">
      <h3 ref="heading" tabindex="-1">确认{{ actionLabels[action] }}</h3>
      <p class="h5-muted">{{ action === 'CANCEL' ? '撤销已批准请假，按原计算记录冲回审批使用量；保留申请与流水。' : action === 'WITHDRAW' ? '撤回后停止审批并释放本次预占余额。' : '提醒当前审批人处理，仅发送模拟站内通知；每次至少间隔 5 分钟。' }}</p>
      <label :for="'h5-request-reason-' + requestId">操作原因{{ action === 'REMIND' ? '（选填）' : '' }}<textarea :id="'h5-request-reason-' + requestId" v-model="reason" :required="action !== 'REMIND'" :disabled="locked" /></label>
      <p v-if="validation || error" class="h5-message h5-message-error" role="alert">{{ validation || error }}</p>
      <div class="h5-actions">
        <Button v-if="retryable" type="primary" :loading="busy" :disabled="busy" @click="retry">重试原操作</Button>
        <Button v-else type="primary" native-type="submit" :loading="busy" :disabled="busy">确认{{ actionLabels[action] }}</Button>
        <Button :disabled="locked" @click="begin('')">取消</Button>
      </div>
    </form>
  </section>
</template>
