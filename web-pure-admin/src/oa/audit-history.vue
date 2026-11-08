<script setup>
import { computed, shallowRef, watch } from "vue";
import { ElButton, ElTag } from "element-plus";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import { formatSalary, contractStatuses } from "./format";
import { listPage } from "./api";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const props = defineProps({
  employeeId: Number,
  contractId: Number,
  departmentId: Number,
  revision: Number,
  title: { type: String, default: "变更历史" }
});
const session = useSession();
const page = shallowRef(1);
const type = shallowRef("");
const labels = {
  SALARY: "约定工资",
  DEPARTMENT: "部门",
  ANNUAL_LEAVE: "年假",
  CONTRACT: "合同",
  ACCOUNT: "账号",
  EMPLOYMENT: "在职状态"
};
const actions = {
  CREATE: "新增",
  UPDATE: "更新",
  IMPORT: "导入",
  RENEW: "续约",
  SIGN: "登记签署",
  TERMINATE: "终止",
  BIND: "绑定",
  SET_ROLE: "变更角色",
  RESET_PASSWORD: "重置密码",
  DISABLE: "停用",
  ENABLE: "启用",
  OFFBOARD: "办理离职"
};
const fields = {
  SALARY: { salaryCents: "约定工资" },
  DEPARTMENT: {
    name: "名称",
    departmentId: "部门编号",
    department: "部门",
    parentId: "上级部门编号"
  },
  ANNUAL_LEAVE: {
    year: "年度",
    entitledHalfDays: "应享",
    usedHalfDays: "已用"
  },
  CONTRACT: {
    contractNo: "编号",
    startDate: "开始日期",
    endDate: "结束日期",
    status: "状态",
    version: "版本",
    previousContractId: "续约来源编号",
    signedDate: "签署日期",
    terminatedDate: "终止日期"
  },
  ACCOUNT: {
    userId: "账号编号",
    role: "角色",
    enabled: "启用状态",
    handoffUserId: "交接账号编号"
  },
  EMPLOYMENT: { employmentStatus: "在职状态" }
};
const visibleTypes = computed(() =>
  Object.entries(labels).filter(
    ([id]) => session.canReadSalary || id !== "SALARY"
  )
);
const timestamp = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Asia/Shanghai"
});
const { result, loading, error, load } = useRemote(signal => {
  const query = new URLSearchParams();
  for (const key of ["employeeId", "contractId", "departmentId"])
    if (props[key]) query.set(key, props[key]);
  if (type.value) query.set("type", type.value);
  return session.authorized(
    listPage("/audit-events", page.value) + "&" + query,
    { signal }
  );
});
const visible = computed(() =>
  (result.value?.list || []).filter(
    event => session.canReadSalary || event.type !== "SALARY"
  )
);
watch(
  [
    page,
    type,
    () => props.revision,
    () => props.employeeId,
    () => props.contractId,
    () => props.departmentId
  ],
  load,
  { immediate: true }
);

function value(key, raw) {
  if (raw == null) return "—";
  if (key === "salaryCents") return formatSalary(raw);
  if (key === "employmentStatus") return raw === "ACTIVE" ? "在职" : "已离职";
  if (key === "role")
    return { EMPLOYEE: "员工", APPROVER: "审批人" }[raw] || raw;
  if (key === "enabled") return raw ? "启用" : "停用";
  if (key === "status") return contractStatuses[raw]?.label || raw;
  if (key.endsWith("HalfDays")) return raw / 2 + " 天";
  return raw;
}
function changes(event, all = false) {
  return Object.entries(fields[event.type] || {})
    .filter(
      ([key]) =>
        ((event.before && Object.hasOwn(event.before, key)) ||
          (event.after && Object.hasOwn(event.after, key))) &&
        (all || !Object.is(event.before?.[key], event.after?.[key]))
    )
    .map(([key, label]) => ({
      label,
      before: value(key, event.before?.[key]),
      after: value(key, event.after?.[key])
    }));
}
</script>

<template>
  <section class="oa-surface oa-audit" :aria-label="title">
    <header class="oa-section-head">
      <div>
        <h2>{{ title }}</h2>
        <p class="oa-muted">按发生时间查看已记录的业务变更。</p>
      </div>
      <div class="oa-actions">
        <label v-if="!contractId && !departmentId" class="oa-filter"
          >变更类型<select v-model="type" @change="page = 1">
            <option value="">全部类型</option>
            <option v-for="[id, label] in visibleTypes" :key="id" :value="id">
              {{ label }}
            </option>
          </select></label
        >
        <ElButton :disabled="loading" @click="load">刷新历史</ElButton>
      </div>
    </header>
    <RemoteState
      :loading="loading"
      :error="error"
      :empty="Boolean(result && !visible.length)"
      empty-text="暂无符合条件的变更记录。"
      @retry="load"
    />
    <ol v-if="!loading && visible.length" class="oa-audit-timeline">
      <li v-for="event in visible" :key="event.id">
        <div class="oa-section-head">
          <strong
            >{{ actions[event.action] || event.action }} ·
            {{ labels[event.type] || event.type }}</strong
          >
          <time class="oa-muted" :datetime="event.createdAt">{{
            timestamp.format(new Date(event.createdAt))
          }}</time>
        </div>
        <p class="oa-muted">操作账号 #{{ event.actorId }}</p>
        <p v-if="event.reason" class="oa-audit-reason">{{ event.reason }}</p>
        <dl v-if="changes(event).length" class="oa-description">
          <div v-for="item in changes(event)" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>
              {{ item.before }}<span aria-label="变更为"> → </span
              >{{ item.after }}
            </dd>
          </div>
        </dl>
        <p v-else class="oa-muted">未记录字段值变化。</p>
        <details class="oa-audit-full">
          <summary>查看完整记录</summary>
          <dl class="oa-description">
            <div v-for="item in changes(event, true)" :key="item.label">
              <dt>{{ item.label }}</dt>
              <dd>
                {{ item.before }}<span aria-label="变更为"> → </span
                >{{ item.after }}
              </dd>
            </div>
          </dl>
        </details>
      </li>
    </ol>
    <PageNav
      v-if="result"
      :page="page"
      :total="result.total"
      :page-size="result.pageSize"
      :disabled="loading"
      label="变更历史分页"
      @change="page = $event"
    />
  </section>
</template>

<style scoped>
.oa-audit-full {
  margin-top: 12px;
}

.oa-audit-full summary {
  width: fit-content;
  color: var(--el-color-primary);
  cursor: pointer;
}

.oa-audit-full summary:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 4px;
}

.oa-audit-full .oa-description {
  margin-top: 12px;
}

.oa-audit-timeline > li {
  padding: 16px 0;
}
</style>
