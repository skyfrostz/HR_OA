<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from "vue";
import { ElAlert, ElButton, ElDrawer, ElTree } from "element-plus";
import { useMediaQuery } from "@vueuse/core";
import { Plus } from "@element-plus/icons-vue";
import { saveAttachment } from "./api";
import { PureTableBar } from "@/components/RePureTableBar";
import { employeeNavigation } from "./navigation";
import { useRouteWriteLock } from "./use-route-write-lock";
import { formatSalary, nameInitial } from "./format";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import EmployeeForm from "./employee-form.vue";
import EmployeeDetail from "./employee-detail.vue";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const session = useSession();
const workspace = useTemplateRef("workspace");
const heading = useTemplateRef("heading");
const tableBar = useTemplateRef("tableBar");
const navigation = employeeNavigation();
const selectedId = shallowRef(navigation.employeeId);
const drawerOpen = shallowRef(selectedId.value !== null);
let returnFocus;
const returnEmployeeId = shallowRef(null);
const selectedContract = shallowRef(navigation.contractId);
navigation.contractId = null;
navigation.employeeId = null;
const exporting = shallowRef(false);
const exportError = shallowRef("");
const page = shallowRef(1);
const pageSize = shallowRef(20);
const query = shallowRef("");
const departmentId = shallowRef(null);
const departmentScope = shallowRef("DIRECT");
const applied = shallowRef({ query: "", departmentId: null });
const filterError = shallowRef("");
const canFilterDepartment = computed(() =>
  ["HR_ADMIN", "HR_VIEWER"].includes(session.account?.user.role)
);
const departments = useRemote(signal =>
  session.authorized("/departments/tree", { signal })
);
if (canFilterDepartment.value) void departments.load();
const organization = computed(() => {
  const byId = new Map(
    (departments.result.value?.list || []).map(item => [
      item.id,
      { ...item, children: [] }
    ])
  );
  const roots = [];
  for (const node of byId.values()) {
    const parent = byId.get(node.parentId);
    if (parent && parent !== node) parent.children.push(node);
    else roots.push(node);
  }
  return roots;
});
const narrow = useMediaQuery("(max-width: 38rem)");
const filtersOpen = shallowRef(false);
function labelTable(element, { value }) {
  // auto layout renders header and body in the same native Element Plus table.
  const table = element.querySelector(".el-table__body");
  table?.setAttribute("aria-label", "员工列表");
  table?.setAttribute("data-density", value);
}
const vLabelTable = { mounted: labelTable, updated: labelTable };
function desktopColumns(dynamic) {
  return [
    ...dynamic
      .filter(column => !column.hide)
      .map(column => ({
        ...column,
        slot: column.prop,
        minWidth:
          column.prop === "person" ? 145 : column.prop === "salary" ? 108 : 86,
        align: ["tenure", "salary"].includes(column.prop) ? "right" : "left"
      })),
    { label: "操作", slot: "actions", minWidth: 100, align: "right" }
  ];
}
const columns = computed(() => [
  { label: "姓名 / 工号", prop: "person" },
  { label: "部门", prop: "department" },
  { label: "工龄", prop: "tenure" },
  ...(session.canReadSalary ? [{ label: "约定工资", prop: "salary" }] : []),
  { label: "在职状态", prop: "employment" }
]);
const fullscreen = shallowRef(false);
function selectDepartment(node) {
  if (adding.value) return;
  departmentId.value = node?.id ?? null;
  applyFilters();
  if (narrow.value) filtersOpen.value = false;
}
const adding = shallowRef(false);
const editingDetail = shallowRef(false);
const locked = computed(() => adding.value || editingDetail.value);
const selfService = computed(() =>
  ["EMPLOYEE", "APPROVER"].includes(session.account?.user.role)
);
const list = useRemote(signal =>
  session.authorized("/employees/search", {
    method: "POST",
    data: { ...applied.value, page: page.value, pageSize: pageSize.value },
    signal
  })
);
const { result, loading, error, load } = list;

watch(
  [page, pageSize, applied],
  () => {
    returnEmployeeId.value = null;
    void load();
  },
  { immediate: true }
);
watch([drawerOpen, adding], ([open, creating]) => {
  if (open || creating) returnEmployeeId.value = null;
});
watch(
  [loading, returnEmployeeId],
  async ([pending, id]) => {
    if (pending || id === null) return;
    await nextTick();
    if (
      returnEmployeeId.value !== id ||
      loading.value ||
      drawerOpen.value ||
      adding.value ||
      !workspace.value?.isConnected
    )
      return;
    returnEmployeeId.value = null;
    const active = document.activeElement;
    if (
      active !== document.body &&
      active !== returnFocus &&
      !active?.closest(".oa-employee-drawer")
    )
      return;
    const action = workspace.value.querySelector(
      `button[data-employee-id="${id}"]:not(:disabled)`
    );
    (action || heading.value)?.focus();
    returnFocus = undefined;
  },
  { flush: "post" }
);

useRouteWriteLock(locked);

function applyFilters() {
  if (adding.value) return;
  const normalized = query.value.trim();
  if ([...normalized].length > 100) {
    filterError.value = "搜索内容不能超过 100 个字符。";
    return;
  }
  filterError.value = "";
  exportError.value = "";
  page.value = 1;
  applied.value = {
    query: normalized,
    departmentId: canFilterDepartment.value ? departmentId.value : null,
    ...(canFilterDepartment.value &&
    departmentId.value &&
    departmentScope.value === "SUBTREE"
      ? { departmentScope: "SUBTREE" }
      : {})
  };
}

function resetFilters() {
  if (adding.value) return;
  query.value = "";
  departmentId.value = null;
  departmentScope.value = "DIRECT";
  applyFilters();
}

function closeDetail(done) {
  if (locked.value) return;
  if (typeof done === "function") done();
  else drawerOpen.value = false;
}

function clearDetail() {
  returnEmployeeId.value = selectedId.value;
  selectedId.value = null;
  selectedContract.value = null;
}

function openEmployee(id, event) {
  tableBar.value?.exitFullscreen();
  if (adding.value) {
    adding.value = false;
    void load();
  }
  returnFocus = event?.currentTarget;
  selectedContract.value = null;
  selectedId.value = id;
  drawerOpen.value = true;
}
async function exportEmployees() {
  if (exporting.value || !session.account?.permissions.canExportEmployees)
    return;
  exporting.value = true;
  exportError.value = "";
  try {
    const file = await session.authorized("/employees/export.xlsx", {
      method: "POST",
      data: applied.value,
      binary: true
    });
    saveAttachment(file, "员工档案.xlsx");
  } catch (cause) {
    exportError.value =
      cause.errorCode === "EXPORT_LIMIT"
        ? "授权范围内超过 5000 条记录，无法导出。"
        : cause.message;
  } finally {
    exporting.value = false;
  }
}
</script>

<template>
  <section
    ref="workspace"
    class="oa-pure oa-employee-page"
    aria-labelledby="employees-heading"
  >
    <div class="oa-section-head oa-page-heading">
      <div>
        <h1 id="employees-heading" ref="heading" tabindex="-1">
          员工档案
          <span v-if="result && !adding" class="oa-count"
            >{{ result.total }} 人</span
          >
        </h1>
        <p v-if="selfService" class="oa-muted">本人的档案与劳动合同</p>
      </div>
      <div class="oa-actions">
        <ElButton
          v-if="session.account?.permissions.canExportEmployees && !adding"
          :loading="exporting"
          :disabled="adding"
          @click="exportEmployees"
          >按当前筛选导出</ElButton
        >
        <ElButton
          v-if="session.canWrite && !adding"
          type="primary"
          :icon="Plus"
          @click="adding = true"
          >新增员工</ElButton
        >
      </div>
    </div>
    <ElAlert
      v-if="exportError"
      :title="exportError"
      type="error"
      :closable="false"
    />

    <EmployeeForm
      v-if="adding && session.canWrite"
      @cancel="adding = false"
      @created="openEmployee"
    />

    <div
      v-show="!adding"
      class="oa-directory-layout"
      :class="{
        'oa-directory-with-tree':
          canFilterDepartment && (!narrow || filtersOpen)
      }"
    >
      <aside
        v-if="canFilterDepartment && (!narrow || filtersOpen)"
        class="oa-surface oa-org-tree"
        aria-labelledby="oa-org-heading"
      >
        <h2 id="oa-org-heading">组织架构</h2>
        <p class="oa-muted">
          树上人数为授权范围内的直属员工数。可在筛选中选择包含子部门。
        </p>
        <RemoteState
          :loading="departments.loading.value"
          :error="departments.error.value"
          :empty="Boolean(departments.result.value && !organization.length)"
          empty-text="暂无部门。"
          @retry="departments.load"
        />
        <ElButton
          text
          type="primary"
          :disabled="adding"
          :aria-pressed="applied.departmentId === null"
          @click="selectDepartment(null)"
          >全部部门</ElButton
        >
        <ElTree
          v-if="organization.length"
          :data="organization"
          :props="{ label: 'name' }"
          node-key="id"
          highlight-current
          :current-node-key="applied.departmentId"
          :expand-on-click-node="false"
          @node-click="selectDepartment"
        >
          <template #default="{ data }"
            ><span class="oa-tree-node"
              ><span>{{ data.name }}</span
              ><span class="oa-count">{{ data.employeeCount }} 人</span></span
            ></template
          >
        </ElTree>
      </aside>
      <section class="oa-surface oa-table-panel" aria-label="员工名册">
        <form
          class="oa-form oa-directory-filters"
          autocomplete="off"
          @submit.prevent="applyFilters"
        >
          <div class="oa-fields">
            <label for="oa-employee-search">
              搜索员工
              <input
                id="oa-employee-search"
                v-model="query"
                type="search"
                placeholder="姓名、工号或部门名称"
                :disabled="adding"
              />
            </label>
            <label
              v-if="canFilterDepartment && (!narrow || filtersOpen)"
              for="oa-department-filter"
            >
              部门筛选
              <select
                id="oa-department-filter"
                v-model="departmentId"
                :disabled="
                  adding ||
                  departments.loading.value ||
                  Boolean(departments.error.value)
                "
              >
                <option :value="null">全部部门</option>
                <option
                  v-for="department in departments.result.value?.list || []"
                  :key="department.id"
                  :value="department.id"
                >
                  {{ department.name }}
                </option>
              </select>
            </label>
            <label
              v-if="
                canFilterDepartment && departmentId && (!narrow || filtersOpen)
              "
              for="oa-department-scope"
              >部门范围
              <select
                id="oa-department-scope"
                v-model="departmentScope"
                :disabled="adding"
              >
                <option value="DIRECT">仅直属员工</option>
                <option value="SUBTREE">包含子部门</option>
              </select>
            </label>
          </div>
          <ElAlert
            v-if="filterError"
            :title="filterError"
            type="warning"
            :closable="false"
          />
          <div v-if="departments.error.value" class="oa-actions">
            <span class="oa-muted" role="status">暂时无法读取部门选项。</span>
            <ElButton :disabled="adding" @click="departments.load"
              >重新加载部门</ElButton
            >
          </div>
          <div class="oa-actions">
            <div class="oa-actions">
              <ElButton
                v-if="applied.departmentId"
                round
                text
                type="primary"
                :disabled="adding"
                @click="selectDepartment(null)"
              >
                {{
                  departments.result.value?.list.find(
                    item => item.id === applied.departmentId
                  )?.name || "部门筛选"
                }}
                ×
              </ElButton>
              <ElButton native-type="submit" type="primary" :disabled="adding"
                >搜索</ElButton
              >
              <ElButton
                v-if="narrow && canFilterDepartment"
                :aria-expanded="filtersOpen"
                @click="filtersOpen = !filtersOpen"
                >{{ filtersOpen ? "收起筛选" : "部门筛选" }}</ElButton
              >
              <ElButton :disabled="adding" @click="resetFilters">清空</ElButton>
              <ElButton :disabled="loading || adding" @click="load"
                >刷新</ElButton
              >
            </div>
          </div>
        </form>
        <PureTableBar
          ref="tableBar"
          :key="session.canReadSalary"
          :columns="columns"
          :show-pinning="false"
          :class="{ 'oa-directory-fullscreen': fullscreen }"
          @refresh="!adding && load()"
          @fullscreen="fullscreen = $event"
        >
          <template #title
            ><span class="oa-directory-view-label">{{
              applied.departmentId
                ? applied.departmentScope === "SUBTREE"
                  ? "当前部门及子部门"
                  : "当前部门 · 直属员工"
                : "员工名册"
            }}</span></template
          >
          <template #default="{ size, dynamicColumns }">
            <RemoteState
              :loading="loading"
              :error="error"
              :empty="Boolean(result && !result.list?.length)"
              :empty-text="
                applied.query || applied.departmentId
                  ? '没有符合当前筛选条件的员工。'
                  : '暂无员工档案。'
              "
              @retry="load"
            />
            <template v-if="!loading && !error && result?.list?.length">
              <pure-table
                v-if="!narrow"
                :key="
                  dynamicColumns
                    .filter(column => !column.hide)
                    .map(column => column.prop)
                    .join(':')
                "
                v-label-table="size"
                class="oa-directory-pure-table"
                :data="result.list"
                :size="size"
                :columns="desktopColumns(dynamicColumns)"
                row-key="id"
                table-layout="auto"
                scrollbar-tabindex="0"
              >
                <template #person="{ row }">
                  <div class="oa-person-cell">
                    <span class="oa-avatar" aria-hidden="true">{{
                      nameInitial(row.name)
                    }}</span>
                    <div>
                      <div class="oa-person-name">{{ row.name }}</div>
                      <div class="oa-employee-number">{{ row.employeeNo }}</div>
                    </div>
                  </div>
                </template>
                <template #department="{ row }">{{ row.department }}</template>
                <template #tenure="{ row }">{{ row.tenureYears }} 年</template>
                <template v-if="session.canReadSalary" #salary="{ row }">{{
                  formatSalary(row.salaryCents)
                }}</template>
                <template #employment="{ row }">{{
                  row.employmentStatus === "OFFBOARDED" ? "已离职" : "在职"
                }}</template>
                <template #actions="{ row }">
                  <ElButton
                    class="oa-employee-open"
                    :data-employee-id="row.id"
                    text
                    type="primary"
                    :disabled="adding"
                    @click="openEmployee(row.id, $event)"
                    >查看档案</ElButton
                  >
                </template>
              </pure-table>
              <ul v-else class="oa-mobile-employees" aria-label="员工列表">
                <li
                  v-for="employee in result.list"
                  :key="employee.id"
                  class="oa-mobile-employee"
                >
                  <div class="oa-mobile-employee-head">
                    <div class="oa-mobile-identity">
                      <strong
                        :id="'oa-employee-name-' + employee.id"
                        class="oa-person-name"
                        >{{ employee.name }}</strong
                      >
                      <span class="oa-employee-number">{{
                        employee.employeeNo
                      }}</span>
                    </div>
                    <ElButton
                      class="oa-employee-open"
                      :data-employee-id="employee.id"
                      text
                      type="primary"
                      :disabled="adding"
                      :aria-describedby="'oa-employee-name-' + employee.id"
                      @click="openEmployee(employee.id, $event)"
                      >查看档案</ElButton
                    >
                  </div>
                  <dl class="oa-mobile-employee-facts">
                    <div>
                      <dt>部门</dt>
                      <dd>{{ employee.department }}</dd>
                    </div>
                    <div>
                      <dt>工龄</dt>
                      <dd>{{ employee.tenureYears }} 年</dd>
                    </div>
                    <div v-if="session.canReadSalary">
                      <dt>工资</dt>
                      <dd>{{ formatSalary(employee.salaryCents) }}</dd>
                    </div>
                    <div>
                      <dt>状态</dt>
                      <dd>
                        {{
                          employee.employmentStatus === "OFFBOARDED"
                            ? "已离职"
                            : "在职"
                        }}
                      </dd>
                    </div>
                  </dl>
                </li>
              </ul>
            </template>
            <div class="oa-directory-pagination">
              <label class="oa-filter" for="oa-employee-page-size">
                每页条数
                <select
                  id="oa-employee-page-size"
                  v-model="pageSize"
                  :disabled="adding"
                  @change="page = 1"
                >
                  <option :value="20">20</option>
                  <option :value="50">50</option>
                  <option :value="100">100</option>
                </select>
              </label>
              <PageNav
                v-if="result"
                :page="page"
                :total="result.total"
                :page-size="result.pageSize"
                label="员工列表分页"
                jump
                :disabled="adding || loading"
                @change="page = $event"
              />
            </div>
          </template>
        </PureTableBar>
      </section>
    </div>
    <ElDrawer
      v-model="drawerOpen"
      class="oa-employee-drawer"
      size="min(55rem, 100vw)"
      title="员工档案详情"
      :with-header="false"
      :close-on-click-modal="false"
      :close-on-press-escape="!locked"
      :before-close="closeDetail"
      @close="drawerOpen = false"
      @closed="clearDetail"
    >
      <EmployeeDetail
        v-if="selectedId !== null"
        :key="selectedId"
        :active="drawerOpen"
        :employee-id="selectedId"
        :contract-id="selectedContract || undefined"
        @back="closeDetail"
        @editing="editingDetail = $event"
        @directory-changed="load"
      />
    </ElDrawer>
  </section>
</template>

<style scoped>
.oa-employee-page :deep(.oa-employee-open.el-button) {
  color: var(--oa-accent-ink);
}

.oa-employee-page :deep(.oa-employee-open.el-button:hover:not(:disabled)) {
  color: var(--oa-accent-ink);
}

.oa-mobile-employees {
  padding: 0 4px;
  margin: 0;
  list-style: none;
}

.oa-mobile-employee {
  padding: 12px 0;
  border-top: 1px solid var(--oa-line);
}

.oa-mobile-employee-head {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
}

.oa-mobile-identity {
  display: grid;
  gap: 4px;
  min-width: 0;
  overflow-wrap: anywhere;
}

.oa-mobile-employee-head :deep(.el-button) {
  flex-shrink: 0;
  min-width: 44px;
  min-height: 44px;
  padding-inline: 8px;
}

.oa-mobile-employee-facts {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 4px 12px;
  margin: 8px 0 0;
  font-size: 14px;
  line-height: 1.6;
}

.oa-mobile-employee-facts > div {
  display: flex;
  gap: 8px;
  align-items: baseline;
  min-width: 0;
}

.oa-mobile-employee-facts dt {
  flex-shrink: 0;
  color: var(--oa-muted);
}

.oa-mobile-employee-facts dd {
  min-width: 0;
  margin: 0;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}

.oa-directory-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: end;
  padding: 12px 16px;
  margin: 0;
}

.oa-directory-filters .oa-fields {
  flex: 1;
  grid-template-columns: minmax(180px, 1fr) minmax(130px, 0.55fr);
  gap: 12px;
}

.oa-directory-filters .oa-fields label {
  gap: 4px;
  font-size: 13px;
}

.oa-directory-filters > .oa-actions {
  gap: 8px;
}

.oa-directory-pagination {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px 12px;
}

.oa-org-tree {
  padding: 16px;
}

.oa-org-tree h2 {
  font-size: 16px;
}

.oa-org-tree p {
  margin: 8px 0;
  font-size: 12px;
}

@media (width <= 38rem) {
  .oa-table-panel :deep(.oa-tablebar-toolbar) {
    display: none;
  }

  .oa-page-heading {
    gap: 8px;
    align-items: center;
  }

  .oa-page-heading > .oa-actions {
    justify-content: flex-start;
  }

  .oa-directory-filters {
    gap: 8px;
    padding: 12px;
  }

  .oa-directory-filters .oa-fields {
    flex-basis: 100%;
    grid-template-columns: 1fr;
  }

  .oa-directory-pagination {
    padding: 0 12px 12px;
  }

  .oa-directory-layout {
    gap: 8px;
  }
}
</style>
