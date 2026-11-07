<script setup>
import { computed, reactive, onMounted, onUnmounted, shallowRef, useTemplateRef, watch } from 'vue'
import { ElButton as Button, ElTag as Tag } from 'element-plus'
import { listPage } from '../api'
import { useSession } from '../session'
import { useRemote } from '../use-remote'
import { actionLabels, displayDate, statusLabels, statusTone, typeLabels } from './approval'
import PageState from '../remote-state.vue'
import Pager from '../page-nav.vue'

const props = defineProps({ view: { type: String, required: true }, activeId: Number, revision: Number })
const emit = defineEmits(['open'])
const session = useSession()
const page = shallowRef(1)
const filters = reactive({ type: '', status: '', fromDate: '', toDate: '' })
const applied = shallowRef({})
const filterError = shallowRef('')
const heading = useTemplateRef('heading')
const views = {
  mine: { heading: '我的申请', path: '/requests', empty: '还没有提交过申请。' },
  pending: { heading: '待办', path: '/tasks', empty: '当前没有待审批任务。' },
  history: { heading: '已办', path: '/tasks/history', empty: '还没有审批操作。' },

}

const view = computed(() => views[props.view])
const { result, loading, error, load } = useRemote((signal) =>
  session.authorized(listPage(view.value.path, page.value) + '&' + new URLSearchParams(applied.value), { signal })
)
const records = computed(() => (result.value?.list || []).map((item) => {
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

watch([page, applied, () => props.revision], load, { immediate: true })
watch(result, (data) => {
  if (!data || data.list.length) return
  const lastPage = Math.max(1, Math.ceil(data.total / data.pageSize))
  if (page.value > lastPage) page.value = lastPage
})
function apply() {
  if (filters.fromDate && filters.toDate && filters.fromDate > filters.toDate) {
    filterError.value = '结束日期不能早于开始日期。'
    return
  }
  filterError.value = ''
  page.value = 1
  applied.value = Object.fromEntries(Object.entries(filters).filter(([, value]) => value))
}
function reset() {
  Object.keys(filters).forEach(key => { filters[key] = '' })
  apply()
}
function refreshWhenVisible() {
  if (document.visibilityState === 'visible') void load()
}

onMounted(() => {
  heading.value.focus()
  document.addEventListener('visibilitychange', refreshWhenVisible)
})
onUnmounted(() => document.removeEventListener('visibilitychange', refreshWhenVisible))
</script>

<template>
  <section aria-labelledby="pc-list-heading">
    <div class="pc-heading pc-list-heading">
      <div>
        <h2 id="pc-list-heading" ref="heading" tabindex="-1">{{ view.heading }}</h2>
      </div>
      <Button size="small" plain type="primary" @click="load">刷新</Button>
    </div>
    <form class="oa-form pc-filters" @submit.prevent="apply">
      <div class="oa-fields">
        <label>申请类型<select v-model="filters.type"><option value="">全部类型</option><option v-for="(label, value) in typeLabels" :key="value" :value="value">{{ label }}</option></select></label>
        <label>申请状态<select v-model="filters.status"><option value="">全部状态</option><option v-for="value in ['IN_REVIEW', 'APPROVED', 'REJECTED']" :key="value" :value="value">{{ statusLabels[value] }}</option></select></label>
        <label>开始日期<input v-model="filters.fromDate" type="date"></label>
        <label>结束日期<input v-model="filters.toDate" type="date"></label>
      </div>
      <p v-if="filterError" role="alert">{{ filterError }}</p>
      <div class="oa-actions"><Button type="primary" native-type="submit">筛选</Button><Button @click="reset">清空</Button></div>
    </form>
    <PageState
      :loading="loading"
      :error="error"
      :empty="Boolean(result && !records.length)" :empty-text="Object.keys(applied).length ? '没有符合当前筛选的记录。' : view.empty"
      @retry="load"
    />
    <ul v-if="!loading && records.length" class="pc-list">
      <li v-for="record in records" :key="record.id">
        <button
          type="button"
          class="pc-card pc-record-button"
           :aria-current="record.requestId === activeId ? 'true' : undefined"
          @click="emit('open', record.requestId, $event.currentTarget)"
        >
          <span class="pc-record-head">
            <span class="pc-record-title">{{ record.title }}</span>
            <Tag v-if="record.badge" :type="record.tone" effect="plain">{{ record.badge }}</Tag>
          </span>
          <span class="pc-meta">
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
      label="审批记录分页"
      @change="page = $event"
    />
    <Button v-else-if="page > 1 && !loading" class="pc-block" size="small" plain @click="page = 1">返回第一页</Button>
  </section>
</template>
