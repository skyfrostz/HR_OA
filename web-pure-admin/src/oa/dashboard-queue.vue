<script setup>
import { shallowRef } from "vue";
import { useRouter } from "vue-router";
import { ElButton, ElAlert } from "element-plus";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import { employeeNavigation } from "./navigation";
import { statusLabels } from "./approvals/approval";
import RemoteState from "./remote-state.vue";
const props = defineProps({
  kind: { type: String, required: true },
  title: { type: String, required: true }
});
const session = useSession();
const router = useRouter();
const pending = shallowRef(false);
const actionError = shallowRef("");
const routes = {
  contracts: "/employees",
  tasks: "/approvals",
  requests: "/approvals",
  exceptions: "/workflow-exceptions"
};
const endpoints = {
  contracts: "/contract-reminders?days=30&page=1&pageSize=5",
  tasks: "/tasks?page=1&pageSize=5",
  requests: "/requests?page=1&pageSize=5",
  exceptions: "/workflow-exceptions?page=1&pageSize=5"
};
const state = useRemote(signal =>
  session.authorized(endpoints[props.kind], { signal })
);
void state.load();
function rowTitle(row) {
  if (props.kind === "contracts")
    return row.employeeName + " · " + row.contractNo;
  if (props.kind === "exceptions")
    return "申请 #" + row.requestId + " · " + row.stepName;
  return row.requestTitle || row.title;
}
function meta(row) {
  if (props.kind === "contracts")
    return (
      row.endDate +
      " 到期 · " +
      (row.daysRemaining < 0
        ? "已逾期 " + -row.daysRemaining + " 天"
        : "剩余 " + row.daysRemaining + " 天")
    );
  if (props.kind === "exceptions")
    return row.reason === "CURRENT_UNAVAILABLE"
      ? "当前处理人不可用"
      : "后续处理人不可用";
  return [row.applicantName, row.stepName, statusLabels[row.status]]
    .filter(Boolean)
    .join(" · ");
}
async function open(row) {
  if (pending.value) return;
  pending.value = true;
  actionError.value = "";
  const nav = employeeNavigation();
  const intended =
    props.kind === "contracts"
      ? { employeeId: row.employeeId, contractId: row.contractId }
      : props.kind === "tasks" || props.kind === "requests"
        ? {
            requestId: row.requestId || row.id,
            approvalView: props.kind === "tasks" ? "pending" : "mine"
          }
        : {};
  Object.assign(nav, intended);
  const clear = () => {
    if (Object.keys(intended).every(key => nav[key] === intended[key]))
      for (const key of Object.keys(intended)) nav[key] = null;
  };
  try {
    if (await router.push(routes[props.kind])) {
      clear();
      actionError.value = "未能打开详情，请重试。";
    }
  } catch {
    clear();
    actionError.value = "未能打开详情，请重试。";
  } finally {
    pending.value = false;
  }
}
</script>
<template>
  <section class="oa-surface oa-dashboard-queue" :aria-label="props.title">
    <header class="oa-section-head">
      <h2>{{ props.title }}</h2>
      <ElButton text :disabled="state.loading.value" @click="state.load"
        >刷新</ElButton
      >
    </header>
    <RemoteState
      :loading="state.loading.value"
      :error="state.error.value"
      :empty="Boolean(state.result.value && !state.result.value.list?.length)"
      :empty-text="'暂无' + props.title + '。'"
      @retry="state.load"
    />
    <ElAlert
      v-if="actionError"
      :title="actionError"
      type="error"
      :closable="false"
    />
    <ul v-if="!state.loading.value && state.result.value?.list?.length">
      <li
        v-for="row in state.result.value.list"
        :key="row.contractId || row.taskId || row.id"
      >
        <div>
          <strong>{{ rowTitle(row) }}</strong>
          <p class="oa-muted">{{ meta(row) }}</p>
        </div>
        <ElButton :disabled="pending" @click="open(row)">{{
          kind === "tasks" || kind === "exceptions" ? "去处理" : "查看"
        }}</ElButton>
      </li>
    </ul>
  </section>
</template>
<style scoped>
.oa-dashboard-queue {
  min-width: 0;
}

ul {
  padding: 0;
  margin: 0.5rem 0 0;
  list-style: none;
}

li {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 0;
  border-top: 1px solid var(--oa-line);
}

li > div {
  min-width: 0;
  overflow-wrap: anywhere;
}

strong {
  font-weight: 600;
}
</style>
