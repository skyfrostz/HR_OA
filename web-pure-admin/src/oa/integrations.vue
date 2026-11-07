<script setup>
import { onMounted, shallowRef, useTemplateRef, watch } from 'vue'
import { ElButton, ElTag } from 'element-plus'
import { request } from './api'
import { useSession } from './session'
import { useRemote } from './use-remote'
import RemoteState from './remote-state.vue'
import PageNav from './page-nav.vue'
const session = useSession()
const heading = useTemplateRef('heading')
onMounted(() => heading.value?.focus())
const page = shallowRef(1)
const status = useRemote(signal => request('/integration/status', { signal }))
const deliveries = useRemote(signal => session.authorized('/notification-deliveries?page=' + page.value + '&pageSize=20', { signal }))
watch(page, deliveries.load, { immediate: true }); void status.load()
const loginLabels = { disabled: '尚未配置', mock: '模拟授权', configured: '已配置 · 待企业验收' }
const notificationLabels = { mock: '本地模拟', 'wecom-mock': '模拟企业微信 API', wecom: '企业微信 API · 待企业验收' }
const storageLabels = { local: '本地私有存储', 'oss-mock': '模拟 OSS', oss: 'OSS · 待企业验收' }
const deliveryLabels = { PENDING: '等待处理', ACCEPTED: 'API 已接受', FAILED: '处理失败', UNCERTAIN: '结果未知', SKIPPED: '已跳过', MOCK: '本地模拟记录' }
const date = value => new Date(value).toLocaleString('zh-CN', { hour12: false })
</script>
<template>
  <section class="oa-pure" aria-labelledby="oa-integrations-heading">
    <header class="oa-section-head oa-page-heading"><div><h1 id="oa-integrations-heading" ref="heading" tabindex="-1">集成状态</h1><p class="oa-muted">查看企业微信与附件存储的实际运行模式。</p></div><ElButton :disabled="status.loading.value" @click="status.load">刷新状态</ElButton></header>
    <RemoteState :loading="status.loading.value" :error="status.error.value" @retry="status.load" />
    <div v-if="status.result.value && !status.loading.value" class="oa-integration-grid">
      <section class="oa-surface"><h2>企业微信登录</h2><p class="oa-integration-status">{{ loginLabels[status.result.value.wecomLogin] || '未知状态' }}</p><p class="oa-muted">先开通员工账号，再在员工档案的“企业微信”中绑定成员 ID。</p></section>
      <section class="oa-surface"><h2>通知投递</h2><p class="oa-integration-status">{{ notificationLabels[status.result.value.notifications] || '未知状态' }}</p><p class="oa-muted">API 接受仅代表请求被服务接收，不代表成员已收到或已读。</p></section>
      <section class="oa-surface"><h2>合同附件</h2><p class="oa-integration-status">{{ storageLabels[status.result.value.attachmentStorage] || '未知状态' }}</p><p class="oa-muted">每次预览和下载都核验权限；附件按各自存储位置读取。</p></section>
    </div>
    <section class="oa-surface oa-import-preview"><h2>配置与启用</h2><ol class="oa-setup-steps"><li>由运维在服务端配置企业微信应用、回调域名与私有 OSS。密钥只保存在服务端。</li><li>在员工档案中核对已有账号，并显式绑定企业成员 ID。</li><li>使用模拟环境检查登录、通知和附件，再在资源就绪后完成真实企业环境验收。</li></ol><p class="oa-muted">模拟模式用于联调；接入真实企业资源后，需再验证实际成员登录、消息投递和附件读取。</p></section>
    <section class="oa-surface oa-import-preview" aria-labelledby="oa-deliveries-heading">
      <header class="oa-section-head"><div><h2 id="oa-deliveries-heading">通知处理记录</h2><p class="oa-muted">历史记录保留创建时的投递模式。</p></div><ElButton :disabled="deliveries.loading.value" @click="deliveries.load">刷新处理记录</ElButton></header>
      <RemoteState :loading="deliveries.loading.value" :error="deliveries.error.value" :empty="Boolean(deliveries.result.value && !deliveries.result.value.list.length)" empty-text="暂无通知处理记录。" @retry="deliveries.load" />
      <div v-if="!deliveries.loading.value && deliveries.result.value?.list.length" class="oa-table-wrap"><table class="oa-table" aria-label="通知处理记录"><thead><tr><th>记录</th><th>运行模式</th><th>处理状态</th><th>尝试次数</th><th>创建时间</th></tr></thead><tbody><tr v-for="row in deliveries.result.value.list" :key="row.id"><th scope="row">#{{ row.id }}</th><td data-label="运行模式">{{ notificationLabels[row.mode] || row.mode }}</td><td data-label="处理状态"><ElTag class="oa-delivery-status" :type="['FAILED', 'UNCERTAIN'].includes(row.status) ? 'warning' : 'info'" effect="plain">{{ deliveryLabels[row.status] || row.status }}</ElTag></td><td data-label="尝试次数">{{ row.attempts }}</td><td data-label="创建时间">{{ date(row.createdAt) }}</td></tr></tbody></table></div>
      <PageNav v-if="deliveries.result.value" :page="page" :total="deliveries.result.value.total" :page-size="deliveries.result.value.pageSize" :disabled="deliveries.loading.value" label="通知处理记录分页" @change="page = $event" />
    </section>
  </section>
</template>
