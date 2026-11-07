<script setup lang="ts">
import { shallowRef } from "vue";
import WeComLogin from "./wecom-login.vue";
import { ElAlert, ElButton } from "element-plus";
import { useLoginForm } from "./use-login-form";

const wecomBusy = shallowRef(false);
const emit = defineEmits<{ authenticated: [] }>();
const { session, username, password, busy, error, submit } = useLoginForm();

async function authenticate() {
  if (wecomBusy.value) return;
  if (await submit()) emit("authenticated");
}
</script>

<template>
  <section class="oa-surface oa-login" aria-labelledby="login-heading">
    <p class="oa-login-label">账号登录</p>
    <h2 id="login-heading">登录人事管理系统</h2>
    <p class="oa-muted">请使用分配给你的账号。</p>
    <ElAlert
      v-if="error || session.notice"
      :title="error || session.notice"
      :closable="false"
      type="warning"
      show-icon
    />
    <form class="oa-form" @submit.prevent="authenticate">
      <label for="oa-username">账号</label>
      <input
        id="oa-username"
        v-model="username"
        required
        autocomplete="username"
        autocapitalize="off"
        :disabled="busy || wecomBusy"
      />
      <label for="oa-password">密码</label>
      <input
        id="oa-password"
        v-model="password"
        required
        type="password"
        autocomplete="current-password"
        :disabled="busy || wecomBusy"
      />
      <ElButton native-type="submit" type="primary" :loading="busy" :disabled="wecomBusy">登录</ElButton>
    </form>
    <WeComLogin :disabled="busy" @busy="wecomBusy = $event" @authenticated="emit('authenticated')" />
  </section>
</template>
