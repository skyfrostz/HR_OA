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
import WeComBinding from './wecom-binding.vue'
import EmployeeAccount from './employee-account.vue'
import AuditHistory from './audit-history.vue'
import AnnualLeave from './annual-leave.vue'

const props = defineProps({
  employeeId: { type: Number, required: true },
  contractId: Number
})
const emit = defineEmits(['back', 'editing', 'directory-changed'])
const session = useSession()
const editArea = shallowRef('')
const locked = computed(() => Boolean(editArea.value))
const updated = shallowRef(false)
const auditRevision = shallowRef(0)
const employment = shallowRef('')
const group = shallowRef('contracts')
const groups = computed(() => [
  { key: 'contracts', title: '劳动合同' },
  ...(session.account?.user.role === 'HR_ADMIN' ? [{ key: 'account', title: '员工账号' }, { key: 'wecom', title: '企业微信' }] : []),
  { key: 'leave', title: '年假记录' },
  { key: 'audit', title: '变更历史' }
])
function selectGroup(key) {
  if (!locked.value) group.value = key
}
const employee = shallowRef(null)
const { result: latestEmployee, loading, error, load } = useRemote((signal) =>
  session.authorized('/employees/' + props.employeeId, { signal })
)

watch(latestEmployee, (record) => {
  if (!record) return
  employee.value = record
  employment.value = ''
})
watch(() => props.employeeId, load, { immediate: true })

watch(locked, (value) => emit('editing', value), { flush: 'sync' })

function setEditing(area, value) {
  if (value) editArea.value = area
  else if (editArea.value === area) editArea.value = ''
}

function accountChanged() {
  auditRevision.value++
  emit('directory-changed')
  void load()
}

async function employeeUpdated() {
  setEditing('employee', false)
  emit('directory-changed')
  updated.value = true
  auditRevision.value++
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
      <template v-if="employee">
        <header class="oa-profile-head">
          <span class="oa-avatar oa-avatar-large" aria-hidden="true">{{ nameInitial(employee.name) }}</span>
          <div>
            <h1 id="employee-heading">{{ employee.name }}</h1>
            <p class="oa-muted">工号 {{ employee.employeeNo }}</p>
          </div>
          <div class="oa-actions oa-profile-controls">
            <ElTag :type="(employment || employee.employmentStatus) === 'OFFBOARDED' ? 'info' : 'success'" effect="plain">{{ (employment || employee.employmentStatus) === 'OFFBOARDED' ? '已离职' : '在职' }}</ElTag>
            <ElTag type="info" effect="plain">{{ employee.department }}</ElTag>
            <ElButton v-if="session.canWrite && employee.version" :disabled="locked || loading || Boolean(error)" @click="setEditing('employee', true)">编辑员工</ElButton>
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
    <nav v-if="employee" class="oa-detail-groups oa-segmented" aria-label="员工详情分组">
      <button v-for="item in groups" :key="item.key" type="button" :aria-pressed="group === item.key"
        :disabled="locked" @click="selectGroup(item.key)">{{ item.title }}</button>
    </nav>
    <ContractLedger
      v-if="employee && group === 'contracts'"
      :key="employee.id"
      :employee-id="employee.id"
      :unavailable="Boolean(editArea && editArea !== 'contract')"
      :contract-id="contractId"
      @changed="auditRevision++"
      @editing="setEditing('contract', $event)"
    />
    <EmployeeAccount v-if="employee && group === 'account' && session.account?.user.role === 'HR_ADMIN'" :employee-id="employee.id"
      :unavailable="Boolean(editArea && editArea !== 'account')" @editing="setEditing('account', $event)"
      @employment="employment = $event" @changed="accountChanged" />
    <WeComBinding v-if="employee && group === 'wecom' && session.account?.user.role === 'HR_ADMIN'" :key="employee.id" :employee-id="employee.id" :unavailable="Boolean(editArea && editArea !== 'wecom')" @editing="setEditing('wecom', $event)" @changed="accountChanged" />
    <AnnualLeave
      v-if="employee && group === 'leave'"
      :key="'annual-' + employee.id"
      :employee-id="employee.id"
      :unavailable="Boolean(editArea && editArea !== 'leave')"
      @editing="setEditing('leave', $event)"
      @changed="auditRevision++"
    />
    <AuditHistory v-if="employee && group === 'audit'" :employee-id="employee.id" :revision="auditRevision" />
  </div>
</template>
