<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";
import { ElAlert, ElButton } from "element-plus";
import { listPage } from "./api";
import { halfDays, writeNeedsRefresh, type AnnualLeave, type Page } from "./business";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const props = defineProps<{ employeeId: number; unavailable?: boolean }>();
const emit = defineEmits<{ editing: [boolean]; changed: [] }>();
const session = useSession();
const canManage = computed(() => session.account?.permissions.canManageAnnualLeave === true);
const page = shallowRef(1);
const editor = shallowRef<AnnualLeave | "new" | null>(null);
const form = reactive({ year: "", entitled: "", used: "", note: "" });
const error = shallowRef("");
const feedback = shallowRef("");
const busy = shallowRef(false);
const needsRefresh = shallowRef(false);
const path = computed(() => "/employees/" + props.employeeId + "/annual-leave");
const { result, loading, error: readError, load } = useRemote((signal: AbortSignal) =>
  session.authorized<Page<AnnualLeave>>(listPage(path.value, page.value), { signal })
);
const columns: TableColumnList = [
  { label: "年份", prop: "year", width: 80 },
  { label: "应休", slot: "entitled", minWidth: 80 },
  { label: "已休", slot: "used", minWidth: 80 },
  { label: "剩余", slot: "remaining", minWidth: 80 },
  { label: "备注", prop: "note", minWidth: 110 },
  { label: "操作", slot: "actions", width: 90, hide: () => !canManage.value }
];

watch(page, load, { immediate: true });

function begin(record: AnnualLeave | "new") {
  if (!canManage.value || props.unavailable || editor.value) return;
  editor.value = record;
  form.year = record === "new" ? new Intl.DateTimeFormat("en", { timeZone: "Asia/Shanghai", year: "numeric" }).format(new Date()) : String(record.year);
  form.entitled = record === "new" ? "" : String(record.entitledHalfDays / 2);
  form.used = record === "new" ? "0" : String(record.usedHalfDays / 2);
  form.note = record === "new" ? "" : record.note;
  feedback.value = "";
  error.value = "";
  emit("editing", true);
}

function close() {
  editor.value = null;
  needsRefresh.value = false;
  emit("editing", false);
}

async function refresh() {
  if (await load()) close();
}

async function save() {
  if (!editor.value || busy.value || needsRefresh.value || !canManage.value) return;
  const year = Number(form.year);
  const entitledHalfDays = halfDays(form.entitled);
  const usedHalfDays = halfDays(form.used);
  const note = form.note.trim();
  if (!Number.isInteger(year) || year < 2000 || year > 2100 || entitledHalfDays === null ||
      usedHalfDays === null || usedHalfDays > entitledHalfDays || [...note].length > 1000) {
    error.value = "请检查年份、半天单位和备注。已休不能超过应休。";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    await session.authorized(path.value + "/" + year, {
      method: "PUT", data: { entitledHalfDays, usedHalfDays, note, version: editor.value === "new" ? 0 : editor.value.version }
    });
    close();
    feedback.value = "年假记录已保存。";
    emit("changed");
    if (page.value === 1) await load();
    else page.value = 1;
  } catch (cause) {
    needsRefresh.value = writeNeedsRefresh(cause);
    error.value = needsRefresh.value ? "年假记录可能已变化。请读取当前记录后再操作。" : cause.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="oa-surface" aria-labelledby="oa-annual-leave">
    <div class="oa-section-head">
      <div><h2 id="oa-annual-leave">年假记录</h2><p class="oa-muted">按年度手动维护，以半天为单位。</p></div>
      <ElButton v-if="canManage" type="primary" :disabled="Boolean(editor) || unavailable" @click="begin('new')">新增年假记录</ElButton>
    </div>
    <form v-if="editor" class="oa-inline-form oa-form" autocomplete="off" @submit.prevent="save">
      <h3>{{ editor === 'new' ? '新增年假记录' : '编辑年假记录' }}</h3>
      <div class="oa-fields">
        <label>年份<input v-model="form.year" type="number" min="2000" max="2100" required :disabled="busy || needsRefresh || editor !== 'new'"></label>
        <label>应休（天）<input v-model="form.entitled" type="text" inputmode="decimal" required :disabled="busy || needsRefresh"></label>
        <label>已休（天）<input v-model="form.used" type="text" inputmode="decimal" required :disabled="busy || needsRefresh"></label>
      </div>
      <label>备注<textarea v-model="form.note" rows="2" :disabled="busy || needsRefresh" /></label>
      <ElAlert v-if="error" :title="error" type="warning" :closable="false" />
      <div class="oa-actions">
        <ElButton v-if="needsRefresh" :loading="loading" @click="refresh">重新读取年假</ElButton>
        <template v-else>
          <ElButton type="primary" native-type="submit" :loading="busy">保存年假记录</ElButton>
          <ElButton :disabled="busy" @click="close">取消</ElButton>
        </template>
      </div>
    </form>
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <RemoteState :loading="loading" :error="readError" :empty="Boolean(result && !result.list.length)" empty-text="暂无年假记录。" @retry="load" />
    <pure-table v-if="!loading && result?.list.length" :data="result.list" :columns="columns" row-key="id">
      <template #entitled="{ row }">{{ row.entitledHalfDays / 2 }} 天</template>
      <template #used="{ row }">{{ row.usedHalfDays / 2 }} 天</template>
      <template #remaining="{ row }">{{ row.remainingHalfDays / 2 }} 天</template>
      <template #actions="{ row }"><ElButton v-if="canManage" text type="primary" :disabled="Boolean(editor) || unavailable" @click="begin(row)">编辑年假</ElButton></template>
    </pure-table>
    <PageNav v-if="result" :page="page" :total="result.total" :page-size="result.pageSize" label="年假分页" :disabled="Boolean(editor) || loading" @change="page = $event" />
  </section>
</template>
