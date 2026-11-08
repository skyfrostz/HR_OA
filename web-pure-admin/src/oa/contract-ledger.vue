<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from "vue";
import { ElButton, ElTag } from "element-plus";
import { listPage } from "./api";
import { findAcrossPages } from "./business";
import { contractStatuses } from "./format";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import AttachmentClassification from "./attachment-classification.vue";
import AttachmentForm from "./attachment-form.vue";
import ContractFiles from "./contract-files.vue";
import ContractForm from "./contract-form.vue";
import AuditHistory from "./audit-history.vue";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const props = defineProps({
  employeeId: { type: Number, required: true },
  contractId: Number,
  unavailable: Boolean
});
const emit = defineEmits(["editing", "changed"]);
const session = useSession();
const focusedHeading = useTemplateRef("focusedHeading");
const page = shallowRef(1);
const edit = shallowRef(null);
let returnFocus;
const feedback = shallowRef("");
const selected = shallowRef(props.contractId || null);
const revision = shallowRef(0);
const path = "/employees/" + props.employeeId + "/contracts";
const list = useRemote(signal =>
  session.authorized(listPage(path, page.value), { signal })
);
const detail = useRemote(async signal => {
  const contract = await session.authorized("/contracts/" + selected.value, {
    signal
  });
  if (contract.employeeId !== props.employeeId)
    throw new Error("这份合同不属于当前员工。");
  return contract;
});
const { result, loading, error, load } = list;
const detailRow = computed(() =>
  detail.result.value?.id === selected.value ? detail.result.value : null
);

watch(page, load, { immediate: true });
watch(
  selected,
  async id => {
    if (!id) return;
    await detail.load();
    await nextTick();
    focusedHeading.value?.focus();
    focusedHeading.value?.scrollIntoView?.({ block: "nearest" });
  },
  { immediate: true }
);

function startEdit(kind, contract = null, file = null) {
  if (edit.value || props.unavailable || !session.canWrite) return;
  feedback.value = "";
  returnFocus = document.activeElement;
  edit.value = { kind, contract, file };
  emit("editing", true);
}
function stopEdit() {
  edit.value = null;
  emit("editing", false);
  void nextTick(() => {
    if (returnFocus?.isConnected) returnFocus.focus();
    else focusedHeading.value?.focus();
  });
}

async function refreshRecords(contractNo = "") {
  const checks = [load()];
  if (selected.value) checks.push(detail.load());
  if (edit.value?.contract && edit.value.contract.id !== selected.value) {
    const id = edit.value.contract.id;
    checks.push(
      session.authorized("/contracts/" + id).then(
        () => true,
        () => false
      )
    );
  }
  let matched;
  if (contractNo) {
    checks.push(
      findAcrossPages(
        current =>
          session.authorized(path + "?page=" + current + "&pageSize=100"),
        record => record.contractNo === contractNo
      ).then(
        row => {
          matched = row;
          return true;
        },
        () => false
      )
    );
  }
  const refreshed = (await Promise.all(checks)).every(Boolean);
  if (refreshed && matched) selected.value = matched.id;
  if (!refreshed) feedback.value = "未能核对合同，请重新读取。";
  return refreshed;
}

async function saved(contract, message) {
  stopEdit();
  feedback.value = message;
  revision.value++;
  emit("changed");
  if (contract?.id && selected.value !== contract.id)
    selected.value = contract.id;
  else if (selected.value) void detail.load();
  if (page.value === 1) await load();
  else page.value = 1;
}

function reconciled() {
  stopEdit();
  revision.value++;
  emit("changed");
  feedback.value =
    "已重新读取合同，请核对当前状态及变更历史，再决定是否需要操作。";
}
</script>

<template>
  <section class="oa-surface" aria-labelledby="contracts-heading">
    <div v-if="!edit" class="oa-section-head">
      <div>
        <h2 id="contracts-heading">劳动合同</h2>
        <p class="oa-muted">查看合同期限、续约关系和私有附件。</p>
      </div>
      <ElButton
        v-if="session.canWrite"
        type="primary"
        :disabled="Boolean(edit) || unavailable"
        @click="startEdit('create')"
        >新建合同</ElButton
      >
    </div>
    <ContractForm
      v-if="edit && !['attachment', 'classification'].includes(edit.kind)"
      :path="path"
      :kind="edit.kind"
      :contract="edit.contract"
      :refresh="refreshRecords"
      @cancel="stopEdit"
      @saved="saved"
      @reconciled="reconciled"
    />
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>

    <section
      v-show="!edit"
      v-if="selected"
      class="oa-contract oa-contract-spotlight"
      aria-labelledby="oa-focused-contract"
    >
      <header class="oa-section-head">
        <div>
          <p class="oa-muted">合同详情</p>
          <h3 id="oa-focused-contract" ref="focusedHeading" tabindex="-1">
            {{ detailRow?.contractNo || "查看合同" }}
          </h3>
        </div>
        <ElButton
          :disabled="Boolean(edit) || unavailable"
          @click="selected = null"
          >关闭合同详情</ElButton
        >
      </header>
      <RemoteState
        :loading="detail.loading.value"
        :error="detail.error.value"
        @retry="detail.load"
      />
      <template v-if="detailRow">
        <div class="oa-contract-summary">
          <ElTag
            :type="contractStatuses[detailRow.status]?.type"
            effect="plain"
            >{{ contractStatuses[detailRow.status]?.label }}</ElTag
          >
          <span>{{ detailRow.startDate }} 至 {{ detailRow.endDate }}</span>
          <span v-if="detailRow.version" class="oa-muted"
            >版本 {{ detailRow.version }}</span
          >
        </div>
        <dl class="oa-description oa-account-summary">
          <div>
            <dt>签署日期</dt>
            <dd>{{ detailRow.signedDate || "未登记" }}</dd>
          </div>
          <div>
            <dt>终止日期</dt>
            <dd>{{ detailRow.terminatedDate || "未登记" }}</dd>
          </div>
          <div v-if="detailRow.previousContractId">
            <dt>续约来源</dt>
            <dd>
              <ElButton
                text
                type="primary"
                :disabled="Boolean(edit) || unavailable"
                @click="selected = detailRow.previousContractId"
                >查看原合同</ElButton
              >
            </dd>
          </div>
        </dl>
        <ContractFiles
          :files="detailRow.files || []"
          :unavailable="Boolean(edit) || unavailable"
          @classify="startEdit('classification', detailRow, $event)"
        />
        <div v-if="session.canWrite" class="oa-actions">
          <ElButton
            v-if="detailRow.status === 'DRAFT'"
            :disabled="Boolean(edit) || unavailable"
            @click="startEdit('edit', detailRow)"
            >编辑草稿</ElButton
          >
          <ElButton
            v-if="detailRow.status === 'DRAFT'"
            :disabled="Boolean(edit) || unavailable"
            @click="startEdit('sign', detailRow)"
            >登记签署</ElButton
          >
          <ElButton
            v-if="detailRow.status === 'SIGNED'"
            :disabled="Boolean(edit) || unavailable"
            @click="startEdit('terminate', detailRow)"
            >终止合同</ElButton
          >
          <ElButton
            :disabled="Boolean(edit) || unavailable"
            @click="startEdit('renew', detailRow)"
            >续约</ElButton
          >
          <ElButton
            :disabled="Boolean(edit) || unavailable"
            @click="startEdit('attachment', detailRow)"
            >添加附件</ElButton
          >
        </div>
        <AuditHistory
          :key="detailRow.id"
          :contract-id="detailRow.id"
          :revision="revision"
          title="合同状态与版本历史"
        />
      </template>
    </section>

    <AttachmentClassification
      v-if="edit?.kind === 'classification'"
      :file="edit.file"
      :refresh="refreshRecords"
      @cancel="stopEdit"
      @saved="saved(null, '附件访问范围已更新。')"
      @reconciled="reconciled"
    />
    <AttachmentForm
      v-if="edit?.kind === 'attachment'"
      :contract-id="edit.contract.id"
      :refresh="refreshRecords"
      @cancel="stopEdit"
      @uploaded="saved(null, '附件已上传。')"
    />

    <div v-show="!edit">
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
          v-for="contract in result.list.filter(
            row => row.id !== detailRow?.id
          )"
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
            :unavailable="Boolean(edit) || unavailable"
            @classify="startEdit('classification', contract, $event)"
          />
          <div class="oa-actions">
            <ElButton
              text
              type="primary"
              :disabled="loading || Boolean(edit) || unavailable"
              @click="selected = contract.id"
              >查看合同详情</ElButton
            >
            <template v-if="session.canWrite">
              <ElButton
                v-if="contract.status === 'DRAFT'"
                :disabled="loading || Boolean(edit) || unavailable"
                @click="startEdit('edit', contract)"
                >编辑草稿</ElButton
              >
              <ElButton
                v-if="contract.status === 'DRAFT'"
                :disabled="loading || Boolean(edit) || unavailable"
                @click="startEdit('sign', contract)"
                >登记签署</ElButton
              >
              <ElButton
                v-if="contract.status === 'SIGNED'"
                :disabled="loading || Boolean(edit) || unavailable"
                @click="startEdit('terminate', contract)"
                >终止合同</ElButton
              >
              <ElButton
                :disabled="loading || Boolean(edit) || unavailable"
                @click="startEdit('renew', contract)"
                >续约</ElButton
              >
              <ElButton
                :disabled="loading || Boolean(edit) || unavailable"
                @click="startEdit('attachment', contract)"
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
        :disabled="loading || Boolean(edit) || unavailable"
        label="劳动合同分页"
        @change="page = $event"
      />
    </div>
  </section>
</template>

<style scoped>
.oa-contract {
  padding: 16px 0;
  border: 0;
  border-top: 1px solid var(--oa-line);
  border-radius: 0;
}

.oa-contract:first-child {
  border-top: 0;
}

.oa-contract-spotlight {
  padding-top: 0;
  margin: 0;
  border-top: 0;
  border-left: 0;
}

.oa-contract-spotlight :deep(.oa-audit) {
  padding: 16px 0 0;
  border: 0;
  box-shadow: none;
}

.oa-contract-summary {
  margin: 12px 0;
}
</style>
