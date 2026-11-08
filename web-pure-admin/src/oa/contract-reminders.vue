<script setup lang="ts">
import {
  nextTick,
  onScopeDispose,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { useMediaQuery } from "@vueuse/core";
import { onBeforeRouteLeave } from "vue-router";
import { ElAlert, ElButton, ElTag } from "element-plus";
import { listPage } from "./api";
import type { ContractReminder, Page } from "./business";
import ContractDrawer from "./contract-drawer.vue";
import type { ContractChange } from "./use-contract-record";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const session = useSession();
const workspace = useTemplateRef("workspace");
const drawer = useTemplateRef("drawer");
const heading = useTemplateRef("heading");
const selected = shallowRef<ContractReminder | null>(null);
const drawerOpen = shallowRef(false);
const feedback = shallowRef("");
let restoreRequest = 0;
let active = true;
let trigger: HTMLElement | null = null;
let scrollContainer: HTMLElement | null = null;
let scrollPosition = { top: 0, left: 0 };
let stopRestore: (() => void) | undefined;
let refreshSequence = 0;
let listRevision = 0;
const days = shallowRef(30);
const page = shallowRef(1);
const narrow = useMediaQuery("(max-width: 38rem)");
const {
  result,
  loading,
  error,
  load: read
} = useRemote((signal: AbortSignal) =>
  session.authorized<Page<ContractReminder>>(
    listPage("/contract-reminders", page.value) + "&days=" + days.value,
    { signal }
  )
);
const columns: TableColumnList = [
  { label: "员工", slot: "employee", minWidth: 150 },
  { label: "合同编号", prop: "contractNo", minWidth: 120 },
  { label: "到期日期", prop: "endDate", minWidth: 112 },
  { label: "提醒", slot: "due", minWidth: 115 },
  { label: "操作", slot: "actions", width: 115 }
];

function load() {
  listRevision++;
  return read();
}

watch([days, page], load, { immediate: true });
watch(result, value => {
  if (!value) return;
  const lastPage = Math.max(1, Math.ceil(value.total / value.pageSize));
  if (page.value > lastPage) page.value = lastPage;
});

function open(row: ContractReminder, event: MouseEvent) {
  if (drawerOpen.value) return;
  restoreRequest++;
  stopRestore?.();
  stopRestore = undefined;
  trigger =
    event.currentTarget instanceof HTMLElement ? event.currentTarget : null;
  scrollContainer =
    trigger?.closest<HTMLElement>(".el-scrollbar__wrap") ||
    (document.scrollingElement as HTMLElement | null);
  scrollPosition = {
    top: scrollContainer?.scrollTop || 0,
    left: scrollContainer?.scrollLeft || 0
  };
  selected.value = row;
  drawerOpen.value = true;
}

async function restoreFocus() {
  const attempt = ++restoreRequest;
  const id = selected.value?.contractId;
  const focused = document.activeElement;
  await nextTick();
  const restore = () => {
    if (!active || drawerOpen.value || restoreRequest !== attempt) return;
    const current = document.activeElement;
    if (current !== focused && current !== document.body) return;
    const replacement = workspace.value?.querySelector<HTMLButtonElement>(
      `button[data-contract-id="${id}"]`
    );
    const target = trigger?.isConnected
      ? trigger
      : replacement || heading.value;
    if (scrollContainer?.isConnected) {
      scrollContainer.scrollTop = scrollPosition.top;
      scrollContainer.scrollLeft = scrollPosition.left;
    }
    target?.focus({ preventScroll: true });
  };
  if (!loading.value) restore();
  else {
    stopRestore?.();
    stopRestore = watch(loading, pending => {
      if (!pending) {
        stopRestore?.();
        stopRestore = undefined;
        void nextTick(restore);
      }
    });
  }
}

function exclusionMessage(
  change: ContractChange,
  original: ContractReminder | null
) {
  const contract = change.contract;
  if (!original || !contract || contract.employeeId !== original.employeeId)
    return "";
  if (contract.id === original.contractId && contract.status === "TERMINATED")
    return "合同已终止，不再符合到期提醒条件。";
  const today = new Intl.DateTimeFormat("sv-SE", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
  }).format(new Date());
  if (
    contract.previousContractId === original.contractId &&
    contract.status === "SIGNED" &&
    contract.endDate > original.endDate &&
    contract.endDate >= today
  )
    return "续约合同已登记签署，原合同不再符合到期提醒条件。";
  return "";
}

async function changed(change: ContractChange) {
  const sequence = ++refreshSequence;
  const original = selected.value?.contractId;
  const exclusion = exclusionMessage(change, selected.value);
  feedback.value = exclusion || "正在刷新到期提醒。";
  const listRequest = listRevision + 1;
  const loaded = await load();
  if (!active || sequence !== refreshSequence) return;
  if (listRequest !== listRevision) {
    feedback.value = exclusion || "请以当前筛选结果为准。";
    return;
  }
  if (!loaded) {
    feedback.value =
      (exclusion ? exclusion + " " : "") + "提醒列表刷新失败，请重试刷新。";
    return;
  }
  if (exclusion) return;
  if (result.value?.list.some(row => row.contractId === original))
    feedback.value = "到期提醒已刷新。";
  else if (
    result.value?.page === 1 &&
    result.value.total <= result.value.pageSize
  )
    feedback.value = "该合同已不符合当前提醒条件，到期提醒已刷新。";
  else
    feedback.value =
      "到期提醒已刷新，该合同已不在当前页；请按当前提醒范围查看。";
}

onBeforeRouteLeave(async () => {
  const accepted = (await drawer.value?.requestClose()) ?? true;
  if (accepted) restoreRequest++;
  return accepted;
});
onScopeDispose(() => {
  active = false;
  restoreRequest++;
  stopRestore?.();
});

function due(daysRemaining: number) {
  if (daysRemaining === 0) return "今日到期";
  return daysRemaining < 0
    ? "已逾期 " + Math.abs(daysRemaining) + " 天"
    : "剩余 " + daysRemaining + " 天";
}
</script>

<template>
  <section
    ref="workspace"
    class="oa-pure"
    aria-labelledby="oa-reminder-heading"
  >
    <div class="oa-section-head oa-page-heading">
      <div>
        <h1 id="oa-reminder-heading" ref="heading" tabindex="-1">
          合同到期提醒
        </h1>
        <p class="oa-muted">按中国标准时间查看当前授权范围内的合同。</p>
      </div>
      <ElButton :disabled="loading" @click="load">刷新</ElButton>
    </div>
    <section class="oa-surface">
      <div class="oa-section-head">
        <h2>到期队列</h2>
        <label class="oa-filter"
          >查看范围
          <select v-model.number="days" @change="page = 1">
            <option :value="7">逾期及未来 7 天</option>
            <option :value="30">逾期及未来 30 天</option>
            <option :value="60">逾期及未来 60 天</option>
            <option :value="90">逾期及未来 90 天</option>
          </select>
        </label>
      </div>
      <ElAlert
        v-if="feedback"
        :title="feedback"
        type="info"
        :closable="false"
      />
      <RemoteState
        :loading="loading && !result"
        :error="error"
        :empty="Boolean(result && !result.list.length)"
        empty-text="所选范围内暂无到期合同。"
        @retry="load"
      />
      <pure-table
        v-if="!narrow && result?.list.length"
        :aria-busy="loading"
        :data="result.list"
        :columns="columns"
        row-key="contractId"
      >
        <template #employee="{ row }">
          <div>{{ row.employeeName }}</div>
          <div class="oa-employee-number">{{ row.employeeNo }}</div>
        </template>
        <template #due="{ row }"
          ><ElTag
            :type="row.daysRemaining < 0 ? 'danger' : 'warning'"
            effect="plain"
            >{{ due(row.daysRemaining) }}</ElTag
          ></template
        >
        <template #actions="{ row }"
          ><ElButton
            text
            type="primary"
            :data-contract-id="row.contractId"
            @click="open(row, $event)"
            >查看合同</ElButton
          ></template
        >
      </pure-table>
      <ul
        v-if="narrow && !error && result?.list.length"
        :aria-busy="loading"
        class="oa-reminder-list"
      >
        <li v-for="row in result.list" :key="row.contractId">
          <div>
            <strong>{{ row.employeeName }}</strong
            ><span class="oa-muted">{{ row.employeeNo }}</span>
          </div>
          <p>{{ row.contractNo }}</p>
          <div class="oa-reminder-action">
            <span>到期 {{ row.endDate }}<br />{{ due(row.daysRemaining) }}</span
            ><ElButton
              text
              type="primary"
              :data-contract-id="row.contractId"
              @click="open(row, $event)"
              >查看合同</ElButton
            >
          </div>
        </li>
      </ul>
      <PageNav
        v-if="result"
        :page="page"
        :total="result.total"
        :page-size="result.pageSize"
        label="到期提醒分页"
        :disabled="loading"
        @change="page = $event"
      />
    </section>
    <ContractDrawer
      ref="drawer"
      v-model="drawerOpen"
      :contract-id="selected?.contractId ?? null"
      :expected-employee-id="selected?.employeeId ?? null"
      :employee-name="selected?.employeeName"
      :employee-no="selected?.employeeNo"
      @changed="changed"
      @closed="restoreFocus"
    />
  </section>
</template>

<style scoped>
.oa-reminder-list {
  padding: 0;
  margin: 12px 0;
  list-style: none;
}

.oa-reminder-list li {
  padding: 12px 0;
  overflow-wrap: anywhere;
  border-bottom: 1px solid var(--oa-line);
}

.oa-reminder-list li > div:first-child {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.oa-reminder-list p {
  margin: 8px 0;
}

.oa-reminder-action {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
}

.oa-reminder-action :deep(.el-button) {
  flex-shrink: 0;
}
</style>
