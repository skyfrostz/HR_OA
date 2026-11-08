<script setup>
import {
  computed,
  nextTick,
  reactive,
  shallowRef,
  useTemplateRef,
  watch
} from "vue";
import { ElAlert, ElButton, ElTag } from "element-plus";
import { listPage } from "./api";
import { useSession } from "./session";
import { useRemote } from "./use-remote";
import { useVersionedWrite } from "./use-versioned-write";
import PageNav from "./page-nav.vue";
import RemoteState from "./remote-state.vue";

const props = defineProps({
  employeeId: { type: Number, required: true },
  unavailable: Boolean
});
const emit = defineEmits(["editing", "employment", "changed"]);
const session = useSession();
const path = "/employees/" + props.employeeId;
const {
  result,
  loading,
  error: readError,
  load
} = useRemote(signal => session.authorized(path + "/account", { signal }));
const action = shallowRef("");
const feedback = shallowRef("");
const validation = shallowRef("");
const page = shallowRef(1);
const heading = useTemplateRef("actionHeading");
const accountHeading = useTemplateRef("accountHeading");
const labels = {
  CREATE: "开通账号",
  BIND: "绑定现有账号",
  SET_ROLE: "变更角色",
  RESET_PASSWORD: "重置密码",
  DISABLE: "停用登录",
  ENABLE: "启用登录",
  OFFBOARD: "办理离职"
};
const roles = { EMPLOYEE: "员工", APPROVER: "审批人" };
const form = reactive({
  username: "",
  password: "",
  role: "EMPLOYEE",
  reason: "",
  userId: null,
  handoffUserId: null
});
const account = computed(() => result.value?.account);
const active = computed(() => result.value?.employmentStatus === "ACTIVE");
const operations = computed(() => {
  if (
    !active.value ||
    (account.value && !Object.hasOwn(roles, account.value.role))
  )
    return [];
  if (!account.value) return ["CREATE", "BIND", "OFFBOARD"];
  return [
    "SET_ROLE",
    "RESET_PASSWORD",
    account.value.enabled ? "DISABLE" : "ENABLE",
    "OFFBOARD"
  ];
});
const canHandoff = computed(
  () =>
    account.value?.role === "APPROVER" &&
    (["DISABLE", "OFFBOARD"].includes(action.value) ||
      (action.value === "SET_ROLE" && form.role === "EMPLOYEE"))
);
const options = useRemote(signal =>
  session.authorized(
    listPage(
      path +
        (action.value === "BIND" ? "/account-candidates" : "/handoff-targets"),
      page.value
    ),
    { signal }
  )
);
const write = useVersionedWrite();
const { busy, blocked, error } = write;
watch(result, value => {
  if (value) emit("employment", value.employmentStatus);
});
watch(
  [page, action, canHandoff],
  () => {
    if (action.value === "BIND" || canHandoff.value) void options.load();
  },
  { flush: "post" }
);
void load();

function begin(value) {
  if (busy.value || blocked.value || (value && props.unavailable)) return;
  action.value = value;
  if (value) feedback.value = "";
  validation.value = "";
  error.value = "";
  page.value = 1;
  options.result.value = null;
  Object.assign(form, {
    username: "",
    password: "",
    role: account.value?.role || "EMPLOYEE",
    reason: "",
    userId: null,
    handoffUserId: null
  });
  emit("editing", Boolean(value));
  if (value) void nextTick(() => heading.value?.focus());
}

async function submit() {
  if (!action.value || !result.value || busy.value || blocked.value) return;
  validation.value = "";
  const reason = form.reason.trim();
  if (!reason || [...reason].length > 500) {
    validation.value = "请填写 1–500 字的操作原因。";
    return;
  }
  const data = { action: action.value, version: result.value.version, reason };
  if (["CREATE", "RESET_PASSWORD"].includes(action.value)) {
    const bytes = new TextEncoder().encode(form.password).length;
    if (bytes < 12 || bytes > 72) {
      validation.value = "密码须为 12–72 个 UTF-8 字节。";
      return;
    }
    data.password = form.password;
  }
  if (action.value === "CREATE") {
    if (!/^[a-zA-Z0-9._-]{3,64}$/.test(form.username.trim())) {
      validation.value =
        "用户名须为 3–64 个英文字母、数字、点、下划线或连字符。";
      return;
    }
    data.username = form.username.trim();
  }
  if (["CREATE", "SET_ROLE"].includes(action.value)) data.role = form.role;
  if (action.value === "BIND") {
    if (!form.userId) {
      validation.value = "请选择需要绑定的账号。";
      return;
    }
    data.userId = form.userId;
  }
  if (canHandoff.value && form.handoffUserId)
    data.handoffUserId = form.handoffUserId;
  form.password = "";
  const updated = await write.send(path + "/account/actions", "POST", data);
  if (updated) {
    result.value = updated;
    feedback.value = "已完成：" + labels[action.value] + "。";
    begin("");
    emit("changed");
    void nextTick(() => accountHeading.value?.focus());
  }
}
function reload() {
  return write.reconcile(load, () => {
    begin("");
    feedback.value =
      "已重新读取账号状态。请核对当前状态与变更历史，确认是否还需操作。";
    emit("changed");
    void nextTick(() => accountHeading.value?.focus());
  });
}
</script>

<template>
  <section class="oa-surface" aria-labelledby="oa-account-heading">
    <div class="oa-section-head">
      <div>
        <h2 id="oa-account-heading" ref="accountHeading" tabindex="-1">
          员工账号
        </h2>
        <p class="oa-muted">登录权限与员工在职状态分别管理。</p>
      </div>
    </div>
    <RemoteState :loading="loading" :error="readError" @retry="load" />
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <template v-if="result">
      <dl v-if="!action" class="oa-description oa-account-summary">
        <div>
          <dt>在职状态</dt>
          <dd>
            <ElTag effect="plain" :type="active ? 'success' : 'info'">{{
              active ? "在职" : "已离职"
            }}</ElTag>
          </dd>
        </div>
        <div>
          <dt>登录状态</dt>
          <dd>
            {{
              account ? (account.enabled ? "已启用" : "已停用") : "未绑定账号"
            }}
          </dd>
        </div>
        <div v-if="account">
          <dt>用户名</dt>
          <dd>{{ account.username }}</dd>
        </div>
        <div v-if="account">
          <dt>账号角色</dt>
          <dd>{{ roles[account.role] || "管理账号" }}</dd>
        </div>
      </dl>
      <div v-if="!action && operations.length" class="oa-actions">
        <ElButton
          v-for="kind in operations"
          :key="kind"
          :type="kind === 'OFFBOARD' ? 'danger' : undefined"
          :disabled="unavailable || loading"
          plain
          @click="begin(kind)"
          >{{ labels[kind] }}</ElButton
        >
      </div>
    </template>
    <form
      v-if="action"
      class="oa-inline-form oa-form"
      autocomplete="off"
      @submit.prevent="submit"
    >
      <h3 ref="actionHeading" tabindex="-1">{{ labels[action] }}</h3>
      <p v-if="action === 'OFFBOARD'" class="oa-muted">
        离职后将停用登录、撤销现有会话，保留档案及历史记录。
      </p>
      <p
        v-else-if="action === 'DISABLE' || action === 'RESET_PASSWORD'"
        class="oa-muted"
      >
        此操作会撤销当前会话。
      </p>
      <div class="oa-fields">
        <label v-if="action === 'CREATE'"
          >用户名<input
            v-model="form.username"
            required
            :disabled="busy || Boolean(blocked)"
        /></label>
        <label v-if="action === 'CREATE' || action === 'RESET_PASSWORD'"
          >新密码<input
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            required
            :disabled="busy || Boolean(blocked)"
        /></label>
        <label v-if="action === 'CREATE' || action === 'SET_ROLE'"
          >账号角色
          <select v-model="form.role" :disabled="busy || Boolean(blocked)">
            <option v-for="(label, role) in roles" :key="role" :value="role">
              {{ label }}
            </option>
          </select>
        </label>
        <label class="oa-field-wide"
          >操作原因<textarea
            v-model="form.reason"
            required
            maxlength="500"
            placeholder="请勿填写密码、证件号码等敏感信息"
            :disabled="busy || Boolean(blocked)"
          />
        </label>
      </div>
      <template v-if="action === 'BIND' || canHandoff">
        <fieldset :disabled="busy || Boolean(blocked) || options.loading.value">
          <legend>
            {{ action === "BIND" ? "未绑定账号" : "待办与后续审批交接人" }}
          </legend>
          <RemoteState
            :loading="options.loading.value"
            :error="options.error.value"
            :empty="
              Boolean(options.result.value && !options.result.value.list.length)
            "
            empty-text="暂无可选对象。"
            @retry="options.load"
          />
          <label
            v-for="item in options.result.value?.list || []"
            :key="item.id"
            class="oa-option-radio"
          >
            <input
              v-if="action === 'BIND'"
              v-model="form.userId"
              type="radio"
              :value="item.id"
            />
            <input
              v-else
              v-model="form.handoffUserId"
              type="radio"
              :value="item.id"
            />
            {{
              action === "BIND"
                ? item.username +
                  " · " +
                  roles[item.role] +
                  (item.enabled ? " · 已启用" : " · 已停用")
                : item.displayName
            }}
          </label>
          <ElButton
            v-if="canHandoff && form.handoffUserId"
            text
            @click="form.handoffUserId = null"
            >清除交接人</ElButton
          >
        </fieldset>
        <PageNav
          v-if="options.result.value"
          :page="page"
          :page-size="options.result.value.pageSize"
          :total="options.result.value.total"
          label="账号候选分页"
          :disabled="busy || Boolean(blocked) || options.loading.value"
          @change="page = $event"
        />
      </template>
      <ElAlert
        v-if="validation || error"
        :title="validation || error"
        type="error"
        show-icon
        :closable="false"
      />
      <div class="oa-actions">
        <ElButton v-if="blocked" :loading="busy" @click="reload"
          >重新读取账号状态</ElButton
        >
        <ElButton v-else type="primary" native-type="submit" :loading="busy"
          >确认{{ labels[action] }}</ElButton
        >
        <ElButton :disabled="busy || Boolean(blocked)" @click="begin('')"
          >取消</ElButton
        >
      </div>
    </form>
  </section>
</template>

<style scoped>
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
