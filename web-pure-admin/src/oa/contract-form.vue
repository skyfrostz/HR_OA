<script setup>
import {
  computed,
  onMounted,
  onScopeDispose,
  reactive,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { ElAlert, ElButton, ElDatePicker } from "element-plus";
import { useVersionedWrite } from "./use-versioned-write";

const props = defineProps({
  path: { type: String, required: true },
  kind: { type: String, required: true },
  contract: { type: Object, default: null },
  refresh: { type: Function, required: true }
});
const emit = defineEmits(["saved", "cancel", "reconciled", "state-change"]);
const heading = useTemplateRef("heading");
let disposed = false;
onScopeDispose(() => {
  disposed = true;
});
onMounted(() => heading.value?.focus());
const labels = {
  create: "新建合同",
  renew: "续约合同",
  edit: "编辑草稿",
  sign: "登记签署",
  terminate: "终止合同"
};
const changesState = computed(() => ["sign", "terminate"].includes(props.kind));
const initial = props.kind === "edit" ? props.contract : null;
const form = reactive({
  contractNo: initial?.contractNo || "",
  startDate: initial?.startDate || "",
  endDate: initial?.endDate || "",
  effectiveDate: "",
  reason: ""
});
const validation = shallowRef("");
const write = useVersionedWrite();
let creationAttempt;
const { busy, blocked, error } = write;
const baseline = JSON.stringify(form);
watch(
  () => [busy.value, blocked.value, JSON.stringify(form)],
  () =>
    emit("state-change", {
      busy: busy.value,
      dirty: Boolean(blocked.value) || JSON.stringify(form) !== baseline
    }),
  { immediate: true, flush: "sync" }
);
const today = new Intl.DateTimeFormat("sv-SE", {
  timeZone: "Asia/Shanghai",
  year: "numeric",
  month: "2-digit",
  day: "2-digit"
}).format(new Date());

async function submit() {
  if (busy.value || blocked.value) return;
  validation.value = "";
  const reason = form.reason.trim();
  if (!reason || [...reason].length > 500) {
    validation.value = "请填写 1–500 字的操作原因。";
    return;
  }
  let path = props.path;
  let method = "POST";
  let data;
  if (changesState.value) {
    if (!form.effectiveDate || form.effectiveDate > today) {
      validation.value = "日期不得晚于今天。";
      return;
    }
    if (
      props.kind === "terminate" &&
      (form.effectiveDate < props.contract.startDate ||
        (props.contract.signedDate &&
          form.effectiveDate < props.contract.signedDate))
    ) {
      validation.value = "终止日期不得早于合同开始日期或已登记的签署日期。";
      return;
    }
    path = "/contracts/" + props.contract.id + "/actions";
    data = {
      action: props.kind === "sign" ? "SIGN" : "TERMINATE",
      version: props.contract.version,
      reason,
      effectiveDate: form.effectiveDate
    };
  } else {
    const contractNo = form.contractNo.trim();
    if (
      !contractNo ||
      !form.startDate ||
      !form.endDate ||
      form.startDate > form.endDate
    ) {
      validation.value = "请填写合同编号和有效日期，结束日期不能早于开始日期。";
      return;
    }
    data = {
      contractNo,
      startDate: form.startDate,
      endDate: form.endDate,
      reason
    };
    if (props.kind === "edit") {
      path = "/contracts/" + props.contract.id;
      method = "PUT";
      data.version = props.contract.version;
    } else {
      data.status = "DRAFT";
      if (props.kind === "renew") data.previousContractId = props.contract.id;
      creationAttempt = {
        contractNo,
        startDate: form.startDate,
        endDate: form.endDate
      };
    }
  }
  const result = await write.send(path, method, data);
  if (result && !disposed)
    emit("saved", result, labels[props.kind] + "已保存。");
}

async function reload() {
  validation.value = "";
  const creation = ["create", "renew"].includes(props.kind);
  const unknownCreation = creation && blocked.value === "unknown";
  let recovered;
  try {
    await write.reconcile(
      async () => {
        recovered = await props.refresh(
          unknownCreation
            ? creationAttempt?.contractNo || form.contractNo.trim()
            : "",
          {
            kind: props.kind,
            blocked: blocked.value,
            contract: props.contract,
            expected: creationAttempt
          }
        );
        if (unknownCreation && !recovered?.id) {
          validation.value =
            "尚未找到可确认的合同，输入已保留。请稍后重新读取，系统不会重复提交。";
          return false;
        }
        if (!recovered) validation.value = "未能读取合同，输入已保留。";
        return Boolean(recovered);
      },
      previous => {
        if (disposed) return;
        if (unknownCreation)
          emit("saved", recovered, "已找到匹配合同，请核对详情。");
        else emit("reconciled", previous);
      }
    );
  } catch (cause) {
    validation.value =
      cause.message || "核对失败，输入已保留，请稍后重新读取。";
  }
}
</script>

<template>
  <section class="oa-inline-form" aria-labelledby="contract-form-heading">
    <h3 id="contract-form-heading" ref="heading" tabindex="-1">
      {{ labels[kind] }}
    </h3>
    <p v-if="contract" class="oa-muted">
      {{ kind === "renew" ? "续约来源：" : "合同编号："
      }}{{ contract.contractNo }}
    </p>
    <p v-if="kind === 'sign'" class="oa-muted">登记已线下签署的合同及日期。</p>
    <p v-if="kind === 'renew'" class="oa-muted">
      续约会创建关联草稿，原合同与附件将保留。
    </p>
    <ElAlert
      v-if="validation || error"
      :title="validation || error"
      type="error"
      :closable="false"
      show-icon
    />
    <form class="oa-form" autocomplete="off" @submit.prevent="submit">
      <div class="oa-fields">
        <template v-if="!changesState">
          <label for="contract-number"
            >合同编号<input
              id="contract-number"
              v-model="form.contractNo"
              required
              :disabled="busy || Boolean(blocked)"
          /></label>
          <label for="contract-start"
            >开始日期<ElDatePicker
              id="contract-start"
              v-model="form.startDate"
              type="date"
              format="YYYY-MM-DD"
              value-format="YYYY-MM-DD"
              placeholder="选择日期"
              :clearable="false"
              :disabled="busy || Boolean(blocked)"
          /></label>
          <label for="contract-end"
            >结束日期<ElDatePicker
              id="contract-end"
              v-model="form.endDate"
              type="date"
              format="YYYY-MM-DD"
              value-format="YYYY-MM-DD"
              placeholder="选择日期"
              :clearable="false"
              :disabled="busy || Boolean(blocked)"
          /></label>
        </template>
        <label v-else for="contract-effective"
          >{{ kind === "sign" ? "签署日期" : "终止日期"
          }}<ElDatePicker
            id="contract-effective"
            v-model="form.effectiveDate"
            type="date"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            placeholder="选择日期"
            :clearable="false"
            :disabled="busy || Boolean(blocked)"
        /></label>
        <label class="oa-field-wide"
          >操作原因<textarea
            v-model="form.reason"
            maxlength="500"
            required
            placeholder="请勿填写密码、证件号码等敏感信息"
            :disabled="busy || Boolean(blocked)"
          />
        </label>
      </div>
      <div class="oa-actions">
        <ElButton v-if="blocked" :loading="busy" @click="reload"
          >重新读取合同</ElButton
        >
        <ElButton v-else native-type="submit" type="primary" :loading="busy">{{
          changesState ? "确认" + labels[kind] : "保存合同"
        }}</ElButton>
        <ElButton :disabled="busy || Boolean(blocked)" @click="emit('cancel')"
          >取消</ElButton
        >
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
