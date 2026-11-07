<script setup lang="ts">
import { computed, shallowRef, useTemplateRef } from "vue";
import { useRouteWriteLock } from "./use-route-write-lock";
import { ElAlert, ElButton, ElTag } from "element-plus";
import { saveAttachment } from "./api";
import type { ImportPreview, ImportResult } from "./business";
import { useSession } from "./session";

const session = useSession();
const input = useTemplateRef("input");
const selectedFile = shallowRef<File | null>(null);
const preview = shallowRef<ImportPreview | null>(null);
const result = shallowRef<ImportResult | null>(null);
const pending = shallowRef<"template" | "preview" | "confirm" | "status" | "">("");
const uncertain = shallowRef(false);
const invalidated = shallowRef(false);
const error = shallowRef("");
const allowed = computed(() => session.account?.permissions.canImportEmployees === true);
const locked = computed(() => Boolean(pending.value) || uncertain.value);
const canConfirm = computed(() => Boolean(preview.value?.canConfirm && !invalidated.value && !result.value));
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

useRouteWriteLock(locked);

function pick(event: Event) {
  const element = event.target as HTMLInputElement;
  const file = element.files?.[0];
  selectedFile.value = null;
  error.value = "";
  if (!file) return;
  if (!/\.xlsx$/i.test(file.name) || !file.size || file.size > 2 * 1024 * 1024) {
    error.value = "请选择不超过 2 MiB 的非空 .xlsx 文件。";
    element.value = "";
    return;
  }
  selectedFile.value = file;
  preview.value = null;
  result.value = null;
  invalidated.value = false;
}

async function downloadTemplate() {
  if (locked.value || !allowed.value) return;
  pending.value = "template";
  error.value = "";
  try {
    saveAttachment(await session.authorized<Blob>("/employees/import-template.xlsx", { binary: true }), "员工导入模板.xlsx");
  } catch (cause) {
    error.value = cause.message;
  } finally {
    pending.value = "";
  }
}

async function inspect() {
  if (locked.value || !allowed.value || !selectedFile.value) return;
  pending.value = "preview";
  error.value = "";
  invalidated.value = false;
  try {
    preview.value = await session.authorized<ImportPreview>("/employee-imports/preview", {
      method: "POST", file: selectedFile.value
    });
    selectedFile.value = null;
    if (input.value) input.value.value = "";
  } catch (cause) {
    error.value = cause.status === 413 ? "Excel 文件超过 2 MiB。" : cause.message;
  } finally {
    pending.value = "";
  }
}

const previewPath = () => "/employee-imports/" + encodeURIComponent(preview.value.id);

async function checkStatus() {
  if (pending.value || !preview.value) return;
  pending.value = "status";
  try {
    preview.value = await session.authorized<ImportPreview>(previewPath());
    error.value = preview.value.status === "COMMITTED"
      ? "预览已提交。重试同一预览可读取原导入结果。"
      : "预览尚未标记完成。请重试同一预览以确认结果。";
  } catch (cause) {
    if (cause.status === 410) {
      uncertain.value = false;
      invalidated.value = true;
    }
    error.value = cause.status === 410 ? "预览已过期，请重新选择文件。" : cause.message;
  } finally {
    pending.value = "";
  }
}

async function confirm() {
  if (pending.value || !preview.value || !allowed.value || (!uncertain.value && !canConfirm.value)) return;
  pending.value = "confirm";
  error.value = "";
  try {
    result.value = await session.authorized<ImportResult>(previewPath() + "/confirm", { method: "POST", data: {} });
    uncertain.value = false;
  } catch (cause) {
    uncertain.value = !cause.status || cause.status >= 500 || cause.status === 429;
    if (uncertain.value) {
      error.value = "暂时无法确认导入结果。请重试同一预览。";
    } else {
      invalidated.value = true;
      error.value = cause.status === 410
        ? "预览已过期，请重新选择文件。"
        : cause.status === 409
          ? "预览已失效或记录有变动，请重新预览。"
          : cause.message;
    }
  } finally {
    pending.value = "";
  }
}
</script>

<template>
  <section class="oa-pure" aria-labelledby="employee-import-heading">
    <header class="oa-section-head oa-page-heading">
      <div><h1 id="employee-import-heading">Excel 导入</h1><p class="oa-muted">先预览并核对逐行结果，再确认新增员工。</p></div>
      <ElButton :loading="pending === 'template'" :disabled="locked" @click="downloadTemplate">下载导入模板</ElButton>
    </header>
    <section class="oa-surface">
      <h2>选择工作簿</h2>
      <p class="oa-muted">仅支持 .xlsx，最大 2 MiB，最多 1000 行。部门须已存在。</p>
      <form class="oa-form" autocomplete="off" @submit.prevent="inspect">
        <label for="oa-import-file">Excel 文件</label>
        <input id="oa-import-file" ref="input" type="file" accept=".xlsx" :disabled="locked" @change="pick">
        <div class="oa-actions">
          <ElButton type="primary" native-type="submit" :loading="pending === 'preview'" :disabled="locked || !selectedFile">预览导入</ElButton>
        </div>
      </form>
      <ElAlert v-if="error" class="oa-inline-alert" :title="error" type="warning" :closable="false" show-icon />
      <p v-if="result" class="oa-status" role="status">已导入 {{ result.importedCount }} 名员工。</p>
    </section>
    <section v-if="preview" class="oa-surface oa-import-preview" aria-labelledby="oa-import-preview">
      <div class="oa-section-head">
        <div>
          <h2 id="oa-import-preview">预览结果</h2>
          <p class="oa-muted">共 {{ preview.totalRows }} 行 · 通过 {{ preview.validRows }} 行 · 错误 {{ preview.errorRows }} 行</p>
        </div>
        <div class="oa-actions">
          <ElButton v-if="uncertain" :loading="pending === 'status'" :disabled="Boolean(pending)" @click="checkStatus">查询预览状态</ElButton>
          <ElButton
            v-if="!result"
            type="primary"
            :loading="pending === 'confirm'"
            :disabled="Boolean(pending) || (!canConfirm && !uncertain)"
            @click="confirm"
          >{{ uncertain ? '重试同一预览' : '确认导入' }}</ElButton>
        </div>
      </div>
      <pure-table :data="preview.rows" :columns="columns" row-key="row" max-height="560">
        <template #employeeNo="{ row }">{{ row.values.employeeNo }}</template>
        <template #name="{ row }">{{ row.values.name }}</template>
        <template #department="{ row }">{{ row.values.department }}</template>
        <template #birthDate="{ row }">{{ row.values.birthDate }}</template>
        <template #hireDate="{ row }">{{ row.values.hireDate }}</template>
        <template #salary="{ row }">{{ row.values.salary }}</template>
        <template #validation="{ row }">
          <ElTag v-if="!row.errors.length" type="success" effect="plain">通过</ElTag>
          <ul v-else class="oa-import-errors">
            <li v-for="(item, index) in row.errors" :key="index">{{ item.field }} · {{ item.message }}</li>
          </ul>
        </template>
      </pure-table>
    </section>
  </section>
</template>
