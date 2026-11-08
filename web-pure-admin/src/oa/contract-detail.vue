<script setup lang="ts">
import {
  computed,
  nextTick,
  onScopeDispose,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { ElButton, ElMessageBox, ElTag, type TagProps } from "element-plus";
import { useSession } from "./session";
import { contractStatuses } from "./format";
import {
  findCreatedContract,
  useContractRecord,
  type ContractChange,
  type ContractRecord,
  type ContractTaskState
} from "./use-contract-record";
import ContractForm from "./contract-form.vue";
import ContractFiles from "./contract-files.vue";
import AttachmentForm from "./attachment-form.vue";
import AttachmentClassification from "./attachment-classification.vue";
import AuditHistory from "./audit-history.vue";
import RemoteState from "./remote-state.vue";

const props = withDefaults(
  defineProps<{
    contractId: number;
    expectedEmployeeId: number;
    active?: boolean;
  }>(),
  { active: true }
);
const emit = defineEmits<{
  changed: [change: ContractChange];
  select: [contractId: number];
  loaded: [contract: ContractRecord];
  editing: [editing: boolean];
  "state-change": [state: ContractTaskState];
}>();
const session = useSession();
const heading = useTemplateRef("heading");
const { record, loading, error, load, cancel } = useContractRecord(
  () => ({
    contractId: props.contractId,
    employeeId: props.expectedEmployeeId,
    active: props.active
  }),
  session.authorized
);
type Task = {
  kind: string;
  contract: ContractRecord;
  file?: Record<string, unknown>;
};
const edit = shallowRef<Task | null>(null);
const formState = shallowRef({ dirty: false, busy: false });
const feedback = shallowRef("");
const revision = shallowRef(0);
let returnFocus: HTMLElement | null = null;
let scope = new AbortController();
let closeCheck: Promise<boolean> | null = null;
const state = computed(() => ({
  editing: Boolean(edit.value),
  ...formState.value
}));
const statusType = computed(() =>
  record.value
    ? (contractStatuses[record.value.status]?.type as TagProps["type"])
    : undefined
);
watch(
  state,
  value => {
    emit("editing", value.editing);
    emit("state-change", value);
  },
  { immediate: true, flush: "sync" }
);
watch(record, value => {
  if (value && props.active) emit("loaded", value);
});
watch(
  () => [props.contractId, props.expectedEmployeeId, props.active],
  (value, previous) => {
    scope.abort();
    scope = new AbortController();
    if (!props.active && value[0] === previous[0] && value[1] === previous[1])
      return;
    edit.value = null;
    formState.value = { dirty: false, busy: false };
    feedback.value = "";
    returnFocus = null;
  },
  { flush: "sync" }
);
onScopeDispose(() => scope.abort());

function cancelPending() {
  scope.abort();
  cancel();
}

function startTask(kind: string, file?: Record<string, unknown>) {
  const current = record.value;
  if (
    !props.active ||
    loading.value ||
    edit.value ||
    !session.canWrite ||
    !current
  )
    return false;
  if (["edit", "sign"].includes(kind) && current.status !== "DRAFT")
    return false;
  if (kind === "terminate" && current.status !== "SIGNED") return false;
  if (
    ![
      "edit",
      "sign",
      "terminate",
      "renew",
      "attachment",
      "classification"
    ].includes(kind)
  )
    return false;
  if (
    kind === "classification" &&
    (session.account?.user.role !== "HR_ADMIN" ||
      !file?.version ||
      !current.files?.some(item => item.id === file.id))
  )
    return false;
  returnFocus = document.activeElement as HTMLElement | null;
  formState.value = { dirty: false, busy: false };
  edit.value = { kind, contract: current, file };
  feedback.value = "";
  return true;
}

function stopTask(restoreFocus = true) {
  edit.value = null;
  formState.value = { dirty: false, busy: false };
  if (restoreFocus)
    void nextTick(() => {
      if (!props.active || edit.value) return;
      if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
      else heading.value?.focus({ preventScroll: true });
    });
}

function requestClose(): Promise<boolean> {
  if (formState.value.busy) return Promise.resolve(false);
  if (!formState.value.dirty) return Promise.resolve(true);
  if (closeCheck) return closeCheck;
  const currentScope = scope;
  closeCheck = ElMessageBox.confirm(
    "当前修改尚未保存。放弃后将关闭合同详情；已经提交但结果未知的操作不会被撤销。",
    "放弃未保存的修改？",
    {
      confirmButtonText: "放弃修改",
      cancelButtonText: "继续编辑",
      type: "warning",
      closeOnClickModal: false
    }
  )
    .then(
      () => !currentScope.signal.aborted && !formState.value.busy,
      () => false
    )
    .finally(() => {
      closeCheck = null;
    });
  return closeCheck;
}

async function refresh(
  contractNo = "",
  intent?: {
    kind: string;
    blocked: string;
    contract: ContractRecord;
    expected: { startDate: string; endDate: string };
  }
) {
  if (contractNo) {
    if (intent?.kind !== "renew" || intent.blocked !== "unknown") return null;
    return findCreatedContract(
      session.authorized,
      {
        employeeId: props.expectedEmployeeId,
        contractNo,
        ...intent.expected,
        previousContractId: intent.contract.id
      },
      scope.signal
    );
  }
  return load();
}

function saved(value?: ContractRecord | null, message = "合同已更新。") {
  if (!props.active || !edit.value) return;
  const task = edit.value;
  const id = value?.id || task.contract.id;
  const known =
    value?.employeeId === props.expectedEmployeeId ? value : undefined;
  stopTask(false);
  feedback.value = message;
  revision.value++;
  emit("changed", {
    contractId: id,
    employeeId: props.expectedEmployeeId,
    kind: task.kind,
    contract: known
  });
  if (id !== props.contractId) emit("select", id);
  else void load();
}

function reconciled() {
  if (!props.active || !edit.value) return;
  const kind = edit.value.kind;
  stopTask();
  feedback.value =
    "已重新读取，请核对当前合同、附件及变更记录，再决定是否操作。";
  revision.value++;
  emit("changed", {
    contractId: props.contractId,
    employeeId: props.expectedEmployeeId,
    kind,
    contract: record.value || undefined
  });
}

defineExpose({ requestClose, cancelPending, startTask });
</script>

<template>
  <section
    class="oa-contract-detail"
    aria-label="合同详情"
    :aria-busy="loading"
    :inert="!active"
  >
    <template v-if="edit">
      <AttachmentForm
        v-if="edit.kind === 'attachment'"
        :contract-id="edit.contract.id"
        :refresh="refresh"
        @state-change="formState = $event"
        @cancel="stopTask()"
        @uploaded="saved(null, '附件已上传。')"
      />
      <AttachmentClassification
        v-else-if="edit.kind === 'classification'"
        :file="edit.file"
        :refresh="refresh"
        @state-change="formState = $event"
        @cancel="stopTask()"
        @saved="saved(null, '附件访问范围已更新。')"
        @reconciled="reconciled"
      />
      <ContractForm
        v-else
        :path="'/employees/' + expectedEmployeeId + '/contracts'"
        :kind="edit.kind"
        :contract="edit.contract"
        :refresh="refresh"
        @state-change="formState = $event"
        @cancel="stopTask()"
        @saved="saved"
        @reconciled="reconciled"
      />
    </template>
    <template v-else>
      <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
      <RemoteState :loading="loading" :error="error" @retry="load" />
      <template v-if="record">
        <header class="oa-section-head">
          <div>
            <h3 id="oa-focused-contract" ref="heading" tabindex="-1">
              {{ record.contractNo }}
            </h3>
          </div>
          <ElTag :type="statusType" effect="plain">{{
            contractStatuses[record.status]?.label || "未知状态"
          }}</ElTag>
        </header>
        <dl class="oa-description oa-contract-dates">
          <div>
            <dt>合同期限</dt>
            <dd>{{ record.startDate }} 至 {{ record.endDate }}</dd>
          </div>
          <div>
            <dt>版本</dt>
            <dd>{{ record.version }}</dd>
          </div>
          <div>
            <dt>签署日期</dt>
            <dd>{{ record.signedDate || "未登记" }}</dd>
          </div>
          <div>
            <dt>终止日期</dt>
            <dd>{{ record.terminatedDate || "未登记" }}</dd>
          </div>
          <div v-if="record.previousContractId">
            <dt>续约来源</dt>
            <dd>
              <ElButton
                text
                type="primary"
                @click="emit('select', record.previousContractId)"
                >查看原合同</ElButton
              >
            </dd>
          </div>
        </dl>
        <div v-if="session.canWrite" class="oa-actions oa-contract-actions">
          <ElButton v-if="record.status === 'DRAFT'" @click="startTask('edit')"
            >编辑草稿</ElButton
          >
          <ElButton v-if="record.status === 'DRAFT'" @click="startTask('sign')"
            >登记签署</ElButton
          >
          <ElButton
            v-if="record.status === 'SIGNED'"
            @click="startTask('terminate')"
            >终止合同</ElButton
          >
          <ElButton @click="startTask('renew')">续约</ElButton>
          <ElButton @click="startTask('attachment')">添加附件</ElButton>
        </div>
        <ContractFiles
          :key="record.id"
          :files="record.files || []"
          @classify="startTask('classification', $event)"
        />
        <AuditHistory
          :key="record.id"
          :contract-id="record.id"
          :revision="revision"
          title="合同状态与版本历史"
        />
      </template>
    </template>
  </section>
</template>

<style scoped>
.oa-contract-detail {
  min-width: 0;
}

.oa-contract-detail :deep(.oa-audit) {
  padding: 16px 0 0;
  border: 0;
  box-shadow: none;
}

.oa-contract-actions {
  margin-block: 16px;
}

.oa-contract-dates {
  margin-block: 12px;
}

.oa-contract-dates dd {
  overflow-wrap: anywhere;
}
</style>
