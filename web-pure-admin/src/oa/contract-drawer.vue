<script setup lang="ts">
import { nextTick, shallowRef, useTemplateRef, watch } from "vue";
import { ElButton, ElDrawer } from "element-plus";
import ContractDetail from "./contract-detail.vue";
import type { ContractChange } from "./use-contract-record";
import { useContractScrollLock } from "./use-contract-scroll-lock";

const props = defineProps<{
  modelValue: boolean;
  contractId: number | null;
  expectedEmployeeId: number | null;
  employeeName?: string;
  employeeNo?: string;
}>();
const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  changed: [change: ContractChange];
  closed: [];
}>();
const scrollLock = useContractScrollLock();
const detail = useTemplateRef("detail");
const closeButton = useTemplateRef("closeButton");
const currentId = shallowRef<number | null>(null);
const busy = shallowRef(false);
const employeeId = shallowRef<number | null>(null);
let generation = 0;
let closeRequest: Promise<boolean> | null = null;

watch(
  () => [props.modelValue, props.contractId, props.expectedEmployeeId] as const,
  ([opened, id, expected]) => {
    generation++;
    if (opened) {
      scrollLock.acquire();
      busy.value = false;
      currentId.value = id;
      employeeId.value = expected;
    } else detail.value?.cancelPending();
  },
  { immediate: true, flush: "sync" }
);

function requestClose(): Promise<boolean> {
  if (!props.modelValue) return Promise.resolve(true);
  if (busy.value) return Promise.resolve(false);
  if (closeRequest) return closeRequest;
  const attempt = generation;
  closeRequest = (async () => {
    const accepted = (await detail.value?.requestClose()) ?? true;
    if (!accepted || busy.value || generation !== attempt) return false;
    detail.value?.cancelPending();
    emit("update:modelValue", false);
    return true;
  })().finally(() => {
    closeRequest = null;
  });
  return closeRequest;
}

async function beforeClose(done: () => void) {
  if (await requestClose()) done();
}

function select(id: number) {
  if (props.modelValue) currentId.value = id;
}

async function openedFocus() {
  const attempt = generation;
  await nextTick();
  if (!props.modelValue || generation !== attempt) return;
  const active = document.activeElement;
  if (
    active === document.body ||
    active?.classList.contains("el-drawer__sr-focus") ||
    active?.classList.contains("oa-contract-drawer")
  )
    closeButton.value?.$el.focus({ preventScroll: true });
}

function closed() {
  if (props.modelValue) return;
  scrollLock.release();
  currentId.value = null;
  employeeId.value = null;
  emit("closed");
}

defineExpose({ requestClose });
</script>

<template>
  <ElDrawer
    :model-value="modelValue"
    class="oa-pure oa-contract-drawer"
    modal-class="oa-contract-overlay"
    title="合同详情"
    size="min(55rem, 100vw)"
    append-to-body
    :lock-scroll="false"
    :show-close="false"
    :close-on-click-modal="false"
    :before-close="beforeClose"
    @opened="openedFocus"
    @closed="closed"
  >
    <template #header>
      <div class="oa-contract-drawer-heading">
        <h2 class="oa-contract-drawer-title">合同详情</h2>
        <p
          v-if="employeeName || employeeNo"
          class="oa-contract-drawer-employee"
        >
          <span>{{ employeeName }}</span
          ><span>{{ employeeNo }}</span>
        </p>
      </div>
      <ElButton ref="closeButton" :disabled="busy" @click="requestClose"
        >关闭</ElButton
      >
    </template>
    <ContractDetail
      v-if="currentId !== null && employeeId !== null"
      :key="`${employeeId}:${currentId}`"
      ref="detail"
      :contract-id="currentId"
      :expected-employee-id="employeeId"
      :active="modelValue"
      @select="select"
      @state-change="busy = $event.busy"
      @changed="emit('changed', $event)"
    />
  </ElDrawer>
</template>

<style>
body.oa-contract-scroll-locked {
  overflow: hidden;
}

.oa-contract-drawer.el-drawer {
  max-width: 100vw;
  transition: transform 180ms ease-out;
}

.oa-contract-overlay.el-drawer-fade-enter-active,
.oa-contract-overlay.el-drawer-fade-leave-active {
  transition-duration: 180ms;
}

.oa-contract-drawer .el-drawer__header {
  flex-shrink: 0;
  gap: 16px;
  padding: 16px 24px;
  margin: 0;
  color: var(--oa-text);
  border-bottom: 1px solid var(--oa-line);
}

.oa-contract-drawer-heading {
  flex: 1;
  min-width: 0;
}

.oa-contract-drawer-employee {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin: 4px 0 0;
  font-size: 14px;
  color: var(--oa-muted);
  overflow-wrap: anywhere;
}

.oa-contract-drawer-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.oa-contract-drawer .el-drawer__body {
  min-height: 0;
  padding: 24px;
  overflow: auto;
  overscroll-behavior: contain;
  background: var(--oa-surface);
}

@media (width <= 38rem) {
  .oa-contract-drawer .el-drawer__header,
  .oa-contract-drawer .el-drawer__body {
    padding: 16px;
  }

  .oa-contract-drawer .el-drawer__header .el-button {
    min-width: 64px;
    min-height: 44px;
  }

  .oa-contract-drawer .el-drawer__body {
    padding-bottom: max(16px, env(safe-area-inset-bottom));
  }
}

@media (prefers-reduced-motion: reduce) {
  .oa-contract-overlay,
  .oa-contract-overlay .oa-contract-drawer,
  .oa-contract-drawer * {
    transition-duration: 0s !important;
    animation-duration: 0s !important;
  }
}
</style>
