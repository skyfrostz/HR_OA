<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { saveAttachment } from './api'
import { listDepartments } from './business'
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
const pageSize = shallowRef(20)
const query = shallowRef('')
const departmentId = shallowRef(null)
const applied = shallowRef({ query: '', departmentId: null })
const filterError = shallowRef('')
const canFilterDepartment = computed(() => ['HR_ADMIN', 'HR_VIEWER'].includes(session.account?.user.role))
const departments = useRemote((signal) => listDepartments(session.authorized, signal))
if (canFilterDepartment.value) void departments.load()
const adding = shallowRef(false)
const editingDetail = shallowRef(false)
const locked = computed(() => adding.value || editingDetail.value)
const selfService = computed(() => ['EMPLOYEE', 'APPROVER'].includes(session.account?.user.role))
const list = useRemote((signal) => session.authorized('/employees/search', {
  method: 'POST', data: { ...applied.value, page: page.value, pageSize: pageSize.value }, signal
}))
const { result, loading, error, load } = list

watch([page, pageSize, applied, selectedId], () => {
  if (selectedId.value === null) void load()
}, { immediate: true })

useRouteWriteLock(locked)

function applyFilters() {
  if (adding.value) return
  const normalized = query.value.trim()
  if ([...normalized].length > 100) {
    filterError.value = '搜索内容不能超过 100 个字符。'
    return
  }
  filterError.value = ''
  exportError.value = ''
  page.value = 1
  applied.value = { query: normalized, departmentId: canFilterDepartment.value ? departmentId.value : null }
}

function resetFilters() {
  if (adding.value) return
  query.value = ''
  departmentId.value = null
  applyFilters()
}

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
    const file = await session.authorized('/employees/export.xlsx', {
      method: 'POST', data: applied.value, binary: true
    })
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
        >按当前筛选导出</ElButton>
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
      <form class="oa-form oa-directory-filters" autocomplete="off" @submit.prevent="applyFilters">
        <div class="oa-fields">
          <label for="oa-employee-search">
            搜索员工
            <input id="oa-employee-search" v-model="query" type="search" placeholder="姓名、工号或部门名称" :disabled="adding">
          </label>
          <label v-if="canFilterDepartment" for="oa-department-filter">
            部门筛选
            <select id="oa-department-filter" v-model="departmentId" :disabled="adding || departments.loading.value || Boolean(departments.error.value)">
              <option :value="null">全部部门</option>
              <option v-for="department in departments.result.value || []" :key="department.id" :value="department.id">{{ department.name }}</option>
            </select>
          </label>
        </div>
        <ElAlert v-if="filterError" :title="filterError" type="warning" :closable="false" />
        <div v-if="departments.error.value" class="oa-actions">
          <span class="oa-muted" role="status">暂时无法读取部门选项。</span>
          <ElButton :disabled="adding" @click="departments.load">重新加载部门</ElButton>
        </div>
        <div class="oa-actions">
          <div class="oa-actions">
            <ElButton native-type="submit" type="primary" :disabled="adding">搜索</ElButton>
            <ElButton :disabled="adding" @click="resetFilters">清空</ElButton>
          </div>
          <label class="oa-filter" for="oa-employee-page-size">
            每页条数
            <select id="oa-employee-page-size" v-model="pageSize" :disabled="adding" @change="page = 1">
              <option :value="20">20</option>
              <option :value="50">50</option>
              <option :value="100">100</option>
            </select>
          </label>
        </div>
      </form>
      <RemoteState
        :loading="loading"
        :error="error"
        :empty="Boolean(result && !result.list?.length)"
        :empty-text="applied.query || applied.departmentId ? '没有符合当前筛选条件的员工。' : '暂无员工档案。'"
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
      </template>
      <PageNav
          v-if="result"
          :page="page"
          :total="result.total"
          :page-size="result.pageSize"
          label="员工列表分页"
          :disabled="adding || loading"
          @change="page = $event"
        />
    </section>
  </section>
</template>
