<script setup>
  import { computed, shallowRef, onMounted, useTemplateRef } from 'vue'
  import { Button } from 'vant'
  import { useSession } from '../oa/session'
  import { useRemote } from '../oa/use-remote'
  import PageState from './page-state.vue'
  import DashboardRecords from './dashboard-records.vue'
  const emit = defineEmits(['view', 'open'])
  const session = useSession()
  const revision = shallowRef(0)
  const heading = useTemplateRef('heading')
  onMounted(() => heading.value?.focus())
  const { result, loading, error, load } = useRemote((signal) =>
    session.authorized('/dashboard?days=30', { signal })
  )
  const entries = computed(() =>
    result.value
      ? [
          ...(session.account?.permissions.canApproveRequests
            ? [{ key: 'pendingTasks', label: '待我审批', view: 'pending' }]
            : []),
          ...(session.account?.permissions.canSubmitRequests
            ? [{ key: 'myRequests', label: '我的申请', view: 'mine' }]
            : [])
        ].filter((item) => typeof result.value[item.key] === 'number')
      : []
  )
  function refresh() {
    revision.value++
    void load()
  }
  void load()
</script>
<template>
  <section aria-labelledby="h5-dashboard-heading">
    <header class="h5-list-heading">
      <h1 id="h5-dashboard-heading" ref="heading" tabindex="-1">工作台</h1>
      <Button :disabled="loading" @click="refresh">刷新工作台</Button>
    </header>
    <p class="h5-muted">
      {{ session.account?.user.displayName }}，查看当前事项与假期余额。
    </p>
    <PageState :loading="loading" :error="error" @retry="load" />
    <ul v-if="!loading && result" class="h5-list h5-dashboard-list">
      <li v-for="entry in entries" :key="entry.key">
        <button
          class="h5-card h5-record-button h5-dashboard-entry"
          type="button"
          @click="emit('view', entry.view)"
        >
          <span
            ><strong>{{ entry.label }}</strong
            ><span class="h5-template-description">查看列表</span></span
          >
          <span class="h5-dashboard-number">{{ result[entry.key] }}</span>
        </button>
      </li>
      <li v-if="typeof result.contractReminders === 'number'">
        <a
          class="h5-card h5-record-button h5-dashboard-entry h5-summary-contract"
          href="./oa.html#/contract-reminders"
        >
          <span
            ><strong>合同到期</strong
            ><span class="h5-template-description">逾期及未来 30 天</span></span
          ><span class="h5-dashboard-number">{{
            result.contractReminders
          }}</span>
        </a>
      </li>
    </ul>
    <DashboardRecords
      v-if="session.account?.permissions.canApproveRequests"
      kind="pending"
      :revision="revision"
      @view="emit('view', $event)"
      @open="emit('open', $event)"
    />
    <DashboardRecords
      v-else-if="session.account?.permissions.canSubmitRequests"
      kind="mine"
      :revision="revision"
      @view="emit('view', $event)"
      @open="emit('open', $event)"
    />
    <DashboardRecords
      v-if="typeof result?.contractReminders === 'number'"
      kind="contracts"
      :revision="revision"
    />
    <Button
      v-if="session.account?.user.employeeId"
      class="h5-block"
      @click="emit('view', 'balance')"
      >我的假期与流水</Button
    >
  </section>
</template>
