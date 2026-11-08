<script setup>
import {
  computed,
  nextTick,
  onScopeDispose,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { ElButton, ElTag } from "element-plus";
import { listPage } from "./api";
import { contractStatuses } from "./format";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import { findCreatedContract } from "./use-contract-record";
import ContractDetail from "./contract-detail.vue";
import ContractFiles from "./contract-files.vue";
import ContractForm from "./contract-form.vue";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const props = defineProps({
  employeeId: { type: Number, required: true },
  contractId: Number,
  active: { type: Boolean, default: true },
  unavailable: Boolean
});
const emit = defineEmits(["editing", "changed"]);
const session = useSession();
const detail = useTemplateRef("detail");
const heading = useTemplateRef("heading");
const selectedPanel = useTemplateRef("selectedPanel");
let selectionRevision = 0;
const page = shallowRef(1);
const creating = shallowRef(false);
const detailEditing = shallowRef(false);
const selected = shallowRef(props.contractId || null);
const queuedTask = shallowRef(null);
const feedback = shallowRef("");
let returnFocus;
const controller = new AbortController();
onScopeDispose(() => controller.abort());
const path = computed(() => "/employees/" + props.employeeId + "/contracts");
const { result, loading, error, load } = useRemote(signal =>
  session.authorized(listPage(path.value, page.value), { signal })
);
const editing = computed(() => creating.value || detailEditing.value);
watch(editing, value => emit("editing", value), { flush: "sync" });
watch([page, () => props.employeeId], load, { immediate: true });
watch(
  () => props.contractId,
  id => {
    if (id && !editing.value) {
      selectionRevision++;
      queuedTask.value = null;
      selected.value = id;
    }
  }
);
watch(
  () => [props.active, props.employeeId],
  () => {
    selectionRevision++;
  },
  { flush: "sync" }
);

async function select(id) {
  if (editing.value || props.unavailable || !props.active) return;
  const current = ++selectionRevision;
  queuedTask.value = null;
  selected.value = id;
  await nextTick();
  if (
    current !== selectionRevision ||
    selected.value !== id ||
    !props.active ||
    props.unavailable
  )
    return;
  selectedPanel.value?.focus({ preventScroll: true });
  selectedPanel.value?.scrollIntoView?.({ block: "nearest" });
}
function startTask(kind, contract = null, file = null) {
  if (editing.value || props.unavailable || !session.canWrite) return;
  feedback.value = "";
  returnFocus = document.activeElement;
  if (kind === "create") {
    creating.value = true;
    return;
  }
  if (selected.value === contract.id && detail.value?.startTask(kind, file))
    return;
  queuedTask.value = { contractId: contract.id, kind, file };
  selected.value = contract.id;
}
function detailLoaded(contract) {
  const task = queuedTask.value;
  if (task?.contractId !== contract.id) return;
  queuedTask.value = null;
  detail.value?.startTask(task.kind, task.file);
}
function stopCreate() {
  creating.value = false;
  void nextTick(() => {
    if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
    else heading.value?.focus({ preventScroll: true });
  });
}
async function closeSelected() {
  if (await detail.value?.requestClose()) {
    selectionRevision++;
    detail.value?.cancelPending();
    selected.value = null;
    queuedTask.value = null;
  }
}
async function refreshCreation(contractNo = "", intent) {
  if (contractNo && intent?.kind === "create" && intent.blocked === "unknown") {
    return findCreatedContract(
      session.authorized,
      {
        employeeId: props.employeeId,
        contractNo,
        ...intent.expected
      },
      controller.signal
    );
  }
  return load();
}
async function created(contract, message) {
  stopCreate();
  feedback.value = message;
  if (contract?.id) selected.value = contract.id;
  emit("changed");
  if (page.value === 1) await load();
  else page.value = 1;
}
function creationReconciled() {
  stopCreate();
  feedback.value = "已重新读取，请核对现有合同后再操作。";
}
function detailChanged(change) {
  emit("changed", change);
  void load();
}
</script>

<template>
  <section class="oa-surface" aria-labelledby="contracts-heading">
    <div v-if="!editing" class="oa-section-head">
      <div>
        <h2 id="contracts-heading" ref="heading" tabindex="-1">劳动合同</h2>
        <p class="oa-muted">查看合同期限、续约关系和私有附件。</p>
      </div>
      <ElButton
        v-if="session.canWrite"
        type="primary"
        :disabled="unavailable"
        @click="startTask('create')"
        >新建合同</ElButton
      >
    </div>
    <ContractForm
      v-if="creating"
      :path="path"
      kind="create"
      :refresh="refreshCreation"
      @cancel="stopCreate"
      @saved="created"
      @reconciled="creationReconciled"
    />
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <div
      v-if="selected && !creating"
      ref="selectedPanel"
      class="oa-selected-contract"
      tabindex="-1"
    >
      <div v-if="!detailEditing" class="oa-actions oa-close-contract">
        <ElButton :disabled="unavailable" @click="closeSelected"
          >关闭合同详情</ElButton
        >
      </div>
      <ContractDetail
        ref="detail"
        :contract-id="selected"
        :expected-employee-id="employeeId"
        :active="active && !unavailable"
        @select="select"
        @editing="detailEditing = $event"
        @changed="detailChanged"
        @loaded="detailLoaded"
      />
    </div>
    <div v-show="!editing">
      <RemoteState
        :loading="loading"
        :error="error"
        :empty="Boolean(result && !result.list?.length)"
        empty-text="暂无劳动合同。"
        @retry="load"
      />
      <div
        v-if="result?.list?.length"
        class="oa-contracts"
        :aria-busy="loading"
      >
        <article
          v-for="contract in result.list.filter(row => row.id !== selected)"
          :key="contract.id"
          class="oa-contract"
        >
          <div class="oa-section-head">
            <div>
              <h3>{{ contract.contractNo }}</h3>
              <p class="oa-muted">
                {{ contract.startDate }} 至 {{ contract.endDate }}
              </p>
            </div>
            <ElTag
              :type="contractStatuses[contract.status]?.type"
              effect="plain"
              >{{
                contractStatuses[contract.status]?.label || "未知状态"
              }}</ElTag
            >
          </div>
          <p v-if="contract.previousContractId" class="oa-muted">
            续约来源：{{
              result.list.find(item => item.id === contract.previousContractId)
                ?.contractNo || "记录 #" + contract.previousContractId
            }}
          </p>
          <ContractFiles
            :files="contract.files || []"
            :unavailable="unavailable || loading"
            @classify="startTask('classification', contract, $event)"
          />
          <div class="oa-actions">
            <ElButton
              text
              type="primary"
              :disabled="loading || unavailable"
              @click="select(contract.id)"
              >查看合同详情</ElButton
            >
            <template v-if="session.canWrite">
              <ElButton
                v-if="contract.status === 'DRAFT'"
                :disabled="loading || unavailable"
                @click="startTask('edit', contract)"
                >编辑草稿</ElButton
              >
              <ElButton
                v-if="contract.status === 'DRAFT'"
                :disabled="loading || unavailable"
                @click="startTask('sign', contract)"
                >登记签署</ElButton
              >
              <ElButton
                v-if="contract.status === 'SIGNED'"
                :disabled="loading || unavailable"
                @click="startTask('terminate', contract)"
                >终止合同</ElButton
              >
              <ElButton
                :disabled="loading || unavailable"
                @click="startTask('renew', contract)"
                >续约</ElButton
              >
              <ElButton
                :disabled="loading || unavailable"
                @click="startTask('attachment', contract)"
                >添加附件</ElButton
              >
            </template>
          </div>
        </article>
      </div>
      <PageNav
        v-if="result"
        :page="page"
        :total="result.total"
        :page-size="result.pageSize"
        :disabled="loading || editing || unavailable"
        label="劳动合同分页"
        @change="page = $event"
      />
    </div>
  </section>
</template>

<style scoped>
.oa-contract {
  padding: 16px 0;
  border-top: 1px solid var(--oa-line);
}

.oa-contract:first-child {
  border-top: 0;
}

.oa-selected-contract {
  margin-block: 16px;
}

.oa-close-contract {
  justify-content: flex-end;
  margin-bottom: 8px;
}
</style>
