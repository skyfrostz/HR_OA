<script setup lang="ts">
import { computed, nextTick, shallowRef, useTemplateRef, watch } from "vue";
import { useMediaQuery } from "@vueuse/core";
import { ElAlert, ElButton } from "element-plus";
import type { Department, DepartmentTreeNode } from "./business";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";
import DepartmentDrawer from "./departments/department-drawer.vue";
import OrganizationChart from "./departments/organization-chart.vue";
import OrganizationTree from "./departments/organization-tree.vue";
import {
  ancestorIds,
  buildOrganization,
  defaultExpanded,
  searchDepartments,
  visibleDepartments
} from "./departments/organization-model";

const session = useSession();
const canManage = computed(
  () => session.account?.permissions.canManageDepartments === true
);
const narrow = useMediaQuery("(max-width: 38rem)");
const view = shallowRef<"chart" | "tree" | "list">(
  narrow.value ? "tree" : "chart"
);
const chartVisited = shallowRef(view.value === "chart");
const chart = useTemplateRef("chart");
const tree = useTemplateRef("tree");
const surface = useTemplateRef("surface");
const heading = useTemplateRef("heading");
const rows = shallowRef<DepartmentTreeNode[]>([]);
const received = shallowRef(false);
const expanded = shallowRef(new Set<number>());
const model = computed(() => buildOrganization(rows.value));
const visible = computed(() => visibleDepartments(model.value, expanded.value));
const selectedId = shallowRef<number | null>(null);
const rootChoice = shallowRef<number | null>(null);
const search = shallowRef("");
const matches = computed(() => searchDepartments(model.value, search.value));
const searchAttempted = shallowRef(false);
const listPage = shallowRef(1);
const listRows = computed(() =>
  search.value.trim() ? matches.value : rows.value
);
const pagedRows = computed(() =>
  listRows.value.slice((listPage.value - 1) * 20, listPage.value * 20)
);
const drawerOpen = shallowRef(false);
const drawer = shallowRef<{
  id: number | null;
  creating: boolean;
  parentId: number | null;
} | null>(null);
const editing = shallowRef(false);
let returnFocus: HTMLElement | null = null;
const selectedDepartment = computed(
  () => rows.value.find(row => row.id === drawer.value?.id) || null
);
const remote = useRemote((signal: AbortSignal) =>
  session.authorized<{ list: DepartmentTreeNode[] }>("/departments/tree", {
    signal
  })
);
watch(remote.result, result => {
  if (!result) return;
  rows.value = result.list;
  if (!received.value) {
    expanded.value = defaultExpanded(model.value);
    rootChoice.value =
      model.value.roots.find(id => model.value.children.get(id)?.length) ??
      model.value.roots[0] ??
      null;
  } else
    expanded.value = new Set(
      [...expanded.value].filter(id => model.value.byId.has(id))
    );
  received.value = true;
  listPage.value = Math.min(
    listPage.value,
    Math.max(1, Math.ceil(listRows.value.length / 20))
  );
});
void remote.load();
watch(search, () => {
  listPage.value = 1;
  searchAttempted.value = false;
});

async function refresh() {
  return await remote.load();
}
function switchView(value: "chart" | "tree" | "list") {
  if (editing.value) return;
  view.value = value;
  if (value === "chart") chartVisited.value = true;
}
function toggle(id: number) {
  const next = new Set(expanded.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  expanded.value = next;
}
async function locate(id: number) {
  selectedId.value = id;
  expanded.value = new Set([
    ...expanded.value,
    ...ancestorIds(model.value, id)
  ]);
  if (view.value === "list") {
    const index = listRows.value.findIndex(row => row.id === id);
    if (index >= 0) listPage.value = Math.floor(index / 20) + 1;
  }
  await nextTick();
  if (view.value === "chart") chart.value?.locate(id);
  else if (view.value === "tree") tree.value?.locate(id);
  else
    surface.value
      ?.querySelector<HTMLButtonElement>(`button[data-department-id="${id}"]`)
      ?.focus();
}
function find() {
  searchAttempted.value = true;
  if (matches.value.length) void locate(matches.value[0].id);
}
function locateRoot() {
  if (rootChoice.value !== null) void locate(rootChoice.value);
}
function openDepartment(id: number, event?: MouseEvent) {
  if (drawerOpen.value) return;
  selectedId.value = id;
  returnFocus = (event?.currentTarget || document.activeElement) as HTMLElement;
  drawer.value = { id, creating: false, parentId: null };
  drawerOpen.value = true;
}
function createDepartment() {
  if (!canManage.value || drawerOpen.value) return;
  returnFocus = document.activeElement as HTMLElement;
  drawer.value = { id: null, creating: true, parentId: null };
  drawerOpen.value = true;
}
function saved(department: Department) {
  selectedId.value = department.id;
  if (drawer.value)
    drawer.value = { id: department.id, creating: false, parentId: null };
  if (department.parentId !== null)
    expanded.value = new Set([
      ...expanded.value,
      department.parentId,
      ...ancestorIds(model.value, department.parentId)
    ]);
}
async function closed() {
  drawer.value = null;
  editing.value = false;
  await nextTick();
  if (drawerOpen.value) return;
  const original = returnFocus?.isConnected ? returnFocus : null;
  const visibleView =
    view.value === "chart"
      ? ".oa-org-canvas"
      : view.value === "tree"
        ? ".oa-org-tree-list"
        : ".oa-department-list";
  const replacement = surface.value?.querySelector<HTMLButtonElement>(
    `${visibleView} button[data-department-id="${selectedId.value}"]`
  );
  (original || replacement || heading.value)?.focus({ preventScroll: true });
  returnFocus = null;
}
</script>

<template>
  <section
    ref="surface"
    class="oa-pure oa-departments"
    aria-labelledby="oa-department-heading"
  >
    <header class="oa-section-head oa-page-heading">
      <div>
        <h1 id="oa-department-heading" ref="heading" tabindex="-1">
          部门管理
          <span v-if="received" class="oa-count">{{ rows.length }} 个部门</span>
        </h1>
        <p class="oa-muted">组织关系与授权范围内的直属员工人数。</p>
      </div>
      <div class="oa-actions">
        <ElButton :disabled="editing || remote.loading.value" @click="refresh"
          >刷新</ElButton
        >
        <ElButton
          v-if="canManage"
          type="primary"
          :disabled="drawerOpen"
          @click="createDepartment"
          >新增部门</ElButton
        >
      </div>
    </header>
    <section class="oa-surface oa-organization" aria-label="部门组织">
      <div class="oa-org-toolbar">
        <form class="oa-org-search" role="search" @submit.prevent="find">
          <label for="department-search">搜索部门</label>
          <div class="oa-org-search-field">
            <input
              id="department-search"
              v-model="search"
              type="search"
              placeholder="输入部门名称"
              :disabled="editing"
            />
            <ElButton native-type="submit" :disabled="editing || !search.trim()"
              >定位</ElButton
            >
          </div>
        </form>
        <label
          v-if="!narrow && model.roots.length > 1"
          class="oa-org-root-picker"
          for="organization-root"
          >根部门
          <select
            id="organization-root"
            v-model="rootChoice"
            :disabled="editing"
            @change="locateRoot"
          >
            <option v-for="rootId in model.roots" :key="rootId" :value="rootId">
              {{ model.byId.get(rootId)?.name }}
            </option>
          </select>
        </label>
        <div
          v-if="!narrow"
          class="oa-actions oa-org-views"
          aria-label="组织显示方式"
        >
          <ElButton
            :aria-pressed="view === 'chart'"
            :disabled="editing"
            @click="switchView('chart')"
            >结构图</ElButton
          >
          <ElButton
            :aria-pressed="view === 'tree'"
            :disabled="editing"
            @click="switchView('tree')"
            >层级树</ElButton
          >
          <ElButton
            :aria-pressed="view === 'list'"
            :disabled="editing"
            @click="switchView('list')"
            >列表</ElButton
          >
        </div>
        <details v-if="narrow" class="oa-org-mobile-tools">
          <summary>
            浏览选项
            <span class="oa-muted">
              {{
                view === "chart"
                  ? "结构图"
                  : view === "tree"
                    ? "层级树"
                    : "列表"
              }}
              · {{ view === "list" ? listRows.length : visible.length }}/{{
                rows.length
              }}
              个部门
            </span>
          </summary>
          <div class="oa-org-mobile-options">
            <label
              v-if="model.roots.length > 1"
              class="oa-org-root-picker"
              for="organization-root"
              >根部门
              <select
                id="organization-root"
                v-model="rootChoice"
                :disabled="editing"
                @change="locateRoot"
              >
                <option
                  v-for="rootId in model.roots"
                  :key="rootId"
                  :value="rootId"
                >
                  {{ model.byId.get(rootId)?.name }}
                </option>
              </select>
            </label>
            <div class="oa-actions oa-org-views" aria-label="组织显示方式">
              <ElButton
                :aria-pressed="view === 'chart'"
                :disabled="editing"
                @click="switchView('chart')"
                >结构图</ElButton
              >
              <ElButton
                :aria-pressed="view === 'tree'"
                :disabled="editing"
                @click="switchView('tree')"
                >层级树</ElButton
              >
              <ElButton
                :aria-pressed="view === 'list'"
                :disabled="editing"
                @click="switchView('list')"
                >列表</ElButton
              >
            </div>
            <div
              v-if="received && rows.length && view !== 'list'"
              class="oa-actions"
            >
              <ElButton
                v-if="selectedId !== null && model.byId.has(selectedId)"
                :disabled="editing"
                @click="locate(selectedId)"
                >定位当前部门</ElButton
              >
              <ElButton
                :disabled="editing"
                @click="expanded = new Set(model.byId.keys())"
                >展开全部</ElButton
              >
              <ElButton
                :disabled="editing"
                @click="expanded = defaultExpanded(model)"
                >收起至两层</ElButton
              >
            </div>
          </div>
        </details>
      </div>
      <div v-if="search.trim()" class="oa-org-matches">
        <p v-if="matches.length" class="oa-muted" role="status">
          找到 {{ matches.length }} 个部门{{
            matches.length > 20 ? "，显示前 20 项，可继续输入缩小范围。" : ""
          }}
        </p>
        <p v-else-if="received" role="status">
          {{
            searchAttempted
              ? "未找到匹配部门，请调整名称。"
              : "没有匹配的部门。"
          }}
        </p>
        <div class="oa-org-match-list">
          <button
            v-for="match in matches.slice(0, 20)"
            :key="match.id"
            type="button"
            :aria-pressed="selectedId === match.id"
            :disabled="editing"
            @click="locate(match.id)"
          >
            {{ match.name }}
          </button>
        </div>
      </div>
      <RemoteState
        :loading="!received && remote.loading.value"
        :error="remote.error.value"
        :empty="received && !rows.length && !remote.error.value"
        empty-text="暂无部门。管理员可新增第一个部门。"
        @retry="refresh"
      />
      <p
        v-if="received && remote.loading.value"
        class="oa-org-message"
        role="status"
      >
        正在刷新，当前展开与画布位置将保留。
      </p>
      <ElAlert
        v-if="model.invalidHierarchy"
        type="warning"
        :closable="false"
        title="部分上级关系不可读取，已单独显示相关部门，请刷新或联系管理员核对。"
      />
      <template v-if="received && rows.length">
        <div v-if="!narrow" v-show="view !== 'list'" class="oa-org-expansion">
          <span class="oa-muted"
            >已显示 {{ visible.length }} / {{ rows.length }} 个部门</span
          >
          <div class="oa-actions">
            <ElButton
              v-if="selectedId !== null && model.byId.has(selectedId)"
              :disabled="editing"
              @click="locate(selectedId)"
              >定位当前部门</ElButton
            >
            <ElButton
              :disabled="editing"
              @click="expanded = new Set(model.byId.keys())"
              >展开全部</ElButton
            >
            <ElButton
              :disabled="editing"
              @click="expanded = defaultExpanded(model)"
              >收起至两层</ElButton
            >
          </div>
        </div>
        <OrganizationChart
          v-if="chartVisited"
          v-show="view === 'chart'"
          ref="chart"
          :active="view === 'chart'"
          :items="visible"
          :expanded="expanded"
          :selected-id="selectedId"
          @toggle="toggle"
          @select="openDepartment"
        />
        <OrganizationTree
          v-show="view === 'tree'"
          ref="tree"
          :items="visible"
          :model="model"
          :expanded="expanded"
          :selected-id="selectedId"
          @toggle="toggle"
          @select="openDepartment"
        />
        <div v-show="view === 'list'" class="oa-org-directory">
          <ul class="oa-department-list" aria-label="部门名录">
            <li
              v-for="row in pagedRows"
              :key="row.id"
              :class="{ 'is-current': selectedId === row.id }"
            >
              <div>
                <strong>{{ row.name }}</strong>
                <p class="oa-muted">
                  上级：{{
                    row.parentId
                      ? model.byId.get(row.parentId)?.name || "当前不可读取"
                      : "无上级部门"
                  }}
                </p>
                <p class="oa-muted">
                  直属员工 {{ row.employeeCount ?? "—" }} · 直属下级
                  {{ model.children.get(row.id)?.length ?? 0 }}
                </p>
              </div>
              <ElButton
                :data-department-id="row.id"
                :aria-label="'查看部门 ' + row.name"
                @click="openDepartment(row.id, $event)"
                >查看部门</ElButton
              >
            </li>
          </ul>
          <PageNav
            :page="listPage"
            :total="listRows.length"
            :page-size="20"
            label="部门分页"
            :disabled="editing"
            @change="listPage = $event"
          />
        </div>
      </template>
    </section>
    <DepartmentDrawer
      v-if="drawer"
      :open="drawerOpen"
      :department="selectedDepartment"
      :departments="rows"
      :creating="drawer.creating"
      :parent-id="drawer.parentId"
      :reload="refresh"
      @editing="editing = $event"
      @close="drawerOpen = false"
      @closed="closed"
      @saved="saved"
    />
  </section>
</template>

<style scoped>
.oa-organization {
  padding: 0;
}

.oa-org-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: flex-end;
  justify-content: space-between;
  padding: 16px;
  border-bottom: 1px solid var(--oa-line);
}

.oa-org-search {
  display: grid;
  flex: 1 1 240px;
  gap: 8px;
  min-width: 0;
  max-width: 420px;
}

.oa-org-search label {
  font-size: 13px;
  color: var(--oa-muted);
}

.oa-org-root-picker {
  display: grid;
  flex: 0 1 240px;
  gap: 8px;
  min-width: 0;
  font-size: 13px;
  color: var(--oa-muted);
}

.oa-org-root-picker select {
  max-width: 100%;
  min-height: 36px;
  padding: 8px 12px;
  color: var(--oa-text);
  background: var(--oa-surface);
  border: 1px solid var(--oa-line);
  border-radius: 6px;
}

.oa-org-search-field {
  display: flex;
  gap: 8px;
  align-items: center;
}

.oa-org-search-field input {
  width: 100%;
  min-width: 0;
  min-height: 36px;
  padding: 8px 12px;
  color: var(--oa-text);
  background: var(--oa-surface);
  border: 1px solid var(--oa-line);
  border-radius: 6px;
}

.oa-org-views :deep([aria-pressed="true"]) {
  color: var(--oa-accent-text);
  background: var(--oa-tint);
  border-color: var(--oa-accent-text);
}

.oa-org-expansion {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--oa-line);
}

.oa-org-expansion > span {
  font-size: 12px;
}

.oa-org-matches,
.oa-org-message {
  padding: 12px 16px;
  margin: 0;
}

.oa-org-matches p {
  margin: 0 0 8px;
}

.oa-org-match-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.oa-org-match-list button {
  max-width: 100%;
  min-height: 36px;
  padding: 6px 12px;
  color: var(--oa-accent-text);
  overflow-wrap: anywhere;
  cursor: pointer;
  background: var(--oa-surface);
  border: 1px solid var(--oa-line);
  border-radius: 6px;
}

.oa-org-directory {
  padding: 0 16px 16px;
}

.oa-department-list {
  padding: 0;
  margin: 0;
  list-style: none;
}

.oa-department-list li {
  display: flex;
  gap: 16px;
  align-items: center;
  justify-content: space-between;
  padding: 16px 0;
  border-bottom: 1px solid var(--oa-line);
}

.oa-department-list li > div {
  min-width: 0;
  overflow-wrap: anywhere;
}

.oa-department-list li.is-current strong {
  color: var(--oa-accent-text);
}

.oa-department-list p {
  margin: 4px 0 0;
  font-size: 12px;
}

@media (width <= 38rem) {
  .oa-org-toolbar {
    gap: 12px;
    padding: 12px;
  }

  .oa-org-search {
    flex-basis: 100%;
    max-width: none;
  }

  .oa-org-search-field input {
    min-height: 44px;
    font-size: 16px;
  }

  .oa-org-mobile-tools {
    width: 100%;
    min-width: 0;
  }

  .oa-org-mobile-tools summary {
    min-height: 44px;
    padding: 12px 0;
    font-size: 14px;
    cursor: pointer;
  }

  .oa-org-mobile-tools summary span {
    margin-left: 8px;
    font-size: 12px;
  }

  .oa-org-mobile-options {
    display: grid;
    gap: 12px;
    padding-bottom: 8px;
  }

  .oa-org-root-picker select {
    min-height: 44px;
  }

  .oa-org-views {
    width: 100%;
  }

  .oa-org-views .el-button {
    flex: 1;
  }

  .oa-org-expansion {
    padding: 12px;
  }

  .oa-org-match-list button {
    min-height: 44px;
  }

  .oa-department-list li {
    gap: 8px;
  }
}
</style>
