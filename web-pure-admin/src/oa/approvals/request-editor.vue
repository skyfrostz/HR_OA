<script setup>
import {
  computed,
  nextTick,
  onMounted,
  ref,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { ElButton as Button, ElDatePicker } from "element-plus";
import {
  ArrowRight,
  Calendar,
  Document,
  Wallet
} from "@element-plus/icons-vue";
import { useSession } from "../session";
import { useRemote } from "../use-remote";
import {
  approvalError,
  formFields,
  initialDraft,
  requestPayload,
  typeLabels
} from "./approval";
import { useIntent } from "./use-intent";
import LeaveFields from "./leave-fields.vue";
import PageState from "../remote-state.vue";

const props = defineProps({ previous: { type: Object, default: null } });
const emit = defineEmits(["locked", "saved", "back", "task-mode"]);
const session = useSession();
const selectedType = shallowRef(props.previous?.type || "");
const draft = ref(initialDraft(selectedType.value, props.previous));
const validationError = shallowRef("");
const choosingType = shallowRef(!props.previous);
const formError = useTemplateRef("formError");
const leavePreview = shallowRef(null);
const previewRevision = shallowRef(0);
const heading = useTemplateRef("heading");
const templates = useRemote(signal =>
  session.authorized("/request-templates", { signal })
);
const { result, loading, error: loadError, load } = templates;
const available = computed(() =>
  (result.value?.list || []).filter(item =>
    Object.hasOwn(formFields, item.type)
  )
);
const selected = computed(() =>
  available.value.find(item => item.type === selectedType.value)
);
const templateIcons = { LEAVE: Calendar, EXPENSE: Wallet, GENERAL: Document };
const fields = computed(() =>
  selectedType.value === "LEAVE" ? [] : formFields[selectedType.value] || []
);
const mutation = useIntent(approvalError);
const { busy, retryable, error } = mutation;
const locked = computed(() => busy.value || retryable.value);

onMounted(() => {
  heading.value.focus();
  void load();
});
watch(locked, value => emit("locked", value), { flush: "sync" });
watch(selectedType, value => emit("task-mode", Boolean(value)), {
  immediate: true
});
watch([validationError, error], ([validation, failure]) => {
  if (validation || failure) void nextTick(() => formError.value?.focus());
});

function selectType(type) {
  if (locked.value) return;
  choosingType.value = false;
  if (type === selectedType.value) return;
  selectedType.value = type;
  draft.value = initialDraft(type);
  leavePreview.value = null;
  previewRevision.value++;
  validationError.value = "";
  error.value = "";
  void nextTick(() => document.getElementById("pc-request-title")?.focus());
}

async function submit() {
  if (locked.value || !selected.value) return;
  validationError.value = "";
  let data;
  try {
    data = requestPayload(selected.value.type, draft.value, props.previous?.id);
  } catch (cause) {
    validationError.value = cause.message;
    return;
  }
  if (selectedType.value === "LEAVE" && !leavePreview.value?.canSubmit) {
    validationError.value = "请先预览请假天数并处理余额或时段问题。";
    return;
  }
  if (selectedType.value === "LEAVE")
    Object.assign(data.form, {
      leaveTypeVersion: leavePreview.value.calculation.leaveTypeVersion,
      calendarVersion: leavePreview.value.calculation.calendarVersion
    });
  const detail = await mutation.submit("/requests", data);
  finishSubmission(detail);
}

function acceptPreview(value) {
  leavePreview.value = value;
  if (value?.canSubmit) {
    validationError.value = "";
    error.value = "";
  }
}
function finishSubmission(detail) {
  if (detail) emit("saved", detail.id);
  else if (
    ["STALE_RECORD", "LEAVE_OVERLAP", "INSUFFICIENT_LEAVE"].includes(
      mutation.failure.value?.code
    )
  ) {
    leavePreview.value = null;
    previewRevision.value++;
    validationError.value = "配置、余额或可用时段已变化，请重新预览后再提交。";
  }
}
async function retry() {
  validationError.value = "";
  finishSubmission(await mutation.retry());
}
</script>

<template>
  <section class="pc-editor-task" aria-labelledby="pc-editor-title">
    <Button
      class="pc-back"
      size="small"
      plain
      :disabled="locked"
      @click="emit('back')"
      >{{ previous ? "返回原申请" : "返回列表" }}</Button
    >
    <div class="pc-heading pc-editor-intro">
      <h1 id="pc-editor-title" ref="heading" tabindex="-1">
        {{
          previous
            ? "重新提交申请"
            : selected
              ? typeLabels[selected.type]
              : "发起申请"
        }}
      </h1>
      <p v-if="!selected || previous" class="pc-muted">
        {{
          previous
            ? "修改后会生成一条新的申请记录。"
            : "选择申请类型后开始填写。"
        }}
      </p>
    </div>

    <PageState
      :loading="loading"
      :error="loadError"
      :empty="Boolean(result && !available.length)"
      empty-text="暂无可用的申请表单。"
      @retry="load"
    />
    <ul
      v-if="available.length && !previous && choosingType"
      class="pc-list pc-template-list"
      aria-label="申请类型"
    >
      <li v-for="template in available" :key="template.type">
        <button
          type="button"
          class="pc-card pc-record-button pc-template-button"
          :aria-pressed="selectedType === template.type"
          :disabled="locked"
          @click="selectType(template.type)"
        >
          <span
            class="pc-template-icon"
            :data-type="template.type"
            aria-hidden="true"
          >
            <component :is="templateIcons[template.type]" />
          </span>
          <span>
            <strong>{{ typeLabels[template.type] }}</strong>
            <span class="pc-template-description">{{
              template.description
            }}</span>
          </span>
          <ArrowRight class="pc-template-arrow" aria-hidden="true" />
        </button>
      </li>
    </ul>

    <div v-if="selected" class="pc-editor-card">
      <div class="pc-selected-type">
        <span>申请类型 · {{ typeLabels[selected.type] }}</span
        ><Button
          v-if="!previous"
          size="small"
          plain
          :disabled="locked"
          @click="choosingType = !choosingType"
          >{{ choosingType ? "保留当前类型" : "更换类型" }}</Button
        >
      </div>

      <form
        class="oa-form pc-form"
        :aria-describedby="
          validationError || error ? 'pc-request-error' : undefined
        "
        autocomplete="off"
        @submit.prevent="submit"
      >
        <label for="pc-request-title">
          标题
          <input
            id="pc-request-title"
            v-model="draft.title"
            required
            :disabled="locked"
          />
        </label>
        <LeaveFields
          v-if="selectedType === 'LEAVE'"
          :key="previewRevision"
          v-model="draft.form"
          :disabled="locked"
          @validated="acceptPreview"
        />
        <label
          v-for="field in fields"
          :key="field.name"
          :for="'pc-field-' + field.name"
        >
          {{ field.label }}
          <textarea
            v-if="field.input === 'textarea'"
            :id="'pc-field-' + field.name"
            v-model="draft.form[field.name]"
            required
            :disabled="locked"
          />
          <ElDatePicker
            v-else-if="field.input === 'date'"
            :id="'pc-field-' + field.name"
            v-model="draft.form[field.name]"
            type="date"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            placeholder="年-月-日"
            :disabled="locked"
          />
          <input
            v-else
            :id="'pc-field-' + field.name"
            v-model="draft.form[field.name]"
            :type="field.input === 'date' ? 'date' : 'text'"
            :inputmode="field.input === 'decimal' ? 'decimal' : undefined"
            required
            :disabled="locked"
          />
        </label>
        <details v-if="selected.route?.length" class="pc-route-preview">
          <summary>审批路径提示</summary>
          <p>
            {{
              selected.route.map(step => step.name).join(" → ")
            }}。金额规则在提交时确定，结果以申请保存的路径为准。
          </p>
        </details>
        <p
          v-if="validationError || error"
          id="pc-request-error"
          ref="formError"
          class="pc-message pc-message-error"
          tabindex="-1"
          role="alert"
        >
          {{ validationError || error }}
        </p>
        <div class="pc-editor-actions">
          <Button plain :disabled="locked" @click="emit('back')">取消</Button>
          <Button
            v-if="retryable"
            type="primary"
            :disabled="busy"
            :loading="busy"
            @click="retry"
            >重试原提交</Button
          >
          <Button
            v-else
            type="primary"
            native-type="submit"
            :disabled="
              busy || (selectedType === 'LEAVE' && !leavePreview?.canSubmit)
            "
            :loading="busy"
            >提交申请</Button
          >
        </div>
      </form>
    </div>
  </section>
</template>
