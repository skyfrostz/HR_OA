<script setup lang="ts">
import { ElAlert, ElButton } from "element-plus";
import { useLoginForm } from "./use-login-form";

const emit = defineEmits<{ authenticated: [] }>();
const { session, username, password, busy, error, submit } = useLoginForm();

async function authenticate() {
  if (await submit()) emit("authenticated");
}
</script>

<template>
  <section class="oa-surface oa-login" aria-labelledby="login-heading">
    <p class="oa-login-label">本地开发</p>
    <h2 id="login-heading">登录人事管理系统</h2>
    <p class="oa-muted">请使用分配给你的开发账号。</p>
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
        :disabled="busy"
      />
      <label for="oa-password">密码</label>
      <input
        id="oa-password"
        v-model="password"
        required
        type="password"
        autocomplete="current-password"
        :disabled="busy"
      />
      <ElButton native-type="submit" type="primary" :loading="busy">登录</ElButton>
    </form>
  </section>
</template>
