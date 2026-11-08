<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  reactive,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { ElAlert, ElButton, ElDrawer, ElMessageBox } from "element-plus";
import type { OaError } from "../api";
import {
  writeNeedsRefresh,
  type Department,
  type DepartmentTreeNode
} from "../business";
import { useSession } from "../session";
import { useRouteWriteLock } from "../use-route-write-lock";
import AuditHistory from "../audit-history.vue";
import {
  ancestorIds,
  buildOrganization,
  eligibleParents
} from "./organization-model";

const props = defineProps<{
  open: boolean;
  department: DepartmentTreeNode | null;
  departments: DepartmentTreeNode[];
  creating: boolean;
  parentId: number | null;
  reload: () => Promise<boolean>;
}>();
const emit = defineEmits<{
  close: [];
  closed: [];
  saved: [department: Department];
  editing: [value: boolean];
}>();
const session = useSession();
const canManage = computed(
  () => session.account?.permissions.canManageDepartments === true
);
const mode = shallowRef<"view" | "edit" | "new" | "history">(
  props.creating ? "new" : "view"
);
const current = shallowRef<Department | null>(props.department);
const model = computed(() => buildOrganization(props.departments));
const currentNode = computed(() =>
  props.departments.find(row => row.id === current.value?.id)
);
const parentOptions = computed(() =>
  eligibleParents(
    model.value,
    mode.value === "edit" ? current.value?.id : undefined
  )
);
const path = computed(() =>
  current.value
    ? ancestorIds(model.value, current.value.id)
        .map(id => model.value.byId.get(id)?.name)
        .join(" / ")
    : ""
);
const form = reactive({
  name: props.creating ? "" : props.department?.name || "",
  parentId: props.creating
    ? props.parentId
    : (props.department?.parentId ?? null)
});
let initial = { ...form };
let unknownWrite: {
  name: string;
  parentId: number | null;
  ids: Set<number>;
  creating: boolean;
} | null = null;
let disposed = false;
let confirmingClose = false;
const busy = shallowRef(false);
const needsRefresh = shallowRef(false);
const error = shallowRef("");
const notice = shallowRef("");
const fieldError = shallowRef(false);
const auditRevision = shallowRef(0);
const formHeading = useTemplateRef("formHeading");
const nameInput = useTemplateRef("nameInput");
const editButton = useTemplateRef("editButton");
const editing = computed(() => mode.value === "edit" || mode.value === "new");
const dirty = computed(
  () => form.name !== initial.name || form.parentId !== initial.parentId
);
useRouteWriteLock(
  computed(() => editing.value || busy.value || needsRefresh.value)
);
watch(editing, value => emit("editing", value), {
  immediate: true,
  flush: "sync"
});
watch(
  () => props.department,
  value => {
    if (value && !editing.value) current.value = value;
  }
);
onBeforeUnmount(() => {
  disposed = true;
});

function begin(kind: "edit" | "new") {
  if (!canManage.value || busy.value) return;
  form.name = kind === "new" ? "" : current.value?.name || "";
  form.parentId =
    kind === "new"
      ? (current.value?.id ?? null)
      : (current.value?.parentId ?? null);
  initial = { ...form };
  mode.value = kind;
  error.value = "";
  notice.value = "";
  needsRefresh.value = false;
  fieldError.value = false;
  void nextTick(() => formHeading.value?.focus());
}
async function cancel() {
  if (busy.value) return;
  if (!current.value) {
    emit("close");
    return;
  }
  mode.value = "view";
  needsRefresh.value = false;
  error.value = "";
  await nextTick();
  if (props.open) editButton.value?.$el?.focus({ preventScroll: true });
}
async function requestClose(done?: () => void) {
  if (busy.value || confirmingClose || disposed || !props.open) return;
  if (editing.value && (dirty.value || needsRefresh.value)) {
    confirmingClose = true;
    try {
      await ElMessageBox.confirm(
        "尚有未保存或未确认的部门修改。关闭后这些输入不会保留。",
        "关闭部门编辑",
        {
          confirmButtonText: "放弃并关闭",
          cancelButtonText: "继续编辑",
          type: "warning"
        }
      );
    } catch {
      return;
    } finally {
      confirmingClose = false;
    }
  }
  if (busy.value || disposed || !props.open) return;
  emit("close");
  done?.();
}
async function finishSaved(department: Department, refresh = true) {
  if (disposed) return;
  current.value = department;
  mode.value = "view";
  needsRefresh.value = false;
  unknownWrite = null;
  notice.value = "部门已保存。";
  error.value = "";
  auditRevision.value++;
  emit("saved", department);
  if (refresh && !(await props.reload()))
    notice.value = "部门已保存，结构图暂未刷新。请关闭详情后重试刷新。";
  await nextTick();
  const active = document.activeElement;
  const button = editButton.value?.$el;
  if (
    !disposed &&
    props.open &&
    !editing.value &&
    (active === document.body ||
      active === button ||
      active?.matches(".el-drawer"))
  )
    button?.focus({ preventScroll: true });
}
async function reconcile() {
  if (busy.value || !needsRefresh.value) return;
  busy.value = true;
  try {
    if (!(await props.reload()) || disposed) return;
    await nextTick();
    if (unknownWrite?.creating) {
      const matches = props.departments.filter(
        row =>
          !unknownWrite!.ids.has(row.id) &&
          row.name === unknownWrite!.name &&
          row.parentId === unknownWrite!.parentId
      );
      if (matches.length === 1) {
        await finishSaved(matches[0], false);
        return;
      }
      error.value =
        "已重新读取结构，仍无法确认这次新增的结果。输入已保留，请核对部门列表后再决定；不会自动重复新增。";
      return;
    }
    const latest = props.departments.find(row => row.id === current.value?.id);
    if (!latest) {
      error.value = "当前部门已不可读取，请保留输入并联系管理员核对。";
      return;
    }
    current.value = latest;
    needsRefresh.value = false;
    unknownWrite = null;
    error.value = "已读取当前版本，输入已保留。请核对后再保存。";
  } finally {
    busy.value = false;
  }
}
async function save() {
  if (!editing.value || busy.value || needsRefresh.value || !canManage.value)
    return;
  const name = form.name.trim();
  fieldError.value = !name || [...name].length > 128;
  if (fieldError.value) {
    error.value = "请输入 1 至 128 字的部门名称。";
    await nextTick();
    nameInput.value?.focus();
    return;
  }
  if (
    form.parentId !== null &&
    !parentOptions.value.some(row => row.id === form.parentId)
  ) {
    error.value = "请选择当前可用的上级部门，不能选择自身或下级部门。";
    return;
  }
  busy.value = true;
  error.value = "";
  const creating = mode.value === "new";
  const attempt = {
    name,
    parentId: form.parentId,
    ids: new Set(props.departments.map(row => row.id)),
    creating
  };
  try {
    const department = await session.authorized<Department>(
      creating ? "/departments" : "/departments/" + current.value!.id,
      {
        method: creating ? "POST" : "PUT",
        data: {
          name,
          parentId: form.parentId,
          ...(creating ? {} : { version: current.value!.version })
        }
      }
    );
    await finishSaved(department);
  } catch (cause) {
    if (disposed) return;
    const failure = cause as OaError;
    needsRefresh.value = writeNeedsRefresh(failure);
    unknownWrite = needsRefresh.value ? attempt : null;
    error.value =
      failure.errorCode === "STALE_RECORD"
        ? "部门已被其他人修改。输入已保留，请重新读取当前版本后核对。"
        : needsRefresh.value
          ? "无法确认保存结果，输入已保留。请先重新读取部门，勿重复提交。"
          : failure.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ElDrawer
    :model-value="open"
    class="oa-department-drawer"
    title="部门详情"
    size="min(42rem, 100vw)"
    append-to-body
    :close-on-click-modal="false"
    :close-on-press-escape="!busy"
    :before-close="requestClose"
    @close="emit('close')"
    @closed="emit('closed')"
  >
    <div class="oa-pure oa-department-detail">
      <form
        v-if="editing"
        class="oa-form"
        autocomplete="off"
        @submit.prevent="save"
      >
        <h2 ref="formHeading" tabindex="-1">
          {{ mode === "new" ? "新增部门" : "编辑部门" }}
        </h2>
        <p v-if="mode === 'new' && form.parentId" class="oa-muted">
          新增下级部门
        </p>
        <div class="oa-fields">
          <label for="department-name"
            >部门名称
            <input
              id="department-name"
              ref="nameInput"
              v-model="form.name"
              required
              maxlength="128"
              :disabled="busy"
              :aria-invalid="fieldError || undefined"
              :aria-describedby="
                fieldError ? 'department-edit-error' : undefined
              "
            />
          </label>
          <label for="department-parent"
            >上级部门
            <select
              id="department-parent"
              v-model="form.parentId"
              :disabled="busy"
            >
              <option :value="null">无上级部门</option>
              <option
                v-for="parent in parentOptions"
                :key="parent.id"
                :value="parent.id"
              >
                {{ parent.name }}
              </option>
            </select>
          </label>
        </div>
        <ElAlert
          v-if="error"
          id="department-edit-error"
          :title="error"
          type="warning"
          :closable="false"
          role="alert"
        />
        <div class="oa-actions">
          <ElButton v-if="needsRefresh" :loading="busy" @click="reconcile"
            >重新读取部门</ElButton
          >
          <ElButton
            v-else
            native-type="submit"
            type="primary"
            :loading="busy"
            :disabled="!canManage"
            >保存部门</ElButton
          >
          <ElButton :disabled="busy" @click="cancel">取消</ElButton>
        </div>
      </form>
      <template v-else-if="current">
        <p v-if="notice" class="oa-status" role="status">{{ notice }}</p>
        <p v-if="path" class="oa-muted oa-department-path">{{ path }}</p>
        <h2 class="oa-department-name">{{ current.name }}</h2>
        <div class="oa-actions oa-department-tabs" aria-label="部门详情内容">
          <ElButton :aria-pressed="mode === 'view'" @click="mode = 'view'"
            >部门信息</ElButton
          >
          <ElButton :aria-pressed="mode === 'history'" @click="mode = 'history'"
            >变更历史</ElButton
          >
        </div>
        <template v-if="mode === 'view'">
          <dl class="oa-description">
            <div>
              <dt>上级部门</dt>
              <dd>
                {{
                  current.parentId
                    ? model.byId.get(current.parentId)?.name || "当前不可读取"
                    : "无上级部门"
                }}
              </dd>
            </div>
            <div>
              <dt>直属员工</dt>
              <dd>
                {{ currentNode?.employeeCount ?? "—"
                }}<span v-if="currentNode?.employeeCount !== undefined">
                  人</span
                >
              </dd>
            </div>
            <div>
              <dt>直属下级</dt>
              <dd>
                {{ model.children.get(current.id)?.length ?? "—" }} 个部门
              </dd>
            </div>
          </dl>
          <p class="oa-muted">员工人数仅包含当前账号有权查看的直属员工。</p>
          <div v-if="canManage" class="oa-actions">
            <ElButton ref="editButton" type="primary" @click="begin('edit')"
              >编辑部门</ElButton
            >
            <ElButton @click="begin('new')">新增下级</ElButton>
          </div>
        </template>
        <AuditHistory
          v-else
          :department-id="current.id"
          :revision="auditRevision"
          title="部门变更历史"
        />
      </template>
    </div>
  </ElDrawer>
</template>

<style scoped>
.oa-department-detail {
  min-width: 0;
}

.oa-department-name,
.oa-department-path {
  overflow-wrap: anywhere;
}

.oa-department-tabs {
  margin: 16px 0 24px;
}

.oa-fields {
  grid-template-columns: minmax(0, 1fr);
}

.oa-form .oa-actions {
  margin-top: 16px;
}

:global(.oa-department-drawer .el-drawer__header) {
  padding-bottom: 16px;
  margin-bottom: 0;
  color: var(--oa-text);
  border-bottom: 1px solid var(--oa-line);
}

:global(.oa-department-drawer .el-drawer__body) {
  overflow: auto;
  overscroll-behavior: contain;
}

:global(.oa-department-drawer .el-drawer__close-btn) {
  min-width: 44px;
  min-height: 44px;
}

@media (prefers-reduced-motion: reduce) {
  :global(.el-overlay:has(.oa-department-drawer)),
  :global(.oa-department-drawer) {
    transition: none !important;
  }
}
</style>
