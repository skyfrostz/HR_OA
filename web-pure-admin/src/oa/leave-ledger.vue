<script setup>
import { computed, shallowRef, watch } from 'vue'
import { ElButton as Button } from 'element-plus'
import { useRouter } from 'vue-router'
import { employeeNavigation } from './navigation'
import { useSession } from './session'
import { useRemote } from './use-remote'
import Pager from './page-nav.vue'
import PageState from './remote-state.vue'
const props = defineProps({ employeeId: { type: Number, required: true }, unavailable: Boolean })
const router = useRouter()
async function open(id) { const nav = employeeNavigation(); nav.requestId = id; nav.approvalView = 'mine'; if (await router.push({ name: 'OaApprovals' })) { nav.requestId = null; nav.approvalView = null } }
const session = useSession()
const page = shallowRef(1)
const year = shallowRef('')
const draftYear = shallowRef('')
const validation = shallowRef('')
function applyYear() {
  const value = String(draftYear.value).trim()
  if (value && (!/^\d{4}$/.test(value) || Number(value) < 2000 || Number(value) > 2100)) { validation.value = '请输入 2000–2100 的年份，或留空查看全部。'; return }
  validation.value = ''; page.value = 1; year.value = value
}
const labels = { RESERVE: '预占', CONSUME: '批准扣减', RELEASE: '释放预占', REVERSE: '撤销冲回' }
const { result, loading, error, load } = useRemote(signal => session.authorized('/employees/' + props.employeeId + '/leave-ledger?page=' + page.value + '&pageSize=20' + (year.value ? '&year=' + year.value : ''), { signal }))
const canOpen = computed(() => ['EMPLOYEE', 'APPROVER'].includes(session.account?.user.role) && session.account?.user.employeeId === props.employeeId)
watch([page, year, () => props.employeeId], load, { immediate: true })
const timestamp = value => new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
</script>
<template>
  <section class="oa-ledger" aria-labelledby="oa-ledger-heading">
    <div class="oa-ledger-head">
      <div><h2 id="oa-ledger-heading">余额流水</h2><p class="oa-muted">保留每次预占、扣减和冲回；旧申请不补记流水。</p></div>
      <Button :disabled="unavailable || loading" @click="load">刷新流水</Button>
    </div>
    <form class="oa-ledger-filter" @submit.prevent="applyYear"><label>年份（选填）<input v-model="draftYear" type="number" min="2000" max="2100" :disabled="unavailable"></label><Button native-type="submit" :disabled="unavailable || loading">查询流水</Button><Button :disabled="unavailable || loading" @click="draftYear = ''; applyYear()">全部年份</Button></form><p v-if="validation" role="alert">{{ validation }}</p>
    <PageState :loading="loading" :error="error" @retry="load" />
    <p v-if="result && !loading && !result.list.length" class="oa-muted">所选年份暂无审批余额流水。</p>
    <ol v-if="!loading && result?.list.length" class="oa-ledger-list">
      <li v-for="entry in result.list" :key="entry.id">
        <div class="oa-ledger-head"><strong>{{ labels[entry.kind] || entry.kind }}</strong><span>{{ entry.halfDays / 2 }} 天 · {{ entry.year }} 年</span></div>
        <time class="oa-muted" :datetime="entry.createdAt">{{ timestamp(entry.createdAt) }}</time>
        <Button v-if="canOpen" :disabled="unavailable" size="small" @click="open(entry.requestId)">查看关联申请 #{{ entry.requestId }}</Button>
        <p v-else class="oa-muted">关联申请 #{{ entry.requestId }}</p>
      </li>
    </ol>
    <Pager v-if="result && !error" :page="page" :page-size="result.pageSize" :total="result.total" :disabled="loading || unavailable" label="余额流水分页" @change="page = $event" />
  </section>
</template>
<style scoped>
.oa-ledger { margin-top: 1.25rem; min-width: 0; }
.oa-ledger-head { display: flex; flex-wrap: wrap; gap: .5rem; justify-content: space-between; align-items: center; }
.oa-ledger-list { list-style: none; padding: 0; margin: 1rem 0; }
.oa-ledger-list li { display: grid; justify-items: start; gap: .5rem; padding: .8rem 0; border-top: 1px solid var(--el-border-color); overflow-wrap: anywhere; }
.oa-ledger-list li > div { width: 100%; }
.oa-ledger-filter { display: flex; align-items: center; flex-wrap: wrap; gap: .5rem; margin-top: .75rem; }
.oa-ledger-filter input { width: 7rem; min-height: 2.75rem; color: inherit; background: var(--el-bg-color); border: 1px solid var(--el-border-color); border-radius: .375rem; padding: .4rem; }
</style>
