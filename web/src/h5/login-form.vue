<script setup>
import { Button } from 'vant'
import { useLoginForm } from '../oa/use-login-form'

const { session, username, password, busy, error, submit } = useLoginForm()
</script>

<template>
  <section class="h5-card h5-login" aria-labelledby="h5-login-title">
    <p class="h5-kicker">本地开发</p>
    <h1 id="h5-login-title">登录移动审批</h1>
    <p class="h5-muted">使用本地开发账号登录。站内通知使用模拟发送。</p>
    <p v-if="error || session.notice" class="h5-message h5-message-error" role="alert">
      {{ error || session.notice }}
    </p>
    <form class="h5-form" @submit.prevent="submit">
      <label for="h5-username">账号</label>
      <input id="h5-username" v-model="username" required autocomplete="username" autocapitalize="off" :disabled="busy">
      <label for="h5-password">密码</label>
      <input id="h5-password" v-model="password" required type="password" autocomplete="current-password" :disabled="busy">
      <Button type="primary" native-type="submit" block :loading="busy" :disabled="busy">登录</Button>
    </form>
  </section>
</template>
