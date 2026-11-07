<script setup>
import { computed, onMounted, useTemplateRef } from 'vue'
import { Button } from 'vant'
import { useSession } from '../oa/session'
import { useRemote } from '../oa/use-remote'
import PageState from './page-state.vue'
const emit = defineEmits(['view'])
const session = useSession()
const heading = useTemplateRef('heading')
onMounted(() => heading.value?.focus())
const { result, loading, error, load } = useRemote(signal => session.authorized('/dashboard?days=30', { signal }))
const entries = computed(() => result.value ? [
  ...(session.account?.permissions.canApproveRequests ? [{ key: 'pendingTasks', label: '待我审批', view: 'pending' }] : []),
  ...(session.account?.permissions.canSubmitRequests ? [{ key: 'myRequests', label: '我的申请', view: 'mine' }] : [])
].filter(item => typeof result.value[item.key] === 'number') : [])
void load()
</script>
<template>
  <section aria-labelledby="h5-dashboard-heading">
    <header class="h5-list-heading"><h1 id="h5-dashboard-heading" ref="heading" tabindex="-1">工作台</h1><Button :disabled="loading" @click="load">刷新工作台</Button></header>
    <p class="h5-muted">{{ session.account?.user.displayName }}，查看当前事项与假期余额。</p>
    <PageState :loading="loading" :error="error" @retry="load" />
    <ul v-if="!loading && result" class="h5-list h5-dashboard-list">
      <li v-for="entry in entries" :key="entry.key">
        <button class="h5-card h5-record-button h5-dashboard-entry" type="button" @click="emit('view', entry.view)">
          <span><strong>{{ entry.label }}</strong><span class="h5-template-description">查看列表</span></span>
          <span class="h5-dashboard-number">{{ result[entry.key] }}</span>
        </button>
      </li>
      <li v-if="typeof result.contractReminders === 'number'" class="h5-card">
        <div class="h5-dashboard-entry"><span><strong>合同到期</strong><span class="h5-template-description">逾期及未来 30 天</span></span><span class="h5-dashboard-number">{{ result.contractReminders }}</span></div>
        <a class="h5-dashboard-link" href="./oa.html#/contract-reminders">前往 PC 管理端查看合同</a>
      </li>
    </ul>
    <Button v-if="session.account?.user.employeeId" class="h5-block" @click="emit('view', 'balance')">我的假期与流水</Button>
  </section>
</template>
