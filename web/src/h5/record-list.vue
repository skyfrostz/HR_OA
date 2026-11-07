<script setup>
import { computed, onMounted, shallowRef, useTemplateRef, watch } from 'vue'
import { Button, Tag } from 'vant'
import { listPage } from '../oa/api'
import { useSession } from '../oa/session'
import { useRemote } from '../oa/use-remote'
import { actionLabels, displayDate, statusLabels, statusTone, typeLabels } from './approval'
import PageState from './page-state.vue'
import Pager from './pager.vue'

const props = defineProps({ view: { type: String, required: true } })
const emit = defineEmits(['open'])
const session = useSession()
const page = shallowRef(1)
const heading = useTemplateRef('heading')
const views = {
  mine: { heading: '我的申请', path: '/requests', empty: '还没有提交过申请。' },
  pending: { heading: '待办', path: '/tasks', empty: '当前没有待审批任务。' },
  history: { heading: '已办', path: '/tasks/history', empty: '还没有审批操作。' },
  notifications: { heading: '站内通知', path: '/notifications', empty: '暂无站内模拟通知。' }
}

const view = computed(() => views[props.view])
const { result, loading, error, load } = useRemote((signal) =>
  session.authorized(listPage(view.value.path, page.value), { signal })
)
const records = computed(() => (result.value?.list || []).map((item) => {
  if (props.view === 'notifications') {
    return { id: item.id, requestId: item.requestId, title: item.title, badge: '模拟', tone: 'primary', date: item.createdAt }
  }
  const isMine = props.view === 'mine'
  const isHistory = props.view === 'history'
  return {
    id: item.id,
    requestId: isMine ? item.id : item.requestId,
    title: isMine ? item.title : item.requestTitle,
    category: typeLabels[isMine ? item.type : item.requestType],
    meta: isMine ? '' : isHistory ? item.targetName : item.applicantName + ' · ' + item.stepName,
    badge: isHistory ? actionLabels[item.action] : statusLabels[item.status],
    tone: isHistory ? 'primary' : statusTone(item.status),
    date: item.createdAt
  }
}))

watch(page, load, { immediate: true })
onMounted(() => heading.value.focus())
</script>

<template>
  <section aria-labelledby="h5-list-heading">
    <div class="h5-heading">
      <h1 id="h5-list-heading" ref="heading" tabindex="-1">{{ view.heading }}</h1>
      <p v-if="view.path === '/notifications'" class="h5-muted">仅接收模拟站内通知。</p>
    </div>
    <PageState
      :loading="loading"
      :error="error"
      :empty="result && !records.length ? view.empty : ''"
      @retry="load"
    />
    <ul v-if="!loading && records.length" class="h5-list">
      <li v-for="record in records" :key="record.id">
        <button
          type="button"
          class="h5-card h5-record-button"
          @click="emit('open', record.requestId)"
        >
          <span class="h5-record-head">
            <span class="h5-record-title">{{ record.title }}</span>
            <Tag v-if="record.badge" :type="record.tone" plain>{{ record.badge }}</Tag>
          </span>
          <span class="h5-meta">
            <span v-if="record.category">{{ record.category }}</span>
            <span v-if="record.meta">{{ record.meta }}</span>
            <time :datetime="record.date">{{ displayDate(record.date) }}</time>
          </span>
        </button>
      </li>
    </ul>
    <Pager
      v-if="result"
      :page="page"
      :total="result.total"
      :page-size="result.pageSize"
      :disabled="loading"
      @change="page = $event"
    />
    <Button v-else-if="page > 1 && !loading" class="h5-block" size="small" plain @click="page = 1">返回第一页</Button>
  </section>
</template>
