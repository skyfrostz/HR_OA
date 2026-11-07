<script setup>
import { onScopeDispose, shallowRef } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { useSession } from './session'

const session = useSession()
const username = shallowRef('')
const password = shallowRef('')
const busy = shallowRef(false)
const error = shallowRef('')
const controller = new AbortController()

onScopeDispose(() => controller.abort())

async function submit() {
  if (busy.value || !username.value.trim() || !password.value) return
  busy.value = true
  error.value = ''
  const enteredPassword = password.value
  password.value = ''
  try {
    await session.login(username.value.trim(), enteredPassword, controller.signal)
  } catch (cause) {
    if (!controller.signal.aborted) error.value = cause.message
  } finally {
    busy.value = false
  }
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
    <form class="oa-form" @submit.prevent="submit">
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
