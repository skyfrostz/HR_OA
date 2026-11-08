<script setup>
  import { computed, watch } from 'vue'
  import { Button, Tag } from 'vant'
  import { useSession } from '../oa/session'
  import { useRemote } from '../oa/use-remote'
  import { displayDate, statusLabels, statusTone } from './approval'
  import PageState from './page-state.vue'

  const props = defineProps({
    kind: { type: String, required: true },
    revision: Number
  })
  const emit = defineEmits(['open', 'view'])
  const session = useSession()
  const modes = {
    pending: {
      title: '待处理申请',
      path: '/tasks?page=1&pageSize=5',
      empty: '当前没有需要你处理的申请。'
    },
    mine: {
      title: '最近的申请',
      path: '/requests?page=1&pageSize=5',
      empty: '还没有申请，可从“发起申请”开始。'
    },
    contracts: {
      title: '临近到期合同',
      path: '/contract-reminders?page=1&pageSize=5&days=30',
      empty: '逾期及未来 30 天内没有待提醒的合同。'
    }
  }
  const mode = computed(() => modes[props.kind])
  const { result, loading, error, load } = useRemote((signal) =>
    session.authorized(mode.value.path, { signal })
  )
  watch(() => [props.kind, props.revision], load, { immediate: true })
</script>

<template>
  <section class="h5-dashboard-section" :aria-label="mode.title">
    <div class="h5-list-heading">
      <h2>{{ mode.title }}</h2>
      <Button
        v-if="kind !== 'contracts'"
        size="small"
        plain
        @click="emit('view', kind)"
        >查看全部</Button
      >
    </div>
    <PageState
      :loading="loading"
      :error="error"
      :empty="result && !result.list.length ? mode.empty : ''"
      @retry="load"
    />
    <ul v-if="!loading && result?.list.length" class="h5-list">
      <li v-for="row in result.list" :key="row.contractId || row.id">
        <article v-if="kind === 'contracts'" class="h5-card">
          <strong>{{ row.employeeName }} · {{ row.contractNo }}</strong>
          <p class="h5-meta">
            到期 {{ row.endDate }} ·
            {{
              row.daysRemaining < 0
                ? '已逾期 ' + Math.abs(row.daysRemaining) + ' 天'
                : '剩余 ' + row.daysRemaining + ' 天'
            }}
          </p>
          <a class="h5-dashboard-link" href="./oa.html#/contract-reminders"
            >前往管理端查看合同</a
          >
        </article>
        <button
          v-else
          type="button"
          class="h5-card h5-record-button"
          @click="emit('open', kind === 'mine' ? row.id : row.requestId)"
        >
          <span class="h5-record-head"
            ><strong class="h5-record-title">{{
              kind === 'mine' ? row.title : row.requestTitle
            }}</strong
            ><Tag :type="statusTone(row.status)" plain>{{
              statusLabels[row.status]
            }}</Tag></span
          >
          <span v-if="kind === 'pending'" class="h5-meta"
            >{{ row.applicantName }} · {{ row.stepName }}</span
          >
          <time class="h5-time" :datetime="row.createdAt">{{
            displayDate(row.createdAt)
          }}</time>
        </button>
      </li>
    </ul>
  </section>
</template>
