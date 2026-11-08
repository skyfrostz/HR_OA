<script setup>
import {
  shallowRef,
  onMounted,
  onScopeDispose,
  useTemplateRef,
  watch
} from "vue";
import { ElAlert, ElButton } from "element-plus";
import { useSession } from "./session";

const props = defineProps({
  contractId: { type: Number, required: true },
  refresh: { type: Function, required: true }
});
const emit = defineEmits(["uploaded", "cancel", "state-change"]);
const session = useSession();
let disposed = false;
onScopeDispose(() => {
  disposed = true;
});
const heading = useTemplateRef("heading");
const fileInput = useTemplateRef("fileInput");
onMounted(() => heading.value.focus());
const selectedFile = shallowRef(null);
const classification = shallowRef("RESTRICTED");
const busy = shallowRef(false);
const requiresRefresh = shallowRef(false);
const error = shallowRef("");
watch(
  [busy, selectedFile, classification, requiresRefresh],
  () =>
    emit("state-change", {
      busy: busy.value,
      dirty:
        Boolean(selectedFile.value || requiresRefresh.value) ||
        classification.value !== "RESTRICTED"
    }),
  { immediate: true, flush: "sync" }
);

function selectFile(event) {
  error.value = "";
  const file = event.target.files[0];
  selectedFile.value = null;
  if (!file) return;
  if (!/\.(pdf|jpe?g|png)$/i.test(file.name)) {
    error.value = "请选择 PDF、JPEG 或 PNG 文件。";
  } else if (file.size > 10 * 1024 * 1024) {
    error.value = "文件超过 10 MiB 上传限制。";
  } else if (!file.size) {
    error.value = "请选择非空文件。";
  } else {
    selectedFile.value = file;
  }
  if (!selectedFile.value) event.target.value = "";
}

async function submit() {
  if (
    busy.value ||
    requiresRefresh.value ||
    !selectedFile.value ||
    !session.canWrite
  )
    return;
  busy.value = true;
  error.value = "";
  try {
    await session.authorized("/contracts/" + props.contractId + "/files", {
      method: "POST",
      file: selectedFile.value,
      fileFields: { classification: classification.value }
    });
    if (!disposed) emit("uploaded");
  } catch (cause) {
    requiresRefresh.value = !cause.status || cause.status >= 500;
    error.value = requiresRefresh.value
      ? "上传结果未知。请刷新合同并核对附件列表，再决定是否重传。"
      : cause.message;
  } finally {
    busy.value = false;
  }
}

async function refreshAttachments() {
  if (busy.value) return;
  busy.value = true;
  try {
    if (await props.refresh()) {
      requiresRefresh.value = false;
      selectedFile.value = null;
      error.value = "已刷新。请核对附件列表，如需重传请重新选择文件。";
    }
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="oa-inline-form" aria-labelledby="attachment-form-heading">
    <h4 id="attachment-form-heading" ref="heading" tabindex="-1">
      添加合同附件
    </h4>
    <p id="oa-attachment-hint" class="oa-muted">
      支持 PDF、JPEG、PNG，单个文件最多 10 MiB。
    </p>
    <ElAlert
      v-if="error"
      :title="error"
      type="warning"
      :closable="false"
      show-icon
    />
    <form class="oa-form" @submit.prevent="submit">
      <div v-if="!requiresRefresh" class="oa-file-selection">
        <ElButton :disabled="busy" @click="fileInput?.click()">{{
          selectedFile ? "更换文件" : "选择文件"
        }}</ElButton>
        <span role="status">{{ selectedFile?.name || "尚未选择文件" }}</span>
      </div>
      <input
        v-if="!requiresRefresh"
        id="oa-attachment-input"
        ref="fileInput"
        class="oa-file-input"
        tabindex="-1"
        aria-label="选择合同附件"
        type="file"
        accept=".pdf,.jpeg,.jpg,.png"
        aria-describedby="oa-attachment-hint"
        :disabled="busy"
        @change="selectFile"
      />
      <label v-if="!requiresRefresh" for="oa-upload-classification"
        >附件访问范围</label
      >
      <select
        v-if="!requiresRefresh"
        id="oa-upload-classification"
        v-model="classification"
        :disabled="busy"
      >
        <option value="RESTRICTED">敏感 · 管理员与合同所属员工</option>
        <option value="STANDARD">普通 · 按合同权限查看</option>
      </select>
      <p class="oa-muted">含工资或其他敏感信息时，请保留“敏感”。</p>
      <div class="oa-actions">
        <ElButton
          v-if="requiresRefresh"
          :loading="busy"
          @click="refreshAttachments"
          >刷新并核对附件</ElButton
        >
        <ElButton
          v-else
          type="primary"
          native-type="submit"
          :loading="busy"
          :disabled="!selectedFile"
          >上传附件</ElButton
        >
        <ElButton :disabled="busy || requiresRefresh" @click="emit('cancel')"
          >取消</ElButton
        >
      </div>
    </form>
  </section>
</template>

<style scoped>
.oa-file-selection {
  display: flex;
  gap: 12px;
  align-items: center;
  min-width: 0;
}

.oa-file-selection span {
  color: var(--oa-muted);
  overflow-wrap: anywhere;
}

.oa-file-selection :deep(.el-button) {
  flex-shrink: 0;
}

.oa-file-input {
  display: none;
}
</style>
