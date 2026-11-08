<script setup>
import { computed, onMounted, shallowRef, useTemplateRef } from "vue";
import { useRouter } from "vue-router";
import { ElAlert, ElButton, ElStatistic } from "element-plus";
import {
  ArrowRight,
  Calendar,
  Checked,
  Document,
  Warning,
  OfficeBuilding,
  Setting
} from "@element-plus/icons-vue";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import { employeeNavigation } from "./navigation";
import RemoteState from "./remote-state.vue";
import DashboardQueue from "./dashboard-queue.vue";
const session = useSession();
const heading = useTemplateRef("heading");
onMounted(() => heading.value?.focus());
const router = useRouter();
const icons = {
  pendingTasks: Checked,
  myRequests: Document,
  contractReminders: Calendar,
  workflowExceptions: Warning,
  approvedLeaves: Checked
};
const { result, loading, error, load } = useRemote(signal =>
  session.authorized("/dashboard?days=30", { signal })
);
const sections = computed(() => {
  if (!result.value) return [];
  const data = result.value;
  return [
    ...(session.account?.permissions.canApproveRequests
      ? [
          {
            key: "pendingTasks",
            label: "待我审批",
            path: "/approvals",
            view: "pending",
            note: "查看当前分配给你的待办"
          }
        ]
      : []),
    ...(session.account?.permissions.canSubmitRequests
      ? [
          {
            key: "myRequests",
            label: "我的申请",
            path: "/approvals",
            view: "mine",
            note: "查看本人提交的全部申请"
          }
        ]
      : []),
    {
      key: "contractReminders",
      label: "合同到期",
      path: "/contract-reminders",
      note: "逾期及未来 30 天内到期"
    },
    ...(session.account?.user.role === "HR_ADMIN"
      ? [
          {
            key: "workflowExceptions",
            label: "审批异常",
            path: "/workflow-exceptions",
            note: "检查不可用的当前或后续处理人"
          },
          {
            key: "approvedLeaves",
            label: "已批请假",
            path: "/approved-leaves",
            note: "查看可撤销的新版已批请假"
          }
        ]
      : [])
  ]
    .filter(item => typeof data[item.key] === "number")
    .map(item => ({ ...item, value: data[item.key] }));
});
const opening = shallowRef(false);
const navigationError = shallowRef("");
async function open(item) {
  if (opening.value) return;
  opening.value = true;
  navigationError.value = "";
  const navigation = employeeNavigation();
  const intended = item.view || null;
  navigation.approvalView = intended;
  const clear = () => {
    if (navigation.approvalView === intended) navigation.approvalView = null;
  };
  try {
    if (await router.push(item.path)) {
      clear();
      navigationError.value = "未能打开目标页面，请重试。";
    }
  } catch {
    clear();
    navigationError.value = "未能打开目标页面，请重试。";
  } finally {
    opening.value = false;
  }
}
void load();
</script>
<template>
  <section class="oa-pure" aria-labelledby="dashboard-heading">
    <header class="oa-section-head oa-page-heading">
      <div>
        <h1 id="dashboard-heading" ref="heading" tabindex="-1">工作台</h1>
        <p class="oa-muted">
          {{ session.account?.user.displayName }}，从这里查看需要处理的事项。
        </p>
      </div>
      <ElButton :disabled="loading" @click="load">刷新工作台</ElButton>
    </header>
    <RemoteState :loading="loading" :error="error" @retry="load" />
    <ElAlert
      v-if="navigationError"
      :title="navigationError"
      type="error"
      :closable="false"
    />
    <div v-if="!loading && result" class="oa-dashboard-grid">
      <section
        v-for="item in sections"
        :key="item.key"
        class="oa-surface oa-dashboard-section"
      >
        <span class="oa-dashboard-icon" aria-hidden="true"
          ><component :is="icons[item.key]"
        /></span>
        <ElStatistic :title="item.label" :value="item.value" />
        <p class="oa-muted">{{ item.note }}</p>
        <ElButton
          text
          class="oa-dashboard-open"
          :disabled="opening"
          @click="open(item)"
          >查看{{ item.label }}<ArrowRight aria-hidden="true"
        /></ElButton>
      </section>
    </div>
    <div class="oa-dashboard-queues">
      <DashboardQueue
        v-if="session.account?.permissions.canApproveRequests"
        kind="tasks"
        title="待我处理的申请"
      />
      <DashboardQueue
        v-if="session.account?.permissions.canSubmitRequests"
        kind="requests"
        title="最近的申请"
      />
      <DashboardQueue kind="contracts" title="即将到期与逾期合同" />
      <DashboardQueue
        v-if="session.account?.user.role === 'HR_ADMIN'"
        kind="exceptions"
        title="需要交接的审批"
      />
    </div>
    <section
      class="oa-surface oa-dashboard-shortcuts"
      aria-labelledby="dashboard-shortcuts"
    >
      <h2 id="dashboard-shortcuts">常用入口</h2>
      <div class="oa-actions">
        <ElButton :icon="OfficeBuilding" @click="router.push('/employees')"
          >员工档案</ElButton
        >
        <ElButton
          v-if="session.account?.user.employeeId"
          :icon="Calendar"
          @click="router.push('/my-leave')"
          >我的假期与流水</ElButton
        >
        <ElButton
          v-if="session.account?.user.role === 'HR_ADMIN'"
          :icon="Setting"
          @click="router.push('/workflow-settings')"
          >请假与审批配置</ElButton
        >
      </div>
    </section>
  </section>
</template>
<style scoped>
.oa-dashboard-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 10rem), 1fr));
  gap: 1rem;
}

.oa-dashboard-section {
  position: relative;
  display: grid;
  grid-template-rows: auto 1fr auto;
  gap: 0.5rem;
  padding: 1rem;
}

.oa-dashboard-icon {
  position: absolute;
  top: 1rem;
  right: 1rem;
  display: grid;
  place-items: center;
  width: 2.5rem;
  height: 2.5rem;
  color: var(--oa-accent-ink);
  background: var(--oa-tint);
  border-radius: 0.625rem;
}

.oa-dashboard-icon svg {
  width: 1.25rem;
  height: 1.25rem;
}

.oa-dashboard-section :deep(.el-statistic__head) {
  padding-inline-end: 3rem;
  margin-bottom: 0.25rem;
  font-size: 0.9375rem;
  color: var(--el-text-color-regular);
}

.oa-dashboard-section :deep(.el-statistic__content) {
  font-size: 1.75rem;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
  line-height: 1.15;
  color: var(--el-text-color-primary);
  letter-spacing: -0.035em;
}

.oa-dashboard-section > .oa-muted {
  margin: 0.25rem 0 0.5rem;
  font-size: 0.875rem;
}

.oa-dashboard-section .oa-dashboard-open {
  width: 100%;
  padding: 0.5rem 0;
  margin: 0;
  color: var(--oa-accent-text);
  border-top: 1px solid var(--oa-line);
  border-radius: 0;
}

.oa-dashboard-open :deep(span) {
  display: flex;
  justify-content: space-between;
  width: 100%;
}

.oa-dashboard-open svg {
  width: 1rem;
  height: 1rem;
}

.oa-dashboard-queues {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 25rem), 1fr));
  gap: 1rem;
  margin-top: 1rem;
}

.oa-dashboard-shortcuts {
  margin-top: 1rem;
}

.oa-dashboard-shortcuts .oa-actions {
  gap: 0.75rem;
  margin-top: 1rem;
}

@media (width <= 38rem) {
  .oa-dashboard-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.5rem;
  }

  .oa-dashboard-section > .oa-muted,
  .oa-dashboard-icon {
    display: none;
  }

  .oa-dashboard-section :deep(.el-statistic__head) {
    padding-inline-end: 0;
  }

  .oa-dashboard-section :deep(.el-statistic__content) {
    font-size: 1.5rem;
  }

  .oa-dashboard-section {
    padding: 1rem;
  }

  .oa-dashboard-icon {
    top: 1.25rem;
    right: 1.25rem;
  }
}
</style>
