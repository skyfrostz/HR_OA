<script setup lang="ts">
import { shallowRef, watch } from "vue";
import { useRouter } from "vue-router";
import { ElButton, ElTag } from "element-plus";
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
const { result, loading, error, load } = useRemote((signal: AbortSignal) =>
  session.authorized<Page<ContractReminder>>(listPage("/contract-reminders", page.value) + "&days=" + days.value, { signal })
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
  const navigation = employeeNavigation();
  navigation.employeeId = row.employeeId;
  navigation.contractId = row.contractId;
  const failure = await router.push({ name: "OaEmployees" });
  if (failure) {
    navigation.employeeId = null;
    navigation.contractId = null;
  }
}

function due(daysRemaining: number) {
  if (daysRemaining === 0) return "今日到期";
  return daysRemaining < 0 ? "已逾期 " + Math.abs(daysRemaining) + " 天" : "剩余 " + daysRemaining + " 天";
}
</script>

<template>
  <section class="oa-pure" aria-labelledby="oa-reminder-heading">
    <div class="oa-section-head oa-page-heading">
      <div><h1 id="oa-reminder-heading">合同到期提醒</h1><p class="oa-muted">按中国标准时间查看当前授权范围内的合同。</p></div>
      <ElButton :disabled="loading" @click="load">刷新</ElButton>
    </div>
    <section class="oa-surface">
      <div class="oa-section-head">
        <h2>到期队列</h2>
        <label class="oa-filter">查看范围
          <select v-model.number="days" @change="page = 1">
            <option :value="7">逾期及未来 7 天</option>
            <option :value="30">逾期及未来 30 天</option>
            <option :value="60">逾期及未来 60 天</option>
            <option :value="90">逾期及未来 90 天</option>
          </select>
        </label>
      </div>
      <RemoteState :loading="loading" :error="error" :empty="Boolean(result && !result.list.length)" empty-text="所选范围内暂无到期合同。" @retry="load" />
      <pure-table v-if="!loading && result?.list.length" :data="result.list" :columns="columns" row-key="contractId">
        <template #employee="{ row }">
          <div>{{ row.employeeName }}</div><div class="oa-employee-number">{{ row.employeeNo }}</div>
        </template>
        <template #due="{ row }"><ElTag :type="row.daysRemaining < 0 ? 'danger' : 'warning'" effect="plain">{{ due(row.daysRemaining) }}</ElTag></template>
        <template #actions="{ row }"><ElButton text type="primary" @click="open(row)">查看合同</ElButton></template>
      </pure-table>
      <PageNav v-if="result" :page="page" :total="result.total" :page-size="result.pageSize" label="到期提醒分页" :disabled="loading" @change="page = $event" />
    </section>
  </section>
</template>
