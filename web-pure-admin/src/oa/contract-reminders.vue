<script setup lang="ts">
import { shallowRef, watch } from "vue";
import { useMediaQuery } from "@vueuse/core";
import { useRouter } from "vue-router";
import { ElAlert, ElButton, ElTag } from "element-plus";
import { listPage } from "./api";
import type { ContractReminder, Page } from "./business";
import { employeeNavigation } from "./navigation";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const session = useSession();
const router = useRouter();
const days = shallowRef(30);
const page = shallowRef(1);
const opening = shallowRef<number | null>(null);
const navigationError = shallowRef("");
const narrow = useMediaQuery("(max-width: 38rem)");
const { result, loading, error, load } = useRemote((signal: AbortSignal) =>
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

watch([days, page], load, { immediate: true });

async function open(row: ContractReminder) {
  if (opening.value !== null) return;
  opening.value = row.contractId;
  navigationError.value = "";
  const navigation = employeeNavigation();
  navigation.employeeId = row.employeeId;
  navigation.contractId = row.contractId;
  const clearOwnNavigation = () => {
    if (
      navigation.employeeId === row.employeeId &&
      navigation.contractId === row.contractId
    ) {
      navigation.employeeId = null;
      navigation.contractId = null;
    }
  };
  try {
    const failure = await router.push({ name: "OaEmployees" });
    if (failure) {
      clearOwnNavigation();
      navigationError.value = "未能打开合同，请结束当前编辑后重试。";
    }
  } catch {
    clearOwnNavigation();
    navigationError.value = "打开合同失败，请重试。";
  } finally {
    opening.value = null;
  }
}

function due(daysRemaining: number) {
  if (daysRemaining === 0) return "今日到期";
  return daysRemaining < 0
    ? "已逾期 " + Math.abs(daysRemaining) + " 天"
    : "剩余 " + daysRemaining + " 天";
}
</script>

<template>
  <section class="oa-pure" aria-labelledby="oa-reminder-heading">
    <div class="oa-section-head oa-page-heading">
      <div>
        <h1 id="oa-reminder-heading">合同到期提醒</h1>
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
        v-if="navigationError"
        :title="navigationError"
        type="error"
        :closable="false"
      />
      <RemoteState
        :loading="loading"
        :error="error"
        :empty="Boolean(result && !result.list.length)"
        empty-text="所选范围内暂无到期合同。"
        @retry="load"
      />
      <pure-table
        v-if="!narrow && !loading && result?.list.length"
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
            :loading="opening === row.contractId"
            :disabled="opening !== null"
            @click="open(row)"
            >查看合同</ElButton
          ></template
        >
      </pure-table>
      <ul
        v-if="narrow && !loading && !error && result?.list.length"
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
              :loading="opening === row.contractId"
              :disabled="opening !== null"
              @click="open(row)"
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
