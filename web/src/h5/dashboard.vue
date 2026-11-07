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
    <ul v-if="!loading && result" class="h5-list"><li v-for="entry in entries" :key="entry.key"><button class="h5-card h5-record-button" type="button" @click="emit('view', entry.view)"><strong>{{ entry.label }} · {{ result[entry.key] }}</strong><span class="h5-muted">查看列表</span></button></li><li v-if="typeof result.contractReminders === 'number'" class="h5-card"><strong>合同到期 · {{ result.contractReminders }}</strong><p class="h5-muted">逾期及未来 30 天</p><a href="./oa.html#/contract-reminders">前往 PC 管理端查看合同</a></li></ul>
    <Button v-if="session.account?.user.employeeId" class="h5-block" @click="emit('view', 'balance')">我的假期与流水</Button>
  </section>
</template>
