<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from "vue";
import { ElButton } from "element-plus";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import { useVersionedWrite } from "./use-versioned-write";
import { useRouteWriteLock } from "./use-route-write-lock";
import { typeLabels } from "./approvals/approval";
import RemoteState from "./remote-state.vue";
import PageNav from "./page-nav.vue";
const session = useSession();
const page = shallowRef(1);
const candidatePage = shallowRef(1);
const selected = shallowRef(null);
const reason = shallowRef("");
const targetId = shallowRef(null);
const validation = shallowRef("");
const feedback = shallowRef("");
const heading = useTemplateRef("heading");
const listHeading = useTemplateRef("listHeading");
const {
  result,
  loading,
  error: readError,
  load
} = useRemote(signal =>
  session.authorized(
    "/workflow-exceptions?page=" + page.value + "&pageSize=20",
    { signal }
  )
);
const candidates = useRemote(signal =>
  session.authorized(
    "/approval-rules/assignees?page=" + candidatePage.value + "&pageSize=20",
    { signal }
  )
);
const write = useVersionedWrite();
const { busy, blocked, error } = write;
const disabled = computed(() => busy.value || Boolean(blocked.value));
useRouteWriteLock(computed(() => Boolean(selected.value)));
watch(page, load, { immediate: true });
watch(candidatePage, () => {
  if (selected.value) void candidates.load();
});
watch(result, data => {
  if (data && page.value > Math.max(1, Math.ceil(data.total / data.pageSize)))
    page.value = Math.max(1, Math.ceil(data.total / data.pageSize));
});
function begin(row) {
  if (disabled.value) return;
  selected.value = row;
  reason.value = "";
  targetId.value = null;
  validation.value = "";
  error.value = "";
  if (row) {
    candidatePage.value = 1;
    void candidates.load();
    void nextTick(() => heading.value?.focus());
  } else {
    void nextTick(() => listHeading.value?.focus());
  }
}
async function submit() {
  const note = reason.value.trim();
  if (!note || [...note].length > 500 || !targetId.value) {
    validation.value = "请选择交接审批人并填写 1–500 字的原因。";
    return;
  }
  const saved = await write.send(
    "/workflow-exceptions/" + selected.value.taskId + "/handoff",
    "POST",
    {
      version: selected.value.version,
      step: selected.value.step,
      targetUserId: targetId.value,
      reason: note
    }
  );
  if (saved) {
    begin(null);
    feedback.value = "交接已保存，已重新检查异常任务。";
    await load();
  }
}
async function reconcile() {
  await write.reconcile(load, () => {
    begin(null);
    feedback.value =
      "已重新检查异常。请核对当前处理人，确认原交接是否已经生效。";
  });
}
</script>
<template>
  <section class="oa-pure" aria-labelledby="exceptions-heading">
    <header class="oa-section-head oa-page-heading">
      <div>
        <h1 id="exceptions-heading" ref="listHeading" tabindex="-1">
          审批异常
        </h1>
        <p class="oa-muted">
          每条申请显示最早的异常步骤；交接保留原始审批路径和操作记录。
        </p>
      </div>
      <ElButton :disabled="Boolean(selected) || loading" @click="load"
        >重新检查</ElButton
      >
    </header>
    <section class="oa-surface">
      <RemoteState
        :loading="loading"
        :error="readError"
        :empty="Boolean(result && !result.list.length)"
        empty-text="当前没有检测到不可用的审批步骤。"
        @retry="load"
      />
      <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
      <ul v-if="result && !selected" class="oa-settings-list">
        <li v-for="row in result.list" :key="row.taskId + '-' + row.step">
          <div>
            <strong
              >{{ typeLabels[row.requestType] }} #{{ row.requestId }} · 第
              {{ row.step }} 步</strong
            >
            <p class="oa-muted">
              {{ row.stepName }} ·
              {{
                row.reason === "CURRENT_UNAVAILABLE"
                  ? "当前处理人不可用"
                  : "后续处理人不可用"
              }}
              · 账号 #{{ row.assigneeId }}
            </p>
          </div>
          <ElButton :disabled="Boolean(selected)" @click="begin(row)"
            >处理异常</ElButton
          >
        </li>
      </ul>
      <PageNav
        v-if="result && !selected"
        :page="page"
        :total="result.total"
        :page-size="result.pageSize"
        :disabled="Boolean(selected) || loading"
        label="异常任务分页"
        @change="page = $event"
      />
      <form
        v-if="selected"
        class="oa-form oa-exception-task"
        aria-describedby="exception-error"
        @submit.prevent="submit"
      >
        <h2 ref="heading" tabindex="-1">
          交接第 {{ selected.step }} 步 · {{ selected.stepName }}
        </h2>
        <p class="oa-muted">
          {{
            selected.reason === "FUTURE_UNAVAILABLE"
              ? "修改后续步骤的实际处理人，不改写原审批路径。"
              : "重新分配当前待办。"
          }}
        </p>
        <RemoteState
          :loading="candidates.loading.value"
          :error="candidates.error.value"
          @retry="candidates.load"
        />
        <label
          >交接给<select
            v-model="targetId"
            :disabled="disabled || candidates.loading.value"
            required
          >
            <option :value="null" disabled>请选择审批人</option>
            <option
              v-for="person in candidates.result.value?.list || []"
              :key="person.id"
              :value="person.id"
            >
              {{ person.displayName }}
            </option>
          </select></label
        >
        <PageNav
          v-if="candidates.result.value"
          :page="candidatePage"
          :total="candidates.result.value.total"
          :page-size="candidates.result.value.pageSize"
          :disabled="disabled || candidates.loading.value"
          label="交接候选分页"
          @change="
            targetId = null;
            candidatePage = $event;
          "
        />
        <label
          >交接原因<textarea
            v-model="reason"
            maxlength="500"
            required
            :disabled="disabled"
          />
        </label>
        <p v-if="validation || error" id="exception-error" role="alert">
          {{ validation || error }}
        </p>
        <div class="oa-actions">
          <ElButton v-if="blocked" :loading="busy" @click="reconcile"
            >重新读取并核对</ElButton
          ><ElButton v-else type="primary" native-type="submit" :loading="busy"
            >确认交接</ElButton
          ><ElButton :disabled="disabled" @click="begin(null)">取消</ElButton>
        </div>
      </form>
    </section>
  </section>
</template>

<style scoped>
.oa-exception-task {
  max-width: 800px;
}
</style>
