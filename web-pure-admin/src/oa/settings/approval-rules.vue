<script setup>
import {
  computed,
  nextTick,
  ref,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { ElButton } from "element-plus";
import { parseSalary, formatSalary } from "../format";
import { useSession } from "../session";
import { useRemote } from "../use-remote";
import { useVersionedWrite } from "../use-versioned-write";
import { typeLabels } from "../approvals/approval";
import RemoteState from "../remote-state.vue";
import PageNav from "../page-nav.vue";
const emit = defineEmits(["locked"]);
const session = useSession();
const listHeading = useTemplateRef("listHeading");
const page = shallowRef(1);
const candidatePage = shallowRef(1);
const editor = shallowRef(null);
const draft = ref(null);
const validation = shallowRef("");
const feedback = shallowRef("");
const heading = useTemplateRef("heading");
const {
  result,
  loading,
  error: readError,
  load
} = useRemote(signal =>
  session.authorized("/approval-rules?page=" + page.value + "&pageSize=20", {
    signal
  })
);
const candidates = useRemote(signal =>
  session.authorized(
    "/approval-rules/assignees?page=" + candidatePage.value + "&pageSize=20",
    { signal }
  )
);
const departments = useRemote(signal =>
  session.authorized("/departments/tree", { signal })
);
const write = useVersionedWrite();
const { busy, blocked, error } = write;
const disabled = computed(() => busy.value || Boolean(blocked.value));
const departmentOptions = computed(() => {
  const rows = [];
  function visit(list, depth = 0) {
    for (const row of list || []) {
      rows.push({ id: row.id, label: "　".repeat(depth) + row.name });
      visit(row.children, depth + 1);
    }
  }
  visit(departments.result.value?.list);
  return rows;
});
watch(page, load, { immediate: true });
void departments.load();
watch(candidatePage, () => {
  if (editor.value) void candidates.load();
});
function begin(row) {
  if (disabled.value) return;
  editor.value = row;
  validation.value = "";
  error.value = "";
  if (row) {
    draft.value =
      row === "new"
        ? {
            name: "",
            type: "GENERAL",
            departmentId: null,
            minAmount: "",
            maxAmount: "",
            priority: 100,
            enabled: true,
            steps: [{ name: "", assigneeId: null }]
          }
        : {
            ...JSON.parse(JSON.stringify(row)),
            minAmount:
              row.minAmountCents === null
                ? ""
                : String(row.minAmountCents / 100),
            maxAmount:
              row.maxAmountCents === null
                ? ""
                : String(row.maxAmountCents / 100)
          };
    candidatePage.value = 1;
    void candidates.load();
    void departments.load();
    void nextTick(() => heading.value?.focus());
  } else {
    draft.value = null;
    void nextTick(() => listHeading.value?.focus());
  }
  emit("locked", Boolean(row));
}
function move(index, delta) {
  const target = index + delta;
  if (target < 0 || target >= draft.value.steps.length || disabled.value)
    return;
  const row = draft.value.steps.splice(index, 1)[0];
  draft.value.steps.splice(target, 0, row);
}
function optionsFor(step) {
  const list = candidates.result.value?.list || [];
  return step.assigneeId && !list.some(item => item.id === step.assigneeId)
    ? [
        { id: step.assigneeId, displayName: "已选择账号 #" + step.assigneeId },
        ...list
      ]
    : list;
}
async function save() {
  const form = draft.value;
  const data = {
    name: form.name.trim(),
    type: form.type,
    departmentId: form.departmentId,
    minAmountCents: null,
    maxAmountCents: null,
    priority: Number(form.priority),
    enabled: form.enabled,
    steps: form.steps.map(step => ({
      name: step.name.trim(),
      assigneeId: step.assigneeId
    }))
  };
  if (
    !data.name ||
    [...data.name].length > 100 ||
    !Number.isInteger(data.priority) ||
    data.priority < 1 ||
    data.priority > 10000 ||
    !data.steps.length ||
    data.steps.some(
      step => !step.name || [...step.name].length > 100 || !step.assigneeId
    )
  ) {
    validation.value =
      "请填写规则名称、1–10000 的优先级及每一步的名称和审批人。";
    return;
  }
  if (data.type === "EXPENSE") {
    for (const [key, field] of [
      ["minAmountCents", "minAmount"],
      ["maxAmountCents", "maxAmount"]
    ]) {
      if (form[field] !== "") {
        data[key] = parseSalary(String(form[field]));
        if (data[key] === null || data[key] < 1) {
          validation.value =
            "金额留空表示不限制；填写时须大于零且最多两位小数。";
          return;
        }
      }
    }
    if (
      data.minAmountCents !== null &&
      data.maxAmountCents !== null &&
      data.minAmountCents > data.maxAmountCents
    ) {
      validation.value = "金额下限不能超过上限。";
      return;
    }
  }
  if (editor.value !== "new") data.version = editor.value.version;
  const saved = await write.send(
    "/approval-rules" + (editor.value === "new" ? "" : "/" + editor.value.id),
    editor.value === "new" ? "POST" : "PUT",
    data
  );
  if (saved) {
    begin(null);
    feedback.value = "审批规则已保存，只影响新申请。";
    await load();
  }
}
async function reconcile() {
  await write.reconcile(load, () => {
    begin(null);
    feedback.value = "已重新读取规则。请核对当前规则与版本后再操作。";
  });
}
</script>
<template>
  <section aria-labelledby="rules-heading">
    <header v-show="!draft" class="oa-section-head">
      <div>
        <h2 id="rules-heading" ref="listHeading" tabindex="-1">顺序审批规则</h2>
        <p class="oa-muted">
          优先级越大越先匹配；没有匹配规则时使用演示默认路径。既有申请保留原路径。
        </p>
      </div>
      <ElButton :disabled="Boolean(editor) || loading" @click="begin('new')"
        >新增审批规则</ElButton
      >
    </header>
    <RemoteState :loading="loading" :error="readError" @retry="load" />
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <form
      v-if="draft"
      class="oa-form oa-settings-task"
      aria-describedby="rule-error"
      @submit.prevent="save"
    >
      <h3 ref="heading" tabindex="-1">
        {{
          editor === "new"
            ? "新增审批规则"
            : "编辑审批规则 · 版本 " + editor.version
        }}
      </h3>
      <div class="oa-fields">
        <label
          >规则名称<input
            v-model="draft.name"
            maxlength="100"
            required
            :disabled="disabled"
        /></label>
        <label
          >申请类型<select v-model="draft.type" :disabled="disabled">
            <option
              v-for="(label, value) in typeLabels"
              :key="value"
              :value="value"
            >
              {{ label }}
            </option>
          </select></label
        >
        <label
          >直属部门范围（不含子部门）<select
            v-model="draft.departmentId"
            :disabled="disabled || departments.loading.value"
          >
            <option :value="null">全员</option>
            <option
              v-if="
                draft.departmentId &&
                !departmentOptions.some(item => item.id === draft.departmentId)
              "
              :value="draft.departmentId"
            >
              已选择部门 #{{ draft.departmentId }}
            </option>
            <option
              v-for="row in departmentOptions"
              :key="row.id"
              :value="row.id"
            >
              {{ row.label }}
            </option>
          </select></label
        >
        <label
          >优先级<input
            v-model="draft.priority"
            type="number"
            min="1"
            max="10000"
            required
            :disabled="disabled"
        /></label>
        <label
          >使用状态<select v-model="draft.enabled" :disabled="disabled">
            <option :value="true">启用</option>
            <option :value="false">停用</option>
          </select></label
        >
        <template v-if="draft.type === 'EXPENSE'"
          ><label
            >金额下限（元，含）<input
              v-model="draft.minAmount"
              inputmode="decimal"
              placeholder="不限制"
              :disabled="disabled" /></label
          ><label
            >金额上限（元，含）<input
              v-model="draft.maxAmount"
              inputmode="decimal"
              placeholder="不限制"
              :disabled="disabled" /></label
        ></template>
      </div>
      <RemoteState :error="departments.error.value" @retry="departments.load" />
      <h3>审批顺序</h3>
      <p class="oa-muted">依次执行 1–10 步；审批人不能是申请人。</p>
      <ol class="oa-rule-steps">
        <li v-for="(step, index) in draft.steps" :key="index">
          <h4 class="oa-step-index">第 {{ index + 1 }} 步</h4>
          <div class="oa-fields">
            <label
              >步骤名称<input
                v-model="step.name"
                maxlength="100"
                required
                :disabled="disabled" /></label
            ><label
              >审批人<select
                v-model="step.assigneeId"
                :disabled="disabled"
                required
              >
                <option :value="null" disabled>请选择审批人</option>
                <option
                  v-for="person in optionsFor(step)"
                  :key="person.id"
                  :value="person.id"
                >
                  {{ person.displayName }}
                </option>
              </select></label
            >
          </div>
          <div class="oa-actions">
            <ElButton
              :disabled="disabled || index === 0"
              :aria-label="'上移第 ' + (index + 1) + ' 步'"
              @click="move(index, -1)"
              >上移</ElButton
            ><ElButton
              :disabled="disabled || index === draft.steps.length - 1"
              :aria-label="'下移第 ' + (index + 1) + ' 步'"
              @click="move(index, 1)"
              >下移</ElButton
            ><ElButton
              :disabled="disabled || draft.steps.length === 1"
              :aria-label="'删除第 ' + (index + 1) + ' 步'"
              @click="draft.steps.splice(index, 1)"
              >删除步骤</ElButton
            >
          </div>
        </li>
      </ol>
      <aside class="oa-assignee-pages" aria-label="审批人候选范围">
        <h4>审批人候选</h4>
        <p class="oa-muted">
          每页最多 20
          个账号，翻页后可在各步骤的审批人选项中选择。已选择的账号会保留。
        </p>
        <RemoteState
          :loading="candidates.loading.value"
          :error="candidates.error.value"
          @retry="candidates.load"
        />
        <PageNav
          v-if="candidates.result.value"
          :page="candidatePage"
          :total="candidates.result.value.total"
          :page-size="candidates.result.value.pageSize"
          :disabled="disabled || candidates.loading.value"
          label="审批人候选分页"
          @change="candidatePage = $event"
        />
      </aside>
      <ElButton
        :disabled="disabled || draft.steps.length >= 10"
        @click="draft.steps.push({ name: '', assigneeId: null })"
        >添加审批步骤</ElButton
      >
      <p v-if="validation || error" id="rule-error" role="alert">
        {{ validation || error }}
      </p>
      <div class="oa-actions">
        <ElButton v-if="blocked" :loading="busy" @click="reconcile"
          >重新读取并核对</ElButton
        ><ElButton v-else type="primary" native-type="submit" :loading="busy"
          >保存审批规则</ElButton
        ><ElButton :disabled="disabled" @click="begin(null)">取消</ElButton>
      </div>
    </form>
    <ul v-if="result && !draft" class="oa-settings-list oa-rule-list">
      <li v-for="row in result.list" :key="row.id">
        <div class="oa-rule-summary">
          <strong>{{ row.name }}</strong>
          <dl class="oa-rule-facts">
            <div>
              <dt>类型</dt>
              <dd>{{ typeLabels[row.type] }}</dd>
            </div>
            <div>
              <dt>范围</dt>
              <dd>
                {{
                  row.departmentId === null
                    ? "全员"
                    : (departmentOptions
                        .find(item => item.id === row.departmentId)
                        ?.label.trim() || "部门 #" + row.departmentId) +
                      "（直属）"
                }}
              </dd>
            </div>
            <div>
              <dt>优先级</dt>
              <dd>{{ row.priority }}</dd>
            </div>
            <div>
              <dt>状态</dt>
              <dd>
                {{ row.enabled ? "启用" : "停用" }} · 版本 {{ row.version }}
              </dd>
            </div>
          </dl>
          <p class="oa-muted">
            {{ row.steps.length }} 步：{{
              row.steps.map(step => step.name).join(" → ")
            }}
          </p>
          <p v-if="row.type === 'EXPENSE'" class="oa-muted">
            金额
            {{
              row.minAmountCents === null
                ? "无下限"
                : formatSalary(row.minAmountCents)
            }}
            至
            {{
              row.maxAmountCents === null
                ? "无上限"
                : formatSalary(row.maxAmountCents)
            }}
          </p>
        </div>
        <ElButton
          :disabled="loading"
          :aria-label="'编辑' + row.name"
          @click="begin(row)"
          >编辑</ElButton
        >
      </li>
    </ul>
    <p
      v-if="result && !result.list.length && !draft && !loading"
      class="oa-status"
    >
      暂无审批规则。未匹配的申请将使用默认顺序路径。
    </p>
    <PageNav
      v-if="result && !draft"
      :page="page"
      :total="result.total"
      :page-size="result.pageSize"
      :disabled="Boolean(editor) || loading"
      label="审批规则分页"
      @change="page = $event"
    />
  </section>
</template>

<style scoped>
.oa-rule-list > li {
  align-items: start;
}

.oa-rule-summary {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

.oa-rule-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 24px;
  margin: 8px 0;
  font-size: 14px;
}

.oa-rule-facts > div {
  display: flex;
  gap: 8px;
}

.oa-rule-facts dt {
  color: var(--oa-muted);
}

.oa-rule-facts dd {
  margin: 0;
}

.oa-rule-steps {
  padding: 0;
  margin: 0;
  list-style: none;
}

.oa-rule-steps > li {
  padding: 12px 0;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--oa-line);
  border-radius: 0;
}

.oa-step-index {
  margin: 0 0 8px;
  color: var(--oa-accent-text);
}

.oa-assignee-pages {
  padding: 12px;
  margin: 8px 0;
  border-inline-start: 1px solid var(--oa-line);
}

.oa-assignee-pages h4,
.oa-assignee-pages p {
  margin: 0 0 8px;
}

.oa-settings-task {
  max-width: 900px;
}

.oa-settings-task .oa-fields {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

@media (width <= 38rem) {
  .oa-settings-task .oa-fields {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
