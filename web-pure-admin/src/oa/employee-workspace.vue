<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from 'vue'
import { ElAlert, ElButton, ElDrawer, ElTree } from 'element-plus'
import { useMediaQuery } from '@vueuse/core'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { saveAttachment } from './api'
import { PureTableBar } from "@/components/RePureTableBar"
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
const tableBar = useTemplateRef('tableBar')
const navigation = employeeNavigation()
const selectedId = shallowRef(navigation.employeeId)
const drawerOpen = shallowRef(selectedId.value !== null)
let returnFocus
const selectedContract = shallowRef(navigation.contractId)
navigation.contractId = null
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
const departments = useRemote((signal) => session.authorized('/departments/tree', { signal }))
if (canFilterDepartment.value) void departments.load()
const organization = computed(() => {
  const byId = new Map((departments.result.value?.list || []).map(item => [item.id, { ...item, children: [] }]))
  const roots = []
  for (const node of byId.values()) {
    const parent = byId.get(node.parentId)
    if (parent && parent !== node) parent.children.push(node)
    else roots.push(node)
  }
  return roots
})
const narrow = useMediaQuery('(max-width: 38rem)')
function labelTable(element, { value }) {
  // auto layout renders header and body in the same native Element Plus table.
  const table = element.querySelector('.el-table__body')
  table?.setAttribute('aria-label', '员工列表')
  table?.setAttribute('data-density', value)
}
const vLabelTable = { mounted: labelTable, updated: labelTable }
function desktopColumns(dynamic) {
  return [...dynamic.filter(column => !column.hide).map(column => ({
    ...column, slot: column.prop,
    minWidth: column.prop === 'person' ? 145 : column.prop === 'salary' ? 108 : 86,
    align: ['tenure', 'salary'].includes(column.prop) ? 'right' : 'left'
  })), { label: '操作', slot: 'actions', minWidth: 100, align: 'right' }]
}
const columns = computed(() => [
  { label: '姓名 / 工号', prop: 'person' },
  { label: '部门', prop: 'department' },
  { label: '工龄', prop: 'tenure' },
  ...(session.canReadSalary ? [{ label: '约定工资', prop: 'salary' }] : []),
  { label: '在职状态', prop: 'employment' }
])
const fullscreen = shallowRef(false)
function selectDepartment(node) {
  if (adding.value) return
  departmentId.value = node?.id ?? null
  applyFilters()
}
const adding = shallowRef(false)
const editingDetail = shallowRef(false)
const locked = computed(() => adding.value || editingDetail.value)
const selfService = computed(() => ['EMPLOYEE', 'APPROVER'].includes(session.account?.user.role))
const list = useRemote((signal) => session.authorized('/employees/search', {
  method: 'POST', data: { ...applied.value, page: page.value, pageSize: pageSize.value }, signal
}))
const { result, loading, error, load } = list

watch([page, pageSize, applied], load, { immediate: true })

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

function closeDetail(done) {
  if (locked.value) return
  if (typeof done === 'function') done()
  else drawerOpen.value = false
}

function clearDetail() {
  selectedId.value = null
  selectedContract.value = null
  void nextTick(() => {
    if (returnFocus?.isConnected) returnFocus.focus()
    else heading.value?.focus()
    returnFocus = undefined
  })
}

function openEmployee(id, event) {
  tableBar.value?.exitFullscreen()
  if (adding.value) {
    adding.value = false
    void load()
  }
  returnFocus = event?.currentTarget
  selectedContract.value = null
  selectedId.value = id
  drawerOpen.value = true
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
  <section class="oa-pure oa-employee-page" aria-labelledby="employees-heading">
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

    <div class="oa-directory-layout" :class="{ 'oa-directory-with-tree': canFilterDepartment }">
    <aside v-if="canFilterDepartment" class="oa-surface oa-org-tree" aria-labelledby="oa-org-heading">
      <h2 id="oa-org-heading">组织架构</h2>
      <p class="oa-muted">人数为授权范围内的直属档案数。</p>
      <RemoteState :loading="departments.loading.value" :error="departments.error.value" :empty="Boolean(departments.result.value && !organization.length)" empty-text="暂无部门。" @retry="departments.load" />
      <ElButton text type="primary" :disabled="adding" :aria-pressed="applied.departmentId === null" @click="selectDepartment(null)">全部部门</ElButton>
      <ElTree v-if="organization.length" :data="organization" :props="{ label: 'name' }" node-key="id" highlight-current :current-node-key="applied.departmentId" :expand-on-click-node="false" @node-click="selectDepartment">
        <template #default="{ data }"><span class="oa-tree-node"><span>{{ data.name }}</span><span class="oa-count">{{ data.employeeCount }} 人</span></span></template>
      </ElTree>
    </aside>
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
              <option v-for="department in departments.result.value?.list || []" :key="department.id" :value="department.id">{{ department.name }}</option>
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
            <ElButton v-if="applied.departmentId" round text type="primary" :disabled="adding" @click="selectDepartment(null)">
              {{ departments.result.value?.list.find(item => item.id === applied.departmentId)?.name || '部门筛选' }} ×
            </ElButton>
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
      <PureTableBar ref="tableBar" :key="session.canReadSalary" :columns="columns" :show-pinning="false" :class="{ 'oa-directory-fullscreen': fullscreen }"
        @refresh="!adding && load()" @fullscreen="fullscreen = $event">
        <template #title><span class="oa-directory-view-label">表格视图</span></template>
        <template #default="{ size, dynamicColumns }">
      <RemoteState
        :loading="loading"
        :error="error"
        :empty="Boolean(result && !result.list?.length)"
        :empty-text="applied.query || applied.departmentId ? '没有符合当前筛选条件的员工。' : '暂无员工档案。'"
        @retry="load"
      />
      <template v-if="!loading && !error && result?.list?.length">
        <pure-table v-if="!narrow" v-label-table="size" class="oa-directory-pure-table"
          :data="result.list" :size="size" :columns="desktopColumns(dynamicColumns)"
          :key="dynamicColumns.filter(column => !column.hide).map(column => column.prop).join(':')"
          row-key="id" table-layout="auto" scrollbar-tabindex="0">
          <template #person="{ row }">
            <div class="oa-person-cell">
              <span class="oa-avatar" aria-hidden="true">{{ nameInitial(row.name) }}</span>
              <div><div class="oa-person-name">{{ row.name }}</div><div class="oa-employee-number">{{ row.employeeNo }}</div></div>
            </div>
          </template>
          <template #department="{ row }">{{ row.department }}</template>
          <template #tenure="{ row }">{{ row.tenureYears }} 年</template>
          <template v-if="session.canReadSalary" #salary="{ row }">{{ formatSalary(row.salaryCents) }}</template>
          <template #employment="{ row }">{{ row.employmentStatus === 'OFFBOARDED' ? '已离职' : '在职' }}</template>
          <template #actions="{ row }">
            <ElButton text type="primary" :disabled="adding" @click="openEmployee(row.id, $event)">查看档案</ElButton>
          </template>
        </pure-table>
        <div v-else class="oa-table-wrap">
          <table class="oa-table" :data-density="size" aria-label="员工列表">
            <thead>
              <tr>
                <template v-for="column in dynamicColumns" :key="column.prop">
                  <th v-if="!column.hide" scope="col" :class="{ 'oa-number': ['tenure', 'salary'].includes(column.prop) }">{{ column.label }}</th>
                </template>
                <th scope="col" class="oa-table-action">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="employee in result.list" :key="employee.id">
                <template v-for="column in dynamicColumns" :key="column.prop">
                  <component :is="column.prop === 'person' ? 'th' : 'td'" v-if="!column.hide"
                    :scope="column.prop === 'person' ? 'row' : undefined" :data-label="column.prop === 'person' ? undefined : column.label"
                    :class="{ 'oa-number': ['tenure', 'salary'].includes(column.prop) }">
                    <div v-if="column.prop === 'person'" class="oa-person-cell">
                      <span class="oa-avatar" aria-hidden="true">{{ nameInitial(employee.name) }}</span>
                      <div><div class="oa-person-name">{{ employee.name }}</div><div class="oa-employee-number">{{ employee.employeeNo }}</div></div>
                    </div>
                    <template v-else-if="column.prop === 'department'">{{ employee.department }}</template>
                    <template v-else-if="column.prop === 'tenure'">{{ employee.tenureYears }} 年</template>
                    <template v-else-if="column.prop === 'salary' && session.canReadSalary">{{ formatSalary(employee.salaryCents) }}</template>
                    <template v-else-if="column.prop === 'employment'">{{ employee.employmentStatus === 'OFFBOARDED' ? '已离职' : '在职' }}</template>
                  </component>
                </template>
                <td class="oa-table-action">
                  <ElButton text type="primary" :disabled="adding" @click="openEmployee(employee.id, $event)">查看档案</ElButton>
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
          label="员工列表分页" jump
          :disabled="adding || loading"
          @change="page = $event"
        />
        </template>
      </PureTableBar>
    </section>
    </div>
    <ElDrawer v-model="drawerOpen" class="oa-employee-drawer" size="min(55rem, 100vw)"
      title="员工档案详情" :with-header="false" :close-on-click-modal="false"
      :close-on-press-escape="!locked" :before-close="closeDetail" @closed="clearDetail">
      <EmployeeDetail v-if="selectedId !== null" :key="selectedId" :employee-id="selectedId"
        :contract-id="selectedContract || undefined" @back="closeDetail" @editing="editingDetail = $event"
        @directory-changed="load" />
    </ElDrawer>
  </section>
</template>
