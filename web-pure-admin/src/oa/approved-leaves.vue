<script setup>
import { nextTick, shallowRef, useTemplateRef, watch } from 'vue'
import { ElButton } from 'element-plus'
import { useSession } from './session'
import { useRemote } from './use-remote'
import { useRouteWriteLock } from './use-route-write-lock'
import RequestActions from './approvals/request-actions.vue'
import RemoteState from './remote-state.vue'
import PageNav from './page-nav.vue'
import './approvals/workspace.css'
const session = useSession()
const page = shallowRef(1)
const selected = shallowRef(null)
const locked = shallowRef(false)
const feedback = shallowRef('')
const heading = useTemplateRef('heading')
const { result, loading, error, load } = useRemote(signal => session.authorized('/approved-leaves?page=' + page.value + '&pageSize=20', { signal }))
useRouteWriteLock(locked)
watch(page, load, { immediate: true })
watch(result, data => { if (data && page.value > Math.max(1, Math.ceil(data.total / data.pageSize))) page.value = Math.max(1, Math.ceil(data.total / data.pageSize)) })
async function changed() { selected.value = null; feedback.value = '请假已撤销，余额按原计算记录冲回。'; await load(); void nextTick(() => heading.value?.focus()) }
async function invalidated(message) { selected.value = null; feedback.value = message; await load() }
</script>
<template>
  <section class="oa-pure" aria-labelledby="approved-leave-heading">
    <header class="oa-section-head oa-page-heading"><div><h1 id="approved-leave-heading" ref="heading" tabindex="-1">已批请假</h1><p class="oa-muted">仅列出可撤销的新版请假摘要；撤销需填写原因，记录与流水会保留。</p></div><ElButton :disabled="locked || loading" @click="load">刷新</ElButton></header>
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <section class="oa-surface">
      <RemoteState :loading="loading" :error="error" :empty="Boolean(result && !result.list.length)" empty-text="暂无可撤销的已批请假。" @retry="load" />
      <ul v-if="result" class="oa-settings-list"><li v-for="row in result.list" :key="row.id"><div><strong>{{ row.applicantName }} · {{ row.leaveTypeName }}</strong><p class="oa-muted">{{ row.startDate }} 至 {{ row.endDate }} · {{ row.totalHalfDays / 2 }} 天 · 申请 #{{ row.id }}</p></div><ElButton :disabled="locked" @click="selected = row">撤销此请假</ElButton></li></ul>
      <PageNav v-if="result" :page="page" :total="result.total" :page-size="result.pageSize" :disabled="locked || loading" label="已批请假分页" @change="selected = null; page = $event" />
      <section v-if="selected" class="oa-inline-form"><h2>{{ selected.applicantName }} · {{ selected.leaveTypeName }}</h2><p class="oa-muted">{{ selected.startDate }} 至 {{ selected.endDate }} · {{ selected.totalHalfDays / 2 }} 天</p><RequestActions :key="selected.id" :request-id="selected.id" :operations="['CANCEL']" @locked="locked = $event" @changed="changed" @invalidated="invalidated" /></section>
    </section>
  </section>
</template>
