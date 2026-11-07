<script setup>
import { onBeforeUnmount, shallowRef, useTemplateRef } from 'vue'
import { ElButton, ElTag } from 'element-plus'
import { ArrowRight, Iphone, OfficeBuilding, SwitchButton, User } from '@element-plus/icons-vue'
import EmployeeWorkspace from './employee-workspace.vue'
import LoginForm from './login-form.vue'
import { nameInitial, roleLabels } from './format'
import { useSession } from './session'

const session = useSession()
const workspace = useTemplateRef('workspace')
const locked = shallowRef(false)

function logout() {
  session.logout()
  locked.value = false
}

onBeforeUnmount(logout)
</script>

<template>
  <div v-if="session.account" class="oa-frame oa-shell">
    <a class="oa-skip" href="#oa-content">跳转到内容</a>
    <aside class="oa-sidebar">
      <div class="oa-brand">
        <span class="oa-brand-mark" aria-hidden="true"><OfficeBuilding /></span>
        <span><strong>HR OA</strong><small>人事工作台</small></span>
      </div>
      <nav class="oa-navigation" aria-label="主导航">
        <p class="oa-nav-label">人事业务</p>
        <button type="button" class="oa-nav-item" aria-current="page" :disabled="locked" @click="workspace.showList()">
          <User aria-hidden="true" />
          <span>员工档案</span>
        </button>
        <a class="oa-nav-item" href="./h5.html">
          <Iphone aria-hidden="true" />
          <span>移动审批</span>
          <ArrowRight class="oa-nav-arrow" aria-hidden="true" />
        </a>
      </nav>
      <p class="oa-sidebar-footer">本地开发工作区</p>
    </aside>

    <div class="oa-workspace">
      <header class="oa-topbar">
        <div class="oa-breadcrumb"><span>人事管理</span><ArrowRight aria-hidden="true" /><strong>员工档案</strong></div>
        <div class="oa-identity">
          <span class="oa-avatar" aria-hidden="true">{{ nameInitial(session.account.user.displayName) }}</span>
          <span class="oa-account-name">{{ session.account.user.displayName }}</span>
          <ElTag effect="plain" type="info">{{ roleLabels[session.account.user.role] }}</ElTag>
          <ElButton class="oa-logout" :icon="SwitchButton" @click="logout">退出登录</ElButton>
        </div>
      </header>
      <main id="oa-content" class="oa-main" tabindex="-1">
        <EmployeeWorkspace ref="workspace" @locked="locked = $event" />
      </main>
      <footer class="oa-footer">HR OA · 基于 Gin-Vue-Admin</footer>
    </div>
  </div>

  <div v-else class="oa-frame oa-auth">
    <aside class="oa-auth-intro" aria-label="产品介绍">
      <div class="oa-auth-copy">
        <div class="oa-brand">
          <span class="oa-brand-mark" aria-hidden="true"><OfficeBuilding /></span>
          <span>HR OA</span>
        </div>
        <h1>人事管理工作台</h1>
        <p>集中维护员工档案和劳动合同，按账号权限查看和办理人事业务。</p>
        <ul class="oa-auth-features">
          <li><User aria-hidden="true" /> 员工档案与劳动合同</li>
          <li><Iphone aria-hidden="true" /> 移动申请与审批</li>
        </ul>
      </div>
    </aside>
    <main class="oa-auth-content">
      <LoginForm />
      <a class="oa-h5-link" href="./h5.html">前往移动审批 <ArrowRight aria-hidden="true" /></a>
      <p class="oa-auth-credit">HR OA · 基于 Gin-Vue-Admin</p>
    </main>
  </div>
</template>
