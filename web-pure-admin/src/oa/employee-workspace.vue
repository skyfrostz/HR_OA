<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { listPage, saveAttachment } from './api'
import { employeeNavigation } from './navigation'
import { useRouteWriteLock } from './use-route-write-lock'
import { formatSalary, nameInitial } from './format'
import { useSession } from './session'
import { useRemote } from './use-remote'
import EmployeeForm from './employee-form.vue'
import EmployeeDetail from './employee-detail.vue'
import PageNav from './page-nav.vue'
import RemoteState from './remote-state.vue'

const session = useSession()
const heading = useTemplateRef('heading')
const navigation = employeeNavigation()
const selectedId = shallowRef(navigation.employeeId)
const focusContracts = shallowRef(selectedId.value !== null)
navigation.employeeId = null
const exporting = shallowRef(false)
const exportError = shallowRef('')
const page = shallowRef(1)
const adding = shallowRef(false)
const editingDetail = shallowRef(false)
const locked = computed(() => adding.value || editingDetail.value)
const selfService = computed(() => ['EMPLOYEE', 'APPROVER'].includes(session.account?.user.role))
const list = useRemote((signal) => session.authorized(listPage('/employees', page.value), { signal }))
const { result, loading, error, load } = list

watch([page, selectedId], () => {
  if (selectedId.value === null) void load()
}, { immediate: true })

useRouteWriteLock(locked)

function showList() {
  if (locked.value) return
  selectedId.value = null
  focusContracts.value = false
  void nextTick(() => heading.value?.focus())
}

function openEmployee(id) {
  adding.value = false
  focusContracts.value = false
  selectedId.value = id
}
async function exportEmployees() {
  if (exporting.value || !session.account?.permissions.canExportEmployees) return
  exporting.value = true
  exportError.value = ''
  try {
    const file = await session.authorized('/employees/export.xlsx', { binary: true })
    saveAttachment(file, '员工档案.xlsx')
  } catch (cause) {
    exportError.value = cause.errorCode === 'EXPORT_LIMIT'
      ? '授权范围内超过 5000 条记录，无法导出。'
      : cause.message
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <EmployeeDetail
    v-if="selectedId !== null"
    :employee-id="selectedId"
    :focus-contracts="focusContracts"
    @back="showList"
    @editing="editingDetail = $event"
  />
  <section v-else class="oa-pure oa-employee-page" aria-labelledby="employees-heading">
    <div class="oa-section-head oa-page-heading">
      <div>
        <h1 id="employees-heading" ref="heading" tabindex="-1">员工档案</h1>
        <p class="oa-muted">
          {{ selfService ? '查看本人的档案和劳动合同。' : '查看员工档案与劳动合同台账。' }}
        </p>
      </div>
      <div class="oa-actions">
        <ElButton
          v-if="session.account?.permissions.canExportEmployees"
          :loading="exporting"
          :disabled="adding"
          @click="exportEmployees"
        >导出 Excel</ElButton>
        <ElButton
        v-if="session.canWrite && !adding"
        type="primary"
        :icon="Plus"
        @click="adding = true"
        >新增员工</ElButton>
      </div>
    </div>
    <ElAlert v-if="exportError" :title="exportError" type="error" :closable="false" />

    <EmployeeForm v-if="adding && session.canWrite" @cancel="adding = false" @created="openEmployee" />

    <section class="oa-surface oa-table-panel" aria-labelledby="oa-directory">
      <div class="oa-table-toolbar">
        <div>
          <h2 id="oa-directory">员工名册</h2>
          <span v-if="result && !loading" class="oa-count">共 {{ result.total }} 人</span>
        </div>
        <ElButton :icon="Refresh" :disabled="loading || adding" @click="load">刷新</ElButton>
      </div>
      <RemoteState
        :loading="loading"
        :error="error"
        :empty="Boolean(result && !result.list?.length)"
        empty-text="暂无员工档案。"
        @retry="load"
      />
      <template v-if="!loading && !error && result?.list?.length">
        <div class="oa-table-wrap">
          <table class="oa-table" aria-label="员工列表">
            <thead>
              <tr>
                <th scope="col">姓名 / 工号</th>
                <th scope="col">部门</th>
                <th scope="col" class="oa-number">工龄</th>
                <th v-if="session.canReadSalary" scope="col" class="oa-number">约定工资</th>
                <th scope="col" class="oa-table-action">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="employee in result.list" :key="employee.id">
                <th scope="row">
                  <div class="oa-person-cell">
                    <span class="oa-avatar" aria-hidden="true">{{ nameInitial(employee.name) }}</span>
                    <div>
                      <div class="oa-person-name">{{ employee.name }}</div>
                      <div class="oa-employee-number">{{ employee.employeeNo }}</div>
                    </div>
                  </div>
                </th>
                <td data-label="部门">{{ employee.department }}</td>
                <td class="oa-number" data-label="工龄">{{ employee.tenureYears }} 年</td>
                <td v-if="session.canReadSalary" class="oa-number" data-label="约定工资">{{ formatSalary(employee.salaryCents) }}</td>
                <td class="oa-table-action">
                  <ElButton text type="primary" :disabled="adding" @click="openEmployee(employee.id)">查看档案</ElButton>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <PageNav
          :page="page"
          :total="result.total"
          :page-size="result.pageSize"
          label="员工列表分页"
          :disabled="adding"
          @change="page = $event"
        />
      </template>
    </section>
  </section>
</template>
