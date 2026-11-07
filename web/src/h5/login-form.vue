<script setup>
import { shallowRef } from 'vue'
import WeComLogin from './wecom-login.vue'
import { Button } from 'vant'
import { useLoginForm } from '../oa/use-login-form'

const wecomBusy = shallowRef(false)
const { session, username, password, busy, error, submit } = useLoginForm()
</script>

<template>
  <section class="h5-card h5-login" aria-labelledby="h5-login-title">
    <p class="h5-kicker">账号登录</p>
    <h1 id="h5-login-title">登录移动审批</h1>
    <p class="h5-muted">使用分配给你的账号登录。</p>
    <p v-if="error || session.notice" class="h5-message h5-message-error" role="alert">
      {{ error || session.notice }}
    </p>
    <form class="h5-form" @submit.prevent="!wecomBusy && submit()">
      <label for="h5-username">账号</label>
      <input id="h5-username" v-model="username" required autocomplete="username" autocapitalize="off" :disabled="busy || wecomBusy">
      <label for="h5-password">密码</label>
      <input id="h5-password" v-model="password" required type="password" autocomplete="current-password" :disabled="busy || wecomBusy">
      <Button type="primary" native-type="submit" block :loading="busy" :disabled="busy || wecomBusy">登录</Button>
    </form>
    <WeComLogin :disabled="busy" @busy="wecomBusy = $event" />
  </section>
</template>
