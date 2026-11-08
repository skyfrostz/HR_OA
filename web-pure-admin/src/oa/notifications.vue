<script setup>
import {
  onScopeDispose,
  onMounted,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { useRouter } from "vue-router";
import { ElAlert, ElButton, ElTag } from "element-plus";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import { employeeNavigation } from "./navigation";
import RemoteState from "./remote-state.vue";
import PageNav from "./page-nav.vue";
const session = useSession();
const router = useRouter();
const controller = new AbortController();
onScopeDispose(() => controller.abort());
const heading = useTemplateRef("heading");
onMounted(() => heading.value?.focus());
const page = shallowRef(1);
const pending = shallowRef(null);
const actionError = shallowRef("");
const { result, loading, error, load } = useRemote(signal =>
  session.authorized("/notifications?page=" + page.value + "&pageSize=20", {
    signal
  })
);
watch(page, load, { immediate: true });
const mode = {
  mock: "模拟",
  "wecom-mock": "模拟 API 已接受",
  wecom: "企业 API 已接受"
};
async function open(record) {
  if (pending.value !== null) return;
  pending.value = record.id;
  actionError.value = "";
  const nav = employeeNavigation();
  let intended = {};
  const clear = () => {
    if (Object.keys(intended).every(key => nav[key] === intended[key]))
      for (const key of Object.keys(intended)) nav[key] = null;
  };
  try {
    if (record.kind === "CONTRACT_EXPIRING" && record.contractId) {
      const contract = await session.authorized(
        "/contracts/" + record.contractId,
        { signal: controller.signal }
      );
      intended = { employeeId: contract.employeeId, contractId: contract.id };
      Object.assign(nav, intended);
      if (await router.push("/employees")) {
        clear();
        actionError.value = "未能打开详情，请重试。";
      }
    } else if (
      record.requestId > 0 &&
      (session.account?.permissions.canSubmitRequests ||
        session.account?.permissions.canApproveRequests)
    ) {
      intended = { requestId: record.requestId };
      Object.assign(nav, intended);
      if (await router.push("/approvals")) {
        clear();
        actionError.value = "未能打开详情，请重试。";
      }
    } else actionError.value = "此通知没有当前账号可打开的详情。";
  } catch (cause) {
    clear();
    actionError.value = cause.message || "未能打开详情，请重试。";
  } finally {
    pending.value = null;
  }
}
</script>
<template>
  <section class="oa-pure" aria-labelledby="oa-notifications-heading">
    <header class="oa-section-head oa-page-heading">
      <div>
        <h1 id="oa-notifications-heading" ref="heading" tabindex="-1">
          站内通知
        </h1>
        <p class="oa-muted">查看与你相关的业务事件并打开详情。</p>
      </div>
      <ElButton :disabled="loading" @click="load">刷新通知</ElButton>
    </header>
    <RemoteState
      :loading="loading"
      :error="error"
      :empty="Boolean(result && !result.list.length)"
      empty-text="暂无站内通知。"
      @retry="load"
    />
    <ElAlert
      v-if="actionError"
      :title="actionError"
      type="warning"
      :closable="false"
    />
    <ul v-if="!loading && result?.list.length" class="oa-notification-list">
      <li v-for="record in result.list" :key="record.id" class="oa-surface">
        <div class="oa-section-head">
          <h2>{{ record.title }}</h2>
          <ElTag
            v-if="
              record.deliveryMode === 'mock' ||
              record.deliveryMode === 'wecom-mock'
            "
            class="oa-delivery-status"
            effect="plain"
            type="info"
            >模拟</ElTag
          >
        </div>
        <p class="oa-notification-meta oa-muted">
          <span
            v-if="record.kind === 'CONTRACT_EXPIRING' && record.contractId > 0"
            class="oa-notification-object"
            >合同 #{{ record.contractId }}</span
          >
          <span v-else-if="record.requestId > 0" class="oa-notification-object"
            >申请 #{{ record.requestId }}</span
          >
          <time :datetime="record.createdAt">
            {{
              new Date(record.createdAt).toLocaleString("zh-CN", {
                hour12: false
              })
            }}
          </time>
        </p>
        <ElButton
          class="oa-notification-open"
          :disabled="pending !== null"
          :loading="pending === record.id"
          @click="open(record)"
          >{{
            record.kind === "CONTRACT_EXPIRING"
              ? "查看合同详情"
              : "查看申请详情"
          }}</ElButton
        >
        <details class="oa-notification-diagnostics">
          <summary>投递信息</summary>
          <p>
            {{ mode[record.deliveryMode] || "投递模式未知" }}。API
            接受不代表消息已送达或已读。
          </p>
        </details>
      </li>
    </ul>
    <PageNav
      v-if="result"
      :page="page"
      :total="result.total"
      :page-size="result.pageSize"
      :disabled="loading || pending !== null"
      label="站内通知分页"
      @change="page = $event"
    />
  </section>
</template>

<style scoped>
.oa-notification-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 1rem;
}

.oa-notification-object {
  color: var(--oa-text);
}

.oa-notification-diagnostics {
  margin-top: 0.5rem;
  color: var(--oa-muted);
}

.oa-notification-diagnostics p {
  margin-block: 0.25rem;
}

.oa-notification-list h2 {
  font-size: 1rem;
}
</style>
