<script setup lang="ts">
import { computed, shallowRef, useTemplateRef } from "vue";
import { useMediaQuery } from "@vueuse/core";
import { ElAlert, ElButton, ElTag } from "element-plus";
import { useRouteWriteLock } from "./use-route-write-lock";
import { useEmployeeImport } from "./use-employee-import";
import ImportHistory from "./import-history.vue";
const narrow = useMediaQuery("(max-width: 38rem)");
const input = useTemplateRef("input");
const errorsOnly = shallowRef(false);
const { selectedFile, preview, result, pending, uncertain, invalidated, error, revision, locked, canConfirm, pick: selectFile, clear, inspect: readPreview, confirm, checkStatus, open: readHistory, downloadTemplate, downloadErrors } = useEmployeeImport();
const rows = computed(() => (preview.value?.rows || []).filter(row => !errorsOnly.value || row.errors.length));
const expiry = computed(() => preview.value?.expiresAt ? new Date(preview.value.expiresAt).toLocaleString("zh-CN", { hour12: false }) : "—");
useRouteWriteLock(locked);
function pick(event: Event) { selectFile((event.target as HTMLInputElement).files?.[0]); if (!selectedFile.value) (event.target as HTMLInputElement).value = ""; errorsOnly.value = false; }
async function inspect() { await readPreview(); if (input.value) input.value.value = ""; }
function reset() { clear(); if (input.value) input.value.value = ""; }
async function open(id: string) { errorsOnly.value = false; if (input.value) input.value.value = ""; await readHistory(id); }
const fieldLabels = { employeeNo: "工号", name: "姓名", department: "部门", birthDate: "出生日期", hireDate: "入职日期", salary: "约定工资" };
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
      <div><h1 id="employee-import-heading">Excel 导入</h1><p class="oa-muted">先预览并核对逐行结果，再确认新增员工。</p></div>
      <ElButton :loading="pending === 'template'" :disabled="locked" @click="downloadTemplate">下载导入模板</ElButton>
    </header>
    <section class="oa-surface">
      <h2>选择工作簿</h2>
      <p class="oa-muted">仅支持 .xlsx，最大 2 MiB，最多 1000 行。部门须已存在。</p>
      <form class="oa-form" autocomplete="off" @submit.prevent="inspect">
        <label for="oa-import-file">Excel 文件</label>
        <input id="oa-import-file" ref="input" class="oa-file-input" type="file" tabindex="-1" accept=".xlsx" :disabled="locked" @change="pick">
        <div class="oa-file-picker"><ElButton :disabled="locked" @click="input?.click()">选择 Excel 文件</ElButton><span class="oa-muted">{{ selectedFile?.name || (preview ? '可选择其他文件重新预览' : '尚未选择文件') }}</span></div>
        <div class="oa-actions">
          <ElButton type="primary" native-type="submit" :loading="pending === 'preview'" :disabled="locked || !selectedFile">预览导入</ElButton>
          <ElButton v-if="selectedFile || preview || result" :disabled="locked" @click="reset">清除当前预览</ElButton>
        </div>
      </form>
      <ElAlert v-if="error" class="oa-inline-alert" :title="error" type="warning" :closable="false" show-icon />
      <ElAlert v-if="result" class="oa-inline-alert oa-import-success" :title="`已导入 ${result.importedCount} 名员工，源文件及逐行内容已从当前预览清除。`" type="success" :closable="false" show-icon />
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
      <p v-if="preview.status === 'PREVIEW'" class="oa-muted">预览有效至 {{ expiry }}；离开页面后清除当前内容。</p>
      <ElAlert v-if="preview.status === 'EXPIRED'" class="oa-inline-alert" title="预览已过期，敏感内容已清理。请重新选择文件并预览。" type="warning" :closable="false" show-icon />
      <div v-if="preview.status === 'PREVIEW' && !invalidated" class="oa-import-controls">
        <div class="oa-segmented" aria-label="导入预览筛选"><button type="button" :aria-pressed="!errorsOnly" :disabled="locked" @click="errorsOnly = false">全部行</button><button type="button" :aria-pressed="errorsOnly" :disabled="locked" @click="errorsOnly = true">仅错误行 · {{ preview.errorRows }}</button></div>
        <ElButton :disabled="locked" :loading="pending === 'errors'" @click="downloadErrors">下载错误行</ElButton>
      </div>
      <p v-if="preview.status === 'PREVIEW' && errorsOnly && !rows.length" class="oa-status">没有错误行。</p>
      <pure-table v-if="rows.length && !narrow" :data="rows" :columns="columns" row-key="row" max-height="560">
        <template #employeeNo="{ row }">{{ row.values.employeeNo }}</template>
        <template #name="{ row }">{{ row.values.name }}</template>
        <template #department="{ row }">{{ row.values.department }}</template>
        <template #birthDate="{ row }">{{ row.values.birthDate }}</template>
        <template #hireDate="{ row }">{{ row.values.hireDate }}</template>
        <template #salary="{ row }">{{ row.values.salary }}</template>
        <template #validation="{ row }">
          <ElTag v-if="!row.errors.length" type="success" effect="plain">通过</ElTag>
          <ul v-else class="oa-import-errors">
            <li v-for="(item, index) in row.errors" :key="index">{{ fieldLabels[item.field] || '内容' }} · {{ item.message }}</li>
          </ul>
        </template>
      </pure-table>
      <div v-if="rows.length && narrow" class="oa-import-cards" tabindex="0" aria-label="导入逐行预览">
        <article v-for="row in rows" :key="row.row" class="oa-import-row">
          <header class="oa-section-head"><h3>第 {{ row.row }} 行 · {{ row.values.name || '姓名未填写' }}</h3><ElTag :type="row.errors.length ? 'warning' : 'success'" effect="plain">{{ row.errors.length ? '需修正' : '通过' }}</ElTag></header>
          <dl class="oa-import-values"><div v-for="field in [{key:'employeeNo',label:'工号'},{key:'department',label:'部门'},{key:'birthDate',label:'出生日期'},{key:'hireDate',label:'入职日期'},{key:'salary',label:'约定工资（元）'}]" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ row.values[field.key] || '—' }}</dd></div></dl>
          <ul v-if="row.errors.length" class="oa-import-errors"><li v-for="(item, index) in row.errors" :key="index">{{ item.message }}</li></ul>
        </article>
      </div>
    </section>
    <ImportHistory :revision="revision" :disabled="locked" @open="open" />
  </section>
</template>
