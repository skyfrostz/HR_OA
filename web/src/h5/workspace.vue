<script setup>
  import { computed, shallowRef, watch } from 'vue'
  import { Tabbar, TabbarItem } from 'vant'
  import {
    Bell,
    CircleCheck,
    EditPen,
    List,
    Tickets
  } from '@element-plus/icons-vue'
  import { useSession } from '../oa/session'
  import Dashboard from './dashboard.vue'
  import PersonalLeave from './personal-leave.vue'
  import RecordList from './record-list.vue'
  import RequestDetail from './request-detail.vue'
  import RequestEditor from './request-editor.vue'

  const emit = defineEmits(['locked', 'task-mode'])
  const session = useSession()
  const canSubmit = computed(
    () => session.account?.permissions.canSubmitRequests === true
  )
  const canApprove = computed(
    () => session.account?.permissions.canApproveRequests === true
  )
  const selected = shallowRef(
    canSubmit.value ? 'new' : canApprove.value ? 'pending' : 'notifications'
  )
  const selectedId = shallowRef(null)
  const previous = shallowRef(null)
  const locked = shallowRef(false)
  const formSelected = shallowRef(false)
  const taskMode = computed(
    () =>
      Boolean(previous.value) ||
      (selected.value === 'new' && !selectedId.value && formSelected.value)
  )
  watch(taskMode, (value) => emit('task-mode', value), { immediate: true })
  watch(locked, (value) => emit('locked', value), { flush: 'sync' })

  function switchView(view) {
    if (locked.value) return
    if (
      view === selected.value &&
      selectedId.value === null &&
      previous.value === null
    )
      return
    selected.value = view
    formSelected.value = false
    selectedId.value = null
    previous.value = null
  }

  function showDetail(id) {
    if (locked.value || !Number.isSafeInteger(id) || id <= 0) return
    selectedId.value = id
    previous.value = null
  }

  function requestSaved(id) {
    selected.value = 'mine'
    showDetail(id)
  }
</script>

<template>
  <div class="h5-workspace" :class="{ 'h5-task-mode': taskMode }">
    <main id="h5-content" class="h5-main" tabindex="-1">
      <nav
        v-if="!taskMode && !selectedId"
        class="h5-shortcuts"
        aria-label="工作台与假期"
      >
        <button
          type="button"
          :disabled="locked"
          :aria-pressed="selected === 'dashboard'"
          @click="switchView('dashboard')"
        >
          工作台</button
        ><button
          v-if="session.account?.user.employeeId"
          type="button"
          :disabled="locked"
          :aria-pressed="selected === 'balance'"
          @click="switchView('balance')"
        >
          我的假期
        </button>
      </nav>
      <Dashboard
        v-if="selected === 'dashboard' && !selectedId"
        @view="switchView"
        @open="showDetail"
      />
      <PersonalLeave
        v-else-if="selected === 'balance' && !selectedId"
        @open="showDetail"
      />
      <RecordList
        v-else-if="selected === 'notifications' && !selectedId"
        view="notifications"
        @open="showDetail"
      />
      <template v-else-if="canSubmit || canApprove">
        <RequestEditor
          v-if="previous || (selected === 'new' && !selectedId)"
          :key="previous?.id ?? 'new'"
          :previous="previous"
          @locked="locked = $event"
          @saved="requestSaved"
          @back="previous ? (previous = null) : switchView('mine')"
          @task-mode="formSelected = $event"
        />
        <RequestDetail
          v-else-if="selectedId"
          :key="selectedId"
          :request-id="selectedId"
          @back="selectedId = null"
          @open="showDetail"
          @resubmit="previous = $event"
          @locked="locked = $event"
        />
        <RecordList
          v-else
          :key="selected"
          :view="selected"
          @open="showDetail"
        />
      </template>
      <section v-else class="h5-card">
        <h1>移动审批</h1>
        <p class="h5-muted">当前账号没有申请或审批权限。</p>
      </section>
    </main>
    <footer v-if="!taskMode" class="h5-footer">HR OA · 按账号权限访问</footer>
    <div v-if="!taskMode" class="h5-nav-shell">
      <Tabbar
        class="h5-nav"
        :model-value="selected"
        :fixed="false"
        :safe-area-inset-bottom="false"
        :aria-disabled="locked"
        aria-label="移动审批导航"
      >
        <TabbarItem v-if="canSubmit" name="new" @click="switchView('new')">
          <template #icon><EditPen aria-hidden="true" /></template>
          发起申请
        </TabbarItem>
        <TabbarItem
          v-if="canSubmit || canApprove"
          name="mine"
          @click="switchView('mine')"
        >
          <template #icon><Tickets aria-hidden="true" /></template>
          我的申请
        </TabbarItem>
        <TabbarItem
          v-if="canApprove"
          name="pending"
          @click="switchView('pending')"
        >
          <template #icon><List aria-hidden="true" /></template>
          待办
        </TabbarItem>
        <TabbarItem
          v-if="canApprove"
          name="history"
          @click="switchView('history')"
        >
          <template #icon><CircleCheck aria-hidden="true" /></template>
          已办
        </TabbarItem>
        <TabbarItem name="notifications" @click="switchView('notifications')">
          <template #icon><Bell aria-hidden="true" /></template>
          站内通知
        </TabbarItem>
      </Tabbar>
    </div>
  </div>
</template>
