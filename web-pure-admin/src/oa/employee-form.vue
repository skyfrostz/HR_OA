<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef, useTemplateRef } from "vue";
import { ElAlert, ElButton, ElDatePicker } from "element-plus";
import {
  findAcrossPages,
  formatYuan,
  listDepartments,
  writeNeedsRefresh,
  type Department,
  type Employee,
  type Page
} from "./business";
import { parseSalary } from "./format";
import { useSession } from "./session";
import { useRemote } from "./use-remote";

const props = defineProps<{ employee?: Employee }>();
const emit = defineEmits<{ created: [number]; updated: []; cancel: [] }>();
const session = useSession();
const heading = useTemplateRef("heading");
const busy = shallowRef(false);
const needsRefresh = shallowRef(false);
const error = shallowRef("");
const current = shallowRef(props.employee);
const form = reactive({
  employeeNo: "",
  name: "",
  department: "",
  departmentId: null as number | null,
  birthDate: "",
  hireDate: "",
  salary: ""
});
const canUseDepartmentId = computed(() =>
  Boolean(session.account?.permissions.canManageDepartments)
);
const departments = useRemote((signal: AbortSignal) =>
  listDepartments(session.authorized, signal)
);
const departmentOptions = computed(
  () => (departments.result.value || []) as Department[]
);

function fill(employee?: Employee) {
  form.employeeNo = employee?.employeeNo || "";
  form.name = employee?.name || "";
  form.department = employee?.department || "";
  form.departmentId = employee?.departmentId || null;
  form.birthDate = employee?.birthDate || "";
  form.hireDate = employee?.hireDate || "";
  form.salary = formatYuan(employee?.salaryCents);
}

fill(current.value);
onMounted(() => {
  heading.value?.focus();
  if (canUseDepartmentId.value) void departments.load();
});

async function refresh() {
  if (busy.value) return;
  busy.value = true;
  try {
    if (current.value) {
      current.value = await session.authorized<Employee>(
        "/employees/" + current.value.id
      );
      fill(current.value);
      if (canUseDepartmentId.value) await departments.load();
      error.value = "已读取当前档案，请核对字段后再编辑。";
    } else {
      const existing = await findAcrossPages<Employee>(
        page =>
          session.authorized<Page<Employee>>(
            "/employees?page=" + page + "&pageSize=100"
          ),
        employee => employee.employeeNo === form.employeeNo.trim()
      );
      if (existing) {
        emit("created", existing.id);
        return;
      }
      error.value = "已核对员工名册，当前没有该工号。请核对后再提交。";
    }
    needsRefresh.value = false;
  } catch (cause) {
    error.value = cause.message;
  } finally {
    busy.value = false;
  }
}

async function submit() {
  if (busy.value || needsRefresh.value || !session.canWrite) return;
  const { birthDate, hireDate, departmentId } = form;
  const employeeNo = form.employeeNo.trim();
  const name = form.name.trim();
  const department = form.department.trim();
  const salaryCents = parseSalary(form.salary);
  const departmentInput = canUseDepartmentId.value
    ? { departmentId }
    : { department };
  if (
    !employeeNo ||
    !name ||
    !birthDate ||
    !hireDate ||
    salaryCents === null ||
    (canUseDepartmentId.value ? !departmentId : !department) ||
    (current.value && !current.value.version)
  ) {
    error.value = "请填写全部字段。工资须为非负金额，最多保留两位小数。";
    return;
  }

  busy.value = true;
  error.value = "";
  const data = {
    employeeNo,
    name,
    ...departmentInput,
    birthDate,
    hireDate,
    salaryCents
  };
  try {
    if (current.value) {
      await session.authorized<Employee>("/employees/" + current.value.id, {
        method: "PUT",
        data: { ...data, version: current.value.version }
      });
      emit("updated");
    } else {
      const created = await session.authorized<Employee>("/employees", {
        method: "POST",
        data
      });
      emit("created", created.id);
    }
  } catch (cause) {
    needsRefresh.value = writeNeedsRefresh(cause);
    error.value = needsRefresh.value
      ? "当前员工记录可能已变化。请重新读取后再提交。"
      : cause.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="oa-inline-form" aria-labelledby="new-employee-heading">
    <h2 id="new-employee-heading" ref="heading" tabindex="-1">
      {{ current ? "编辑员工" : "新增员工" }}
    </h2>
    <ElAlert
      v-if="error"
      :title="error"
      type="warning"
      :closable="false"
      show-icon
    />
    <ElAlert
      v-if="departments.error.value"
      :title="departments.error.value"
      type="error"
      :closable="false"
    />
    <form class="oa-form" autocomplete="off" @submit.prevent="submit">
      <div class="oa-fields">
        <label for="employee-no"
          >工号<input
            id="employee-no"
            v-model="form.employeeNo"
            required
            :disabled="busy || needsRefresh"
        /></label>
        <label for="employee-name"
          >姓名<input
            id="employee-name"
            v-model="form.name"
            required
            :disabled="busy || needsRefresh"
        /></label>
        <label for="employee-department"
          >部门
          <select
            v-if="canUseDepartmentId"
            id="employee-department"
            v-model="form.departmentId"
            required
            :disabled="busy || needsRefresh || departments.loading.value"
          >
            <option :value="null" disabled>请选择部门</option>
            <option
              v-for="item in departmentOptions"
              :key="item.id"
              :value="item.id"
            >
              {{ item.name }}
            </option>
          </select>
          <input
            v-else
            id="employee-department"
            v-model="form.department"
            required
            :disabled="busy || needsRefresh"
          />
        </label>
        <label for="employee-birth"
          >出生日期<ElDatePicker
            id="employee-birth"
            v-model="form.birthDate"
            type="date"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            placeholder="选择日期"
            :clearable="false"
            :disabled="busy || needsRefresh"
        /></label>
        <label for="employee-hire"
          >入职日期<ElDatePicker
            id="employee-hire"
            v-model="form.hireDate"
            type="date"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            placeholder="选择日期"
            :clearable="false"
            :disabled="busy || needsRefresh"
        /></label>
        <label for="employee-salary"
          >约定工资（元）<input
            id="employee-salary"
            v-model="form.salary"
            inputmode="decimal"
            required
            :disabled="busy || needsRefresh"
        /></label>
      </div>
      <div class="oa-actions">
        <ElButton v-if="needsRefresh" :loading="busy" @click="refresh"
          >重新读取员工</ElButton
        >
        <template v-else>
          <ElButton
            type="primary"
            native-type="submit"
            :loading="busy"
            :disabled="
              departments.loading.value || Boolean(departments.error.value)
            "
            >保存员工</ElButton
          >
          <ElButton :disabled="busy" @click="emit('cancel')">取消</ElButton>
          <ElButton v-if="departments.error.value" @click="departments.load"
            >重新加载部门</ElButton
          >
        </template>
      </div>
    </form>
  </section>
</template>

<style scoped>
:deep(.el-date-editor.el-input) {
  width: 100%;
}

.oa-fields {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 16px;
}

.oa-field-wide {
  grid-column: 1 / -1;
}

.oa-inline-form {
  padding: 16px;
  margin-top: 0;
  border-radius: 8px;
}

@media (width <= 38rem) {
  .oa-fields {
    grid-template-columns: 1fr;
  }

  .oa-inline-form {
    padding: 12px;
  }
}
</style>
