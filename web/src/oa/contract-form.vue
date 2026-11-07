<script setup>
import { reactive, shallowRef, onMounted, useTemplateRef } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { contractStatuses } from './format'
import { useSession } from './session'

const props = defineProps({
  path: { type: String, required: true },
  previous: { type: Object, default: null }
})
const emit = defineEmits(['created', 'cancel'])
const session = useSession()
const heading = useTemplateRef('heading')
onMounted(() => heading.value.focus())
const form = reactive({ contractNo: '', startDate: '', endDate: '', status: 'DRAFT' })
const busy = shallowRef(false)
const error = shallowRef('')

async function submit() {
  if (busy.value || !session.canWrite) return
  const contractNo = form.contractNo.trim()
  const { startDate, endDate, status } = form

  if (!contractNo || !startDate || !endDate || startDate > endDate) {
    error.value = '请填写合同编号和有效日期，结束日期不能早于开始日期。'
    return
  }

  busy.value = true
  error.value = ''
  const data = { contractNo, startDate, endDate, status }
  if (props.previous) data.previousContractId = props.previous.id
  try {
    await session.authorized(props.path, { method: 'POST', data })
    emit('created')
  } catch (cause) {
    error.value = !cause.status || cause.status >= 500
      ? '无法确认合同是否已保存。请核对合同列表后再提交。'
      : cause.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="oa-inline-form" aria-labelledby="contract-form-heading">
    <h3 id="contract-form-heading" ref="heading" tabindex="-1">{{ previous ? '续约合同' : '新建合同' }}</h3>
    <p v-if="previous" class="oa-muted">上一份合同：{{ previous.contractNo }}</p>
    <ElAlert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <form class="oa-form" autocomplete="off" @submit.prevent="submit">
      <div class="oa-fields">
        <label for="contract-number">
          合同编号
          <input id="contract-number" v-model="form.contractNo" required :disabled="busy" />
        </label>
        <label for="contract-start">
          开始日期
          <input id="contract-start" v-model="form.startDate" type="date" required :disabled="busy">
        </label>
        <label for="contract-end">
          结束日期
          <input id="contract-end" v-model="form.endDate" type="date" required :disabled="busy">
        </label>
        <label for="contract-status">
          签约状态
          <select id="contract-status" v-model="form.status" :disabled="busy">
            <option v-for="(status, value) in contractStatuses" :key="value" :value="value">{{ status.label }}</option>
          </select>
        </label>
      </div>
      <div class="oa-actions">
        <ElButton native-type="submit" type="primary" :loading="busy">保存合同</ElButton>
        <ElButton :disabled="busy" @click="emit('cancel')">取消</ElButton>
      </div>
    </form>
  </section>
</template>
