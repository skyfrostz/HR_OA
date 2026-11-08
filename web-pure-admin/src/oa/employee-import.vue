<script setup lang="ts">
import { computed, nextTick, shallowRef, useTemplateRef } from "vue";
import { useMediaQuery } from "@vueuse/core";
import { ElAlert, ElButton, ElTag } from "element-plus";
import { useRouteWriteLock } from "./use-route-write-lock";
import { useEmployeeImport } from "./use-employee-import";
import ImportHistory from "./import-history.vue";
const narrow = useMediaQuery("(max-width: 38rem)");
const input = useTemplateRef("input");
const previewHeading = useTemplateRef("previewHeading");
const resultHeading = useTemplateRef("resultHeading");
const errorsOnly = shallowRef(false);
const {
  selectedFile,
  preview,
  result,
  pending,
  uncertain,
  invalidated,
  error,
  revision,
  locked,
  canConfirm,
  replacementPending,
  retryReplacement,
  pick: selectFile,
  clear,
  inspect: readPreview,
  confirm,
  checkStatus,
  open: readHistory,
  downloadTemplate,
  downloadErrors
} = useEmployeeImport();
const stage = computed(() =>
  result.value ? 3 : canConfirm.value ? 2 : preview.value ? 1 : 0
);
const stages = ["选择文件", "校验预览", "确认导入", "导入结果"];
const rows = computed(() =>
  (preview.value?.rows || []).filter(
    row => !errorsOnly.value || row.errors.length
  )
);
const expiry = computed(() =>
  preview.value?.expiresAt
    ? new Date(preview.value.expiresAt).toLocaleString("zh-CN", {
        hour12: false
      })
    : "—"
);
useRouteWriteLock(locked);
async function pick(event: Event) {
  const element = event.target as HTMLInputElement;
  await selectFile(element.files?.[0]);
  element.value = "";
  if (!preview.value) errorsOnly.value = false;
}
async function inspect() {
  await readPreview();
  errorsOnly.value = Boolean(preview.value?.errorRows);
  if (preview.value) {
    if (input.value) input.value.value = "";
    await nextTick();
    previewHeading.value?.focus();
  }
}
async function submitImport() {
  await confirm();
  if (result.value) {
    await nextTick();
    resultHeading.value?.focus();
  }
}
function reset() {
  clear();
  if (input.value) input.value.value = "";
}
async function open(id: string) {
  errorsOnly.value = false;
  if (input.value) input.value.value = "";
  await readHistory(id);
  errorsOnly.value = Boolean(preview.value?.errorRows);
  await nextTick();
  (result.value ? resultHeading.value : previewHeading.value)?.focus();
}
const fieldLabels = {
  employeeNo: "工号",
  name: "姓名",
  department: "部门",
  birthDate: "出生日期",
  hireDate: "入职日期",
  salary: "约定工资"
};
const columns: TableColumnList = [
  { label: "行", prop: "row", width: 58 },
  { label: "工号", slot: "employeeNo", minWidth: 95 },
  { label: "姓名", slot: "name", minWidth: 100 },
  { label: "部门", slot: "department", minWidth: 105 },
  { label: "出生日期", slot: "birthDate", minWidth: 120 },
  { label: "入职日期", slot: "hireDate", minWidth: 120 },
  { label: "约定工资（元）", slot: "salary", minWidth: 130 },
  { label: "校验结果", slot: "validation", minWidth: 220 }
];
</script>

<template>
  <section class="oa-pure" aria-labelledby="employee-import-heading">
    <header class="oa-section-head oa-page-heading">
      <div>
        <h1 id="employee-import-heading">Excel 导入</h1>
        <p class="oa-muted">先预览并核对逐行结果，再确认新增员工。</p>
      </div>
      <ElButton
        :loading="pending === 'template'"
        :disabled="locked"
        @click="downloadTemplate"
        >下载导入模板</ElButton
      >
    </header>
    <ol class="oa-import-stages" aria-label="导入进度">
      <li
        v-for="(label, index) in stages"
        :key="label"
        :aria-current="index === stage ? 'step' : undefined"
        :class="{ completed: index < stage }"
      >
        <span>{{ index + 1 }}</span
        >{{ label }}
      </li>
    </ol>
    <ElAlert
      v-if="error"
      class="oa-inline-alert"
      :title="error"
      type="warning"
      :closable="false"
      show-icon
      role="alert"
    />
    <ElButton
      v-if="replacementPending"
      :loading="pending === 'discard'"
      :disabled="Boolean(pending)"
      @click="retryReplacement"
      >重试替换文件</ElButton
    >
    <input
      id="oa-import-file"
      ref="input"
      class="oa-file-input"
      type="file"
      tabindex="-1"
      aria-label="选择 Excel 文件"
      accept=".xlsx"
      :disabled="locked"
      @change="pick"
    />
    <section v-if="!preview && !result" class="oa-surface oa-import-upload">
      <form class="oa-form" autocomplete="off" @submit.prevent="inspect">
        <h2>选择工作簿</h2>
        <p class="oa-muted">
          仅支持 .xlsx，最大 2 MiB，最多 1000 行。部门须已存在。
        </p>
        <div class="oa-file-picker">
          <ElButton :disabled="locked" @click="input?.click()">{{
            selectedFile ? "替换文件" : "选择 Excel 文件"
          }}</ElButton
          ><span class="oa-import-filename">{{
            selectedFile?.name || "尚未选择文件"
          }}</span>
        </div>
        <div class="oa-actions">
          <ElButton
            type="primary"
            native-type="submit"
            :loading="pending === 'preview'"
            :disabled="locked || !selectedFile"
            >预览导入</ElButton
          >
          <ElButton v-if="selectedFile" :disabled="locked" @click="reset"
            >清除当前预览</ElButton
          >
        </div>
      </form>
    </section>
    <section
      v-if="result"
      class="oa-surface oa-import-result"
      aria-labelledby="oa-import-result"
    >
      <h2 id="oa-import-result" ref="resultHeading" tabindex="-1">导入完成</h2>
      <p class="oa-import-count">
        <strong>{{ result.importedCount }}</strong> 名员工已导入
      </p>
      <p class="oa-muted">
        源文件及逐行内容已从当前预览清除。可前往员工档案核对，或继续导入其他文件。
      </p>
      <div class="oa-actions">
        <ElButton type="primary" :disabled="locked" @click="reset"
          >继续导入</ElButton
        >
      </div>
    </section>
    <section
      v-if="preview && !result"
      class="oa-surface oa-import-preview"
      aria-labelledby="oa-import-preview"
    >
      <div class="oa-section-head">
        <div>
          <h2 id="oa-import-preview" ref="previewHeading" tabindex="-1">
            预览结果
          </h2>
          <p class="oa-muted">
            共 {{ preview.totalRows }} 行 · 通过 {{ preview.validRows }} 行 ·
            错误 {{ preview.errorRows }} 行
          </p>
        </div>
        <div class="oa-actions">
          <ElButton :disabled="locked" @click="input?.click()"
            >替换文件</ElButton
          >
          <ElButton :disabled="locked" @click="reset">清除当前预览</ElButton>
          <ElButton
            v-if="uncertain"
            :loading="pending === 'status'"
            :disabled="Boolean(pending)"
            @click="checkStatus"
            >查询预览状态</ElButton
          >
          <ElButton
            v-if="!result"
            type="primary"
            :loading="pending === 'confirm'"
            :disabled="
              Boolean(pending) ||
              replacementPending ||
              (!canConfirm && !uncertain)
            "
            @click="submitImport"
            >{{ uncertain ? "重试同一预览" : "确认导入" }}</ElButton
          >
        </div>
      </div>
      <ElAlert
        v-if="preview.errorRows && preview.status === 'PREVIEW' && !invalidated"
        title="发现错误，请修正文件后重新预览；本次不会跳过错误行导入。"
        type="warning"
        :closable="false"
        show-icon
      />
      <p v-if="preview.status === 'PREVIEW'" class="oa-muted">
        预览有效至 {{ expiry }}；离开页面后清除当前内容。
      </p>
      <ElAlert
        v-if="preview.status === 'EXPIRED'"
        class="oa-inline-alert"
        title="预览已过期，敏感内容已清理。请重新选择文件并预览。"
        type="warning"
        :closable="false"
        show-icon
      />
      <div
        v-if="preview.status === 'PREVIEW' && !invalidated"
        class="oa-import-controls"
      >
        <div class="oa-segmented" aria-label="导入预览筛选">
          <button
            type="button"
            :aria-pressed="!errorsOnly"
            :disabled="locked"
            @click="errorsOnly = false"
          >
            全部行</button
          ><button
            type="button"
            :aria-pressed="errorsOnly"
            :disabled="locked"
            @click="errorsOnly = true"
          >
            仅错误行 · {{ preview.errorRows }}
          </button>
        </div>
        <ElButton
          :disabled="locked"
          :loading="pending === 'errors'"
          @click="downloadErrors"
          >下载错误行</ElButton
        >
      </div>
      <p
        v-if="preview.status === 'PREVIEW' && errorsOnly && !rows.length"
        class="oa-status"
      >
        没有错误行。
      </p>
      <pure-table
        v-if="rows.length && !narrow"
        :data="rows"
        :columns="columns"
        row-key="row"
        max-height="560"
      >
        <template #employeeNo="{ row }">{{ row.values.employeeNo }}</template>
        <template #name="{ row }">{{ row.values.name }}</template>
        <template #department="{ row }">{{ row.values.department }}</template>
        <template #birthDate="{ row }">{{ row.values.birthDate }}</template>
        <template #hireDate="{ row }">{{ row.values.hireDate }}</template>
        <template #salary="{ row }">{{ row.values.salary }}</template>
        <template #validation="{ row }">
          <ElTag v-if="!row.errors.length" type="success" effect="plain"
            >通过</ElTag
          >
          <ul v-else class="oa-import-errors">
            <li v-for="(item, index) in row.errors" :key="index">
              {{ fieldLabels[item.field] || "内容" }} · {{ item.message }}
            </li>
          </ul>
        </template>
      </pure-table>
      <div
        v-if="rows.length && narrow"
        class="oa-import-cards"
        tabindex="0"
        aria-label="导入逐行预览"
      >
        <article v-for="row in rows" :key="row.row" class="oa-import-row">
          <header class="oa-section-head">
            <h3>第 {{ row.row }} 行 · {{ row.values.name || "姓名未填写" }}</h3>
            <ElTag
              :type="row.errors.length ? 'warning' : 'success'"
              effect="plain"
              >{{ row.errors.length ? "需修正" : "通过" }}</ElTag
            >
          </header>
          <dl class="oa-import-values">
            <div
              v-for="field in [
                { key: 'employeeNo', label: '工号' },
                { key: 'department', label: '部门' },
                { key: 'birthDate', label: '出生日期' },
                { key: 'hireDate', label: '入职日期' },
                { key: 'salary', label: '约定工资（元）' }
              ]"
              :key="field.key"
            >
              <dt>{{ field.label }}</dt>
              <dd>{{ row.values[field.key] || "—" }}</dd>
            </div>
          </dl>
          <ul v-if="row.errors.length" class="oa-import-errors">
            <li v-for="(item, index) in row.errors" :key="index">
              {{ fieldLabels[item.field] || "内容" }} · 原值：{{
                row.values[item.field] || "空"
              }}
              · {{ item.message }}
            </li>
          </ul>
        </article>
      </div>
    </section>
    <details class="oa-import-history-disclosure">
      <summary>历史预览与导入结果</summary>
      <ImportHistory :revision="revision" :disabled="locked" @open="open" />
    </details>
  </section>
</template>

<style scoped>
.oa-import-stages {
  display: flex;
  gap: 24px;
  padding: 0;
  margin: 0 0 16px;
  color: var(--oa-muted);
  list-style: none;
}

.oa-import-stages li {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 14px;
}

.oa-import-stages span {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  font-variant-numeric: tabular-nums;
  border: 1px solid var(--oa-line);
  border-radius: 50%;
}

.oa-import-stages [aria-current="step"] {
  font-weight: 600;
  color: var(--oa-accent-text);
}

.oa-import-stages [aria-current="step"] span {
  background: var(--oa-tint);
  border-color: currentcolor;
}

.oa-import-filename {
  min-width: 0;
  overflow-wrap: anywhere;
}

.oa-import-count {
  margin: 16px 0;
}

.oa-import-count strong {
  font-size: 24px;
  font-variant-numeric: tabular-nums;
  color: var(--oa-accent-text);
}

.oa-import-history-disclosure {
  margin-top: 24px;
  border-top: 1px solid var(--oa-line);
}

.oa-import-history-disclosure > summary {
  padding: 16px 0;
  font-weight: 600;
  cursor: pointer;
}

.oa-import-upload {
  max-width: 820px;
}

@media (width <= 38rem) {
  .oa-import-stages {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 16px;
  }
}
</style>
