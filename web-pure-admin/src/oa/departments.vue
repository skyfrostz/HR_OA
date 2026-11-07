<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";
import { useRouteWriteLock } from "./use-route-write-lock";
import { ElAlert, ElButton } from "element-plus";
import { listPage } from "./api";
import { listDepartments, writeNeedsRefresh, type Department, type Page } from "./business";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const session = useSession();
const canManage = computed(() => session.account?.permissions.canManageDepartments === true);
const page = shallowRef(1);
const editor = shallowRef<Department | "new" | null>(null);
const form = reactive({ name: "", parentId: null as number | null });
const busy = shallowRef(false);
const needsRefresh = shallowRef(false);
const error = shallowRef("");
const feedback = shallowRef("");
const directory = useRemote((signal: AbortSignal) =>
  session.authorized<Page<Department>>(listPage("/departments", page.value), { signal })
);
const parents = useRemote((signal: AbortSignal) => listDepartments(session.authorized, signal));
const parentNames = computed(() => new Map<number, string>((parents.result.value || []).map((entry: Department) => [entry.id, entry.name])));
const eligibleParents = computed(() => {
  const selectedId = typeof editor.value === "object" ? editor.value?.id : null;
  return (parents.result.value || []).filter((parent: Department) => parent.id !== selectedId);
});
const columns: TableColumnList = [
  { label: "部门名称", prop: "name", minWidth: 160 },
  { label: "上级部门", slot: "parent", minWidth: 160 },
  { label: "操作", slot: "actions", width: 100, hide: () => !canManage.value }
];

watch(page, directory.load, { immediate: true });
void parents.load();
useRouteWriteLock(computed(() => busy.value || needsRefresh.value));

function begin(row: Department | "new") {
  if (!canManage.value || editor.value) return;
  editor.value = row;
  form.name = row === "new" ? "" : row.name;
  form.parentId = row === "new" ? null : row.parentId;
  error.value = "";
  feedback.value = "";
}

async function refresh() {
  const results = await Promise.all([directory.load(), parents.load()]);
  if (results.every(Boolean)) {
    needsRefresh.value = false;
    editor.value = null;
    error.value = "";
  }
}

async function save() {
  if (!editor.value || busy.value || needsRefresh.value || !canManage.value) return;
  const name = form.name.trim();
  if (!name || [...name].length > 128) {
    error.value = "请输入 1 至 128 字的部门名称。";
    return;
  }
  busy.value = true;
  error.value = "";
  const current = editor.value;
  try {
    await session.authorized(current === "new" ? "/departments" : "/departments/" + current.id, {
      method: current === "new" ? "POST" : "PUT",
      data: current === "new"
        ? { name, parentId: form.parentId }
        : { name, parentId: form.parentId, version: current.version }
    });
    editor.value = null;
    feedback.value = "部门已保存。";
    await refresh();
  } catch (cause) {
    needsRefresh.value = writeNeedsRefresh(cause);
    error.value = needsRefresh.value
      ? "部门记录可能已变化。请重新读取部门后再操作。"
      : cause.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="oa-pure" aria-labelledby="oa-department-heading">
    <header class="oa-section-head oa-page-heading">
      <div><h1 id="oa-department-heading">部门管理</h1><p class="oa-muted">查看部门与上级关系。</p></div>
      <ElButton v-if="canManage" type="primary" :disabled="Boolean(editor)" @click="begin('new')">新增部门</ElButton>
    </header>
    <form v-if="editor" class="oa-surface oa-form" autocomplete="off" @submit.prevent="save">
      <h2>{{ editor === 'new' ? '新增部门' : '编辑部门' }}</h2>
      <div class="oa-fields">
        <label>部门名称<input v-model="form.name" required :disabled="busy || needsRefresh"></label>
        <label>上级部门
          <select v-model="form.parentId" :disabled="busy || needsRefresh || parents.loading.value">
            <option :value="null">无上级部门</option>
            <option v-for="parent in eligibleParents" :key="parent.id" :value="parent.id">{{ parent.name }}</option>
          </select>
        </label>
      </div>
      <ElAlert v-if="error" :title="error" type="warning" :closable="false" />
      <div class="oa-actions">
        <ElButton v-if="needsRefresh" :loading="directory.loading.value" @click="refresh">重新读取部门</ElButton>
        <template v-else>
          <ElButton native-type="submit" type="primary" :loading="busy" :disabled="parents.loading.value || Boolean(parents.error.value)">保存部门</ElButton>
          <ElButton :disabled="busy" @click="editor = null">取消</ElButton>
        </template>
      </div>
    </form>
    <p v-if="feedback" role="status" class="oa-status">{{ feedback }}</p>
    <section class="oa-surface">
      <div class="oa-section-head">
        <h2>部门名录</h2><ElButton :disabled="busy || directory.loading.value" @click="refresh">刷新</ElButton>
      </div>
      <RemoteState
        :loading="directory.loading.value"
        :error="directory.error.value || parents.error.value"
        :empty="Boolean(directory.result.value && !directory.result.value.list.length)"
        empty-text="暂无部门记录。"
        @retry="refresh"
      />
      <pure-table
        v-if="!directory.loading.value && directory.result.value?.list.length"
        :data="directory.result.value.list" :columns="columns" row-key="id"
      >
        <template #parent="{ row }">{{ row.parentId ? (parentNames.get(row.parentId) || '—') : '—' }}</template>
        <template #actions="{ row }"><ElButton v-if="canManage" text type="primary" :disabled="Boolean(editor)" @click="begin(row)">编辑部门</ElButton></template>
      </pure-table>
      <PageNav v-if="directory.result.value" :page="page" :total="directory.result.value.total" :page-size="directory.result.value.pageSize" label="部门分页" :disabled="Boolean(editor)" @change="page = $event" />
    </section>
  </section>
</template>
