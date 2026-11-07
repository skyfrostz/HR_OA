<script setup>
import { computed, nextTick, shallowRef, watch } from 'vue'
import { ElButton } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { useSession } from '../session'
import { useRouteWriteLock } from '../use-route-write-lock'
import { employeeNavigation } from '../navigation'
import RecordList from './record-list.vue'
import RequestDetail from './request-detail.vue'
import RequestEditor from './request-editor.vue'
import './workspace.css'

const session = useSession()
const navigation = employeeNavigation()
const canSubmit = computed(() => session.account?.permissions.canSubmitRequests === true)
const canApprove = computed(() => session.account?.permissions.canApproveRequests === true)
const views = computed(() => [
  ...(canSubmit.value ? [{ id: 'mine', label: '我的申请' }] : []),
  ...(canApprove.value ? [{ id: 'pending', label: '待办' }, { id: 'history', label: '已办' }] : [])
])
const selected = shallowRef(canApprove.value ? 'pending' : 'mine')
const selectedId = shallowRef(null)
const previous = shallowRef(null)
const composing = shallowRef(false)
const locked = shallowRef(false)
const submitted = shallowRef(false)
const listRevision = shallowRef(0)
let returnFocus
useRouteWriteLock(locked)
watch(() => [navigation.requestId, navigation.approvalView], ([id, view]) => {
  if (locked.value) return
  if (view && views.value.some(item => item.id === view)) selected.value = view
  if (id) selectedId.value = id
  navigation.requestId = null
  navigation.approvalView = null
}, { immediate: true })

function switchView(view) {
  if (locked.value) return
  selected.value = view
  closeDetail()
}
function closeDetail(restore = false) {
  if (locked.value) return
  selectedId.value = null
  previous.value = null
  composing.value = false
  submitted.value = false
  if (restore) void nextTick(() => returnFocus?.isConnected && returnFocus.focus())
}
function showDetail(id, trigger) {
  if (locked.value) return
  closeDetail()
  if (trigger) returnFocus = trigger
  selectedId.value = id
}
function refreshList() {
  listRevision.value++
}
function requestSaved(id) {
  selected.value = 'mine'
  showDetail(id)
  submitted.value = true
  refreshList()
}
</script>

<template>
  <section class="oa-pure pc-approvals" aria-labelledby="approval-center">
    <header class="oa-section-head oa-page-heading">
      <div><h1 id="approval-center">审批中心</h1><p class="oa-muted">跟进申请进度，查看每一步的处理记录。</p></div>
      <ElButton v-if="canSubmit" type="primary" :icon="Plus" :disabled="locked || composing"
        @click="returnFocus = $event.currentTarget; closeDetail(); composing = true">发起申请</ElButton>
    </header>
    <nav v-if="views.length" class="pc-views oa-segmented" aria-label="审批视图">
      <button v-for="view in views" :key="view.id" type="button" :aria-pressed="selected === view.id"
        :disabled="locked" @click="switchView(view.id)">{{ view.label }}</button>
    </nav>
    <p v-else class="oa-status">当前账号没有申请或审批权限。</p>
    <div v-if="views.length" class="pc-layout" :class="{ 'pc-with-detail': selectedId || composing || previous }">
      <div class="oa-surface pc-list-pane" :inert="locked || undefined">
        <RecordList :key="selected" :view="selected" :active-id="selectedId" :revision="listRevision" @open="showDetail" />
      </div>
      <div v-if="selectedId || composing || previous" class="oa-surface pc-detail-pane">
        <p v-if="submitted" class="oa-status" role="status">申请已提交。</p>
        <RequestEditor v-if="composing || previous" :key="previous?.id || 'new'" :previous="previous"
          @locked="locked = $event" @saved="requestSaved" @back="previous ? previous = null : closeDetail(true)" />
        <RequestDetail v-else :key="selectedId" :request-id="selectedId"
          @back="closeDetail(true)" @open="showDetail" @resubmit="previous = $event" @locked="locked = $event"
          @refresh-list="refreshList" />
      </div>
    </div>
  </section>
</template>
