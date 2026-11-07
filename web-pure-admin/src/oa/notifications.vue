<script setup>
import { onScopeDispose, onMounted, shallowRef, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElAlert, ElButton, ElTag } from 'element-plus'
import { useSession } from './session'
import { useRemote } from './use-remote'
import { employeeNavigation } from './navigation'
import RemoteState from './remote-state.vue'
import PageNav from './page-nav.vue'
const session = useSession()
const router = useRouter()
const controller = new AbortController()
onScopeDispose(() => controller.abort())
const heading = useTemplateRef('heading')
onMounted(() => heading.value?.focus())
const page = shallowRef(1)
const pending = shallowRef(null)
const actionError = shallowRef('')
const { result, loading, error, load } = useRemote(signal => session.authorized('/notifications?page=' + page.value + '&pageSize=20', { signal }))
watch(page, load, { immediate: true })
const mode = { mock: '模拟', 'wecom-mock': '模拟 API 已接受', wecom: '企业 API 已接受' }
async function open(record) {
  if (pending.value !== null) return
  pending.value = record.id; actionError.value = ''
  const nav = employeeNavigation()
  try {
    if (record.kind === 'CONTRACT_EXPIRING' && record.contractId) {
      const contract = await session.authorized('/contracts/' + record.contractId, { signal: controller.signal })
      nav.employeeId = contract.employeeId; nav.contractId = contract.id
      if (await router.push('/employees')) { nav.employeeId = null; nav.contractId = null }
    } else if (record.requestId > 0 && (session.account?.permissions.canSubmitRequests || session.account?.permissions.canApproveRequests)) {
      nav.requestId = record.requestId
      if (await router.push('/approvals')) nav.requestId = null
    } else actionError.value = '此通知没有当前账号可打开的详情。'
  } catch (cause) { actionError.value = cause.message }
  finally { pending.value = null }
}
</script>
<template>
  <section class="oa-pure" aria-labelledby="oa-notifications-heading">
    <header class="oa-section-head oa-page-heading"><div><h1 id="oa-notifications-heading" ref="heading" tabindex="-1">站内通知</h1><p class="oa-muted">查看与你相关的事项。API 接受不代表消息已送达或已读。</p></div><ElButton :disabled="loading" @click="load">刷新通知</ElButton></header>
    <RemoteState :loading="loading" :error="error" :empty="Boolean(result && !result.list.length)" empty-text="暂无站内通知。" @retry="load" />
    <ElAlert v-if="actionError" :title="actionError" type="warning" :closable="false" />
    <ul v-if="!loading && result?.list.length" class="oa-notification-list"><li v-for="record in result.list" :key="record.id" class="oa-surface"><div class="oa-section-head"><h2>{{ record.title }}</h2><ElTag class="oa-delivery-status" effect="plain" type="info">{{ mode[record.deliveryMode] || '投递模式未知' }}</ElTag></div><p class="oa-muted">{{ new Date(record.createdAt).toLocaleString('zh-CN', {hour12:false}) }}</p><ElButton class="oa-notification-open" :disabled="pending !== null" :loading="pending === record.id" @click="open(record)">{{ record.kind === 'CONTRACT_EXPIRING' ? '查看合同详情' : '查看申请详情' }}</ElButton></li></ul>
    <PageNav v-if="result" :page="page" :total="result.total" :page-size="result.pageSize" :disabled="loading || pending !== null" label="站内通知分页" @change="page = $event" />
  </section>
</template>
