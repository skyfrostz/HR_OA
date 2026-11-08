<script setup>
import { computed, nextTick, shallowRef, watch } from "vue";
import { ElButton as Button, ElDatePicker } from "element-plus";
import { useSession } from "../session";
import { useRemote } from "../use-remote";
import PageState from "../remote-state.vue";

const props = defineProps({
  modelValue: { type: Object, required: true },
  disabled: Boolean
});
const emit = defineEmits(["update:modelValue", "validated"]);
const session = useSession();
const selectedLeaveType = computed({
  get: () => props.modelValue.leaveTypeId ?? "",
  set: value => update("leaveTypeId", value === "" ? null : Number(value))
});
const types = useRemote(signal =>
  session.authorized("/leave-types", { signal })
);
const {
  result: leaveTypes,
  loading: typesLoading,
  error: typesError,
  load: loadTypes
} = types;
const previewKey = shallowRef("");
const validation = shallowRef("");
const invalidField = shallowRef("");
const fieldIds = {
  leaveTypeId: "pc-leave-type",
  startDate: "pc-field-startDate",
  endDate: "pc-field-endDate",
  startHalf: "pc-start-half",
  endHalf: "pc-end-half"
};
const input = computed(() =>
  Object.fromEntries(
    ["leaveTypeId", "startDate", "endDate", "startHalf", "endHalf"].map(key => [
      key,
      props.modelValue[key]
    ])
  )
);
const inputKey = computed(() => JSON.stringify(input.value));
const previewRead = useRemote(signal =>
  session.authorized("/leave-preview", {
    method: "POST",
    data: input.value,
    signal
  })
);
const { result, loading, error, load } = previewRead;
const preview = computed(() =>
  previewKey.value === inputKey.value ? result.value : null
);
watch(
  inputKey,
  () => {
    previewRead.cancel();
    previewKey.value = "";
    result.value = null;
    error.value = "";
    validation.value = "";
    invalidField.value = "";
  },
  { flush: "sync" }
);
watch(preview, value => emit("validated", value), {
  immediate: true,
  flush: "sync"
});
watch(
  invalidField,
  () => {
    for (const name of ["startDate", "endDate"]) {
      const inputElement = document.getElementById(fieldIds[name]);
      if (!inputElement) continue;
      if (invalidField.value === name) {
        inputElement.setAttribute("aria-invalid", "true");
        inputElement.setAttribute("aria-describedby", "pc-leave-error");
      } else {
        inputElement.removeAttribute("aria-invalid");
        inputElement.removeAttribute("aria-describedby");
      }
    }
  },
  { flush: "post" }
);

void loadTypes();
function update(name, value) {
  emit("update:modelValue", { ...props.modelValue, [name]: value });
}
async function calculate() {
  if (props.disabled || loading.value) return;
  validation.value = "";
  const value = input.value;
  invalidField.value = "";
  const invalid =
    !Number.isSafeInteger(value.leaveTypeId) || value.leaveTypeId < 1
      ? ["leaveTypeId", "请选择请假类型。"]
      : !value.startDate
        ? ["startDate", "请选择开始日期。"]
        : !value.endDate
          ? ["endDate", "请选择结束日期。"]
          : value.startDate > value.endDate
            ? ["endDate", "结束日期不能早于开始日期。"]
            : value.startDate === value.endDate &&
                value.startHalf === "PM" &&
                value.endHalf === "AM"
              ? ["endHalf", "同一天的结束时段不能早于开始时段。"]
              : null;
  if (invalid) {
    invalidField.value = invalid[0];
    validation.value = invalid[1];
    await nextTick();
    const field = document.getElementById(fieldIds[invalidField.value]);
    field?.focus();
    field?.scrollIntoView?.({ block: "center" });
    return;
  }
  result.value = null;
  previewKey.value = inputKey.value;
  await load();
}
const issueLabels = {
  LEAVE_OVERLAP: "与已有请假时段重叠",
  INSUFFICIENT_LEAVE: "年度可用余额不足"
};
</script>

<template>
  <div class="pc-leave-fields">
    <PageState :loading="typesLoading" :error="typesError" @retry="loadTypes" />
    <p
      v-if="
        leaveTypes &&
        !typesLoading &&
        !leaveTypes.list.some(item => item.enabled)
      "
      class="pc-muted"
    >
      暂无可用请假类型，请联系管理员。
    </p>
    <label for="pc-leave-type"
      >请假类型
      <select
        id="pc-leave-type"
        v-model="selectedLeaveType"
        :aria-invalid="invalidField === 'leaveTypeId' || undefined"
        :aria-describedby="
          invalidField === 'leaveTypeId' ? 'pc-leave-error' : undefined
        "
        :disabled="disabled || typesLoading"
        required
      >
        <option value="" disabled>请选择请假类型</option>
        <option
          v-for="item in leaveTypes?.list.filter(item => item.enabled) || []"
          :key="item.id"
          :value="item.id"
        >
          {{ item.name
          }}{{ item.usesBalance ? " · 扣年度余额" : " · 不扣余额" }}
        </option>
      </select>
    </label>
    <div class="pc-leave-dates">
      <label for="pc-field-startDate"
        >开始日期<ElDatePicker
          id="pc-field-startDate"
          :model-value="modelValue.startDate"
          type="date"
          format="YYYY-MM-DD"
          value-format="YYYY-MM-DD"
          placeholder="年-月-日"
          :disabled="disabled"
          @update:model-value="update('startDate', $event || '')"
      /></label>
      <label for="pc-start-half"
        >开始时段<select
          id="pc-start-half"
          :aria-invalid="invalidField === 'startHalf' || undefined"
          :aria-describedby="
            invalidField === 'startHalf' ? 'pc-leave-error' : undefined
          "
          :value="modelValue.startHalf"
          :disabled="disabled"
          @change="update('startHalf', $event.target.value)"
        >
          <option value="AM">上午</option>
          <option value="PM">下午</option>
        </select></label
      >
      <label for="pc-field-endDate"
        >结束日期<ElDatePicker
          id="pc-field-endDate"
          :model-value="modelValue.endDate"
          type="date"
          format="YYYY-MM-DD"
          value-format="YYYY-MM-DD"
          placeholder="年-月-日"
          :disabled="disabled"
          @update:model-value="update('endDate', $event || '')"
      /></label>
      <label for="pc-end-half"
        >结束时段<select
          id="pc-end-half"
          :aria-invalid="invalidField === 'endHalf' || undefined"
          :aria-describedby="
            invalidField === 'endHalf' ? 'pc-leave-error' : undefined
          "
          :value="modelValue.endHalf"
          :disabled="disabled"
          @change="update('endHalf', $event.target.value)"
        >
          <option value="AM">上午</option>
          <option value="PM">下午</option>
        </select></label
      >
    </div>
    <label for="pc-field-reason"
      >请假事由<textarea
        id="pc-field-reason"
        :value="modelValue.reason"
        required
        :disabled="disabled"
        @input="update('reason', $event.target.value)"
      />
    </label>
    <p class="pc-muted">
      按管理员配置的工作日历计算；默认周一至周五，无内置法定节假日。
    </p>
    <Button
      :disabled="disabled || loading"
      :loading="loading"
      :aria-describedby="error ? 'pc-leave-error' : undefined"
      @click="calculate"
      >预览请假天数</Button
    >
    <p
      v-if="validation || error"
      id="pc-leave-error"
      class="pc-message pc-message-error"
      role="alert"
    >
      {{ validation || error }}
    </p>
    <section
      v-if="preview"
      class="pc-card pc-block"
      aria-label="请假计算预览"
      aria-live="polite"
    >
      <h3>
        {{ preview.calculation.leaveTypeName }} ·
        {{ preview.calculation.totalHalfDays / 2 }} 天
      </h3>
      <p class="pc-muted">
        {{
          preview.calculation.usesBalance
            ? "共用年度年假额度；提交后预占，最终批准后计入审批已用。"
            : "此类型不扣年度余额，仍校验时段重叠。"
        }}
      </p>
      <ul class="pc-history">
        <li v-for="year in preview.calculation.years" :key="year.year">
          {{ year.year }} 年：{{ year.halfDays / 2 }} 天
        </li>
      </ul>
      <p
        v-for="balance in preview.balances"
        :key="balance.year"
        class="pc-muted"
      >
        {{ balance.year }} 年可用 {{ balance.remainingHalfDays / 2 }} 天
      </p>
      <p
        v-if="!preview.canSubmit"
        class="pc-message pc-message-error"
        role="alert"
      >
        {{
          preview.issues.map(issue => issueLabels[issue] || issue).join("；")
        }}
      </p>
      <details>
        <summary>查看实际工作时段</summary>
        <ul class="pc-history">
          <li v-for="day in preview.calculation.days" :key="day.date">
            {{ day.date }} ·
            {{
              day.halves
                .map(half => (half === "AM" ? "上午" : "下午"))
                .join("、")
            }}
          </li>
        </ul>
      </details>
    </section>
  </div>
</template>

<style scoped>
.pc-leave-fields {
  display: grid;
  gap: 0.75rem;
  min-width: 0;
}

.pc-leave-fields > label {
  display: grid;
  gap: 0.375rem;
}

.pc-leave-dates {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(5rem, 0.55fr);
  gap: 0.75rem;
}

.pc-leave-dates label {
  display: grid;
  gap: 0.375rem;
  min-width: 0;
}

.pc-leave-fields summary {
  display: list-item;
  min-height: 2.75rem;
  padding-block: 0.5rem;
  cursor: pointer;
}
</style>
