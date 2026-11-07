<script setup>
import { computed, shallowRef, watch } from 'vue'
import { ElButton as Button } from 'element-plus'
import { useSession } from '../session'
import { useRemote } from '../use-remote'
import PageState from '../remote-state.vue'

const props = defineProps({ modelValue: { type: Object, required: true }, disabled: Boolean })
const emit = defineEmits(['update:modelValue', 'validated'])
const session = useSession()
const types = useRemote(signal => session.authorized('/leave-types', { signal }))
const { result: leaveTypes, loading: typesLoading, error: typesError, load: loadTypes } = types
const previewKey = shallowRef('')
const validation = shallowRef('')
const input = computed(() => Object.fromEntries(['leaveTypeId', 'startDate', 'endDate', 'startHalf', 'endHalf'].map(key => [key, props.modelValue[key]])))
const inputKey = computed(() => JSON.stringify(input.value))
const previewRead = useRemote(signal => session.authorized('/leave-preview', { method: 'POST', data: input.value, signal }))
const { result, loading, error, load } = previewRead
const preview = computed(() => previewKey.value === inputKey.value ? result.value : null)
watch(inputKey, () => { previewRead.cancel(); previewKey.value = ''; result.value = null; error.value = ''; validation.value = '' }, { flush: 'sync' })
watch(preview, value => emit('validated', value), { immediate: true, flush: 'sync' })
void loadTypes()
function update(name, value) { emit('update:modelValue', { ...props.modelValue, [name]: value }) }
async function calculate() {
  if (props.disabled || loading.value) return
  validation.value = ''
  const value = input.value
  if (!value.leaveTypeId || !value.startDate || !value.endDate || value.startDate > value.endDate ||
      (value.startDate === value.endDate && value.startHalf === 'PM' && value.endHalf === 'AM')) {
    validation.value = '请选择请假类型及有效的开始、结束日期和时段。'
    return
  }
  result.value = null
  previewKey.value = inputKey.value
  await load()
}
const issueLabels = { LEAVE_OVERLAP: '与已有请假时段重叠', INSUFFICIENT_LEAVE: '年度可用余额不足' }
</script>

<template>
  <div class="pc-leave-fields">
    <PageState :loading="typesLoading" :error="typesError" @retry="loadTypes" />
    <p v-if="leaveTypes && !typesLoading && !leaveTypes.list.some(item => item.enabled)" class="pc-muted">暂无可用请假类型，请联系管理员。</p>
    <label for="pc-leave-type">请假类型
      <select id="pc-leave-type" :value="modelValue.leaveTypeId" :disabled="disabled || typesLoading" required @change="update('leaveTypeId', Number($event.target.value))">
        <option :value="null" disabled>请选择请假类型</option>
        <option v-for="item in leaveTypes?.list.filter(item => item.enabled) || []" :key="item.id" :value="item.id">{{ item.name }}{{ item.usesBalance ? ' · 扣年度余额' : ' · 不扣余额' }}</option>
      </select>
    </label>
    <div class="pc-leave-dates">
      <label for="pc-field-startDate">开始日期<input id="pc-field-startDate" :value="modelValue.startDate" type="date" required :disabled="disabled" @input="update('startDate', $event.target.value)"></label>
      <label for="pc-start-half">开始时段<select id="pc-start-half" :value="modelValue.startHalf" :disabled="disabled" @change="update('startHalf', $event.target.value)"><option value="AM">上午</option><option value="PM">下午</option></select></label>
      <label for="pc-field-endDate">结束日期<input id="pc-field-endDate" :value="modelValue.endDate" type="date" required :disabled="disabled" @input="update('endDate', $event.target.value)"></label>
      <label for="pc-end-half">结束时段<select id="pc-end-half" :value="modelValue.endHalf" :disabled="disabled" @change="update('endHalf', $event.target.value)"><option value="AM">上午</option><option value="PM">下午</option></select></label>
    </div>
    <label for="pc-field-reason">请假事由<textarea id="pc-field-reason" :value="modelValue.reason" required :disabled="disabled" @input="update('reason', $event.target.value)" /></label>
    <p class="pc-muted">按管理员配置的工作日历计算；默认周一至周五，无内置法定节假日。</p>
    <Button :disabled="disabled || loading" :loading="loading" @click="calculate">预览请假天数</Button>
    <p v-if="validation || error" class="pc-message pc-message-error" role="alert">{{ validation || error }}</p>
    <section v-if="preview" class="pc-card pc-block" aria-label="请假计算预览" aria-live="polite">
      <h3>{{ preview.calculation.leaveTypeName }} · {{ preview.calculation.totalHalfDays / 2 }} 天</h3>
      <p class="pc-muted">{{ preview.calculation.usesBalance ? '共用年度年假额度；提交后预占，最终批准后计入审批已用。' : '此类型不扣年度余额，仍校验时段重叠。' }}</p>
      <ul class="pc-history"><li v-for="year in preview.calculation.years" :key="year.year">{{ year.year }} 年：{{ year.halfDays / 2 }} 天</li></ul>
      <p v-for="balance in preview.balances" :key="balance.year" class="pc-muted">{{ balance.year }} 年可用 {{ balance.remainingHalfDays / 2 }} 天</p>
      <p v-if="!preview.canSubmit" class="pc-message pc-message-error" role="alert">{{ preview.issues.map(issue => issueLabels[issue] || issue).join('；') }}</p>
      <details><summary>查看实际工作时段</summary><ul class="pc-history"><li v-for="day in preview.calculation.days" :key="day.date">{{ day.date }} · {{ day.halves.map(half => half === 'AM' ? '上午' : '下午').join('、') }}</li></ul></details>
    </section>
  </div>
</template>

<style scoped>
.pc-leave-fields { display: grid; gap: .75rem; min-width: 0; }
.pc-leave-fields > label { display: grid; gap: .375rem; }
.pc-leave-dates { display: grid; grid-template-columns: minmax(0, 1fr) minmax(5rem, .55fr); gap: .75rem; }
.pc-leave-dates label { display: grid; gap: .375rem; min-width: 0; }
.pc-leave-fields summary { min-height: 2.75rem; display: list-item; cursor: pointer; padding-block: .5rem; }
</style>
