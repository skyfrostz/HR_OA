<script setup>
import { computed, shallowRef, watch } from 'vue'
import { ElButton, ElTag } from 'element-plus'
import { ArrowLeft } from '@element-plus/icons-vue'
import { formatSalary, nameInitial } from './format'
import { useSession } from './session'
import { useRemote } from './use-remote'
import ContractLedger from './contract-ledger.vue'
import RemoteState from './remote-state.vue'
import EmployeeForm from './employee-form.vue'
import AnnualLeave from './annual-leave.vue'

const props = defineProps({
  employeeId: { type: Number, required: true },
  focusContracts: Boolean
})
const emit = defineEmits(['back', 'editing'])
const session = useSession()
const editArea = shallowRef('')
const locked = computed(() => Boolean(editArea.value))
const updated = shallowRef(false)
const { result: employee, loading, error, load } = useRemote((signal) =>
  session.authorized('/employees/' + props.employeeId, { signal })
)

watch(() => props.employeeId, load, { immediate: true })

watch(locked, (value) => emit('editing', value), { flush: 'sync' })

function setEditing(area, value) {
  if (value) editArea.value = area
  else if (editArea.value === area) editArea.value = ''
}

async function employeeUpdated() {
  setEditing('employee', false)
  updated.value = true
  await load()
}
</script>

<template>
  <div class="oa-pure oa-detail">
    <div class="oa-detail-back">
      <ElButton :icon="ArrowLeft" :disabled="locked" @click="emit('back')">返回员工列表</ElButton>
      <span>员工详情</span>
    </div>
    <section class="oa-surface oa-profile" aria-label="员工详情">
      <RemoteState :loading="loading" :error="error" @retry="load" />
      <template v-if="!loading && employee">
        <header class="oa-profile-head">
          <span class="oa-avatar oa-avatar-large" aria-hidden="true">{{ nameInitial(employee.name) }}</span>
          <div>
            <h1 id="employee-heading">{{ employee.name }}</h1>
            <p class="oa-muted">工号 {{ employee.employeeNo }}</p>
          </div>
          <div class="oa-actions oa-profile-controls">
            <ElTag type="info" effect="plain">{{ employee.department }}</ElTag>
            <ElButton v-if="session.canWrite && employee.version" :disabled="locked" @click="setEditing('employee', true)">编辑员工</ElButton>
          </div>
        </header>
        <div class="oa-profile-groups">
          <section aria-labelledby="oa-personal-heading">
            <h2 id="oa-personal-heading">基本信息</h2>
            <dl class="oa-description">
              <div><dt>出生日期</dt><dd>{{ employee.birthDate }}</dd></div>
              <div><dt>年龄</dt><dd>{{ employee.age }} 岁</dd></div>
            </dl>
          </section>
          <section aria-labelledby="oa-employment-heading">
            <h2 id="oa-employment-heading">任职信息</h2>
            <dl class="oa-description">
              <div><dt>入职日期</dt><dd>{{ employee.hireDate }}</dd></div>
              <div><dt>工龄</dt><dd>{{ employee.tenureYears }} 年</dd></div>
              <div v-if="session.canReadSalary && employee.salaryCents !== undefined">
                <dt>约定工资</dt><dd>{{ formatSalary(employee.salaryCents) }}</dd>
              </div>
            </dl>
          </section>
        </div>
        <p v-if="updated" class="oa-status" role="status">员工档案已更新。</p>
        <EmployeeForm
          v-if="editArea === 'employee'"
          :employee="employee"
          @updated="employeeUpdated"
          @cancel="setEditing('employee', false)"
        />
      </template>
    </section>
    <ContractLedger
      v-if="!loading && employee"
      :key="employee.id"
      :employee-id="employee.id"
      :unavailable="Boolean(editArea && editArea !== 'contract')"
      :focus-heading="focusContracts"
      @editing="setEditing('contract', $event)"
    />
    <AnnualLeave
      v-if="!loading && employee"
      :key="'annual-' + employee.id"
      :employee-id="employee.id"
      :unavailable="Boolean(editArea && editArea !== 'leave')"
      @editing="setEditing('leave', $event)"
    />
  </div>
</template>
