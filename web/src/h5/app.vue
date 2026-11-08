<script setup>
  import { onBeforeUnmount, shallowRef, watch } from 'vue'
  import { Button } from 'vant'
  import { OfficeBuilding } from '@element-plus/icons-vue'
  import { roleLabels } from '../oa/format'
  import { useSession } from '../oa/session'
  import LoginForm from './login-form.vue'
  import Workspace from './workspace.vue'

  const session = useSession()
  const locked = shallowRef(false)
  const taskMode = shallowRef(false)
  watch(
    () => session.account,
    (account) => {
      if (!account) {
        locked.value = false
        taskMode.value = false
      }
    }
  )
  onBeforeUnmount(() => session.logout())
</script>

<template>
  <div class="h5-frame">
    <a class="h5-skip" href="#h5-content">跳转到内容</a>
    <header v-if="!taskMode" class="h5-header">
      <div class="h5-header-inner">
        <div class="h5-brand">
          <span class="h5-mark" aria-hidden="true"><OfficeBuilding /></span>
          <span><strong>移动审批</strong><small>HR OA 人事工作台</small></span>
        </div>
        <div class="h5-header-actions">
          <a :href="locked ? undefined : './oa.html'" :aria-disabled="locked"
            >PC 管理端</a
          >
          <template v-if="session.account">
            <span class="h5-account-name"
              >{{ session.account.user.displayName }} ·
              {{ roleLabels[session.account.user.role] }}</span
            >
            <Button size="small" plain @click="session.logout()"
              >退出登录</Button
            >
          </template>
        </div>
      </div>
    </header>
    <Workspace
      v-if="session.account"
      @locked="locked = $event"
      @task-mode="taskMode = $event"
    />
    <main v-else id="h5-content" class="h5-main" tabindex="-1">
      <LoginForm />
    </main>
  </div>
</template>
