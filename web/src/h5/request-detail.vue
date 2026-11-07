<script setup>
import { computed, onMounted, shallowRef, useTemplateRef, watch } from 'vue'
import { Button, Tag } from 'vant'
import { ArrowLeft } from '@element-plus/icons-vue'
import { formatSalary } from '../oa/format'
import { useSession } from '../oa/session'
import { useRemote } from '../oa/use-remote'
import { actionLabels, displayDate, formFields, statusLabels, statusTone, typeLabels } from './approval'
import PageState from './page-state.vue'
import TaskActions from './task-actions.vue'

const props = defineProps({ requestId: { type: Number, required: true } })
const emit = defineEmits(['back', 'open', 'resubmit', 'locked'])
const session = useSession()
const heading = useTemplateRef('heading')
const locked = shallowRef(false)
const feedback = shallowRef(null)
const { result: request, loading, error, load } = useRemote((signal) =>
  session.authorized('/requests/' + props.requestId, { signal })
)

const details = computed(() => (formFields[request.value?.type] || []).map((field) => ({
  label: field.label,
  value: field.input === 'decimal'
    ? formatSalary(request.value.form.amountCents)
    : request.value.form[field.name]
})))
const canResubmit = computed(() =>
  session.account?.permissions.canSubmitRequests &&
  request.value?.status === 'REJECTED' &&
  request.value.applicantId === session.account.user.id
)
const activeTask = computed(() => {
  if (!session.account?.permissions.canApproveRequests || request.value?.status !== 'IN_REVIEW') return null
  return request.value.tasks.find((task) => task.status === 'PENDING' && task.assigneeId === session.account.user.id)
})

function setLocked(value) {
  locked.value = value
  emit('locked', value)
}

async function refreshAfterAction(action) {
  feedback.value = { message: '已记录「' + actionLabels[action] + '」。', error: false }
  await load()
}

async function refreshAfterConflict(message) {
  feedback.value = { message, error: true }
  await load()
}

watch(() => props.requestId, load, { immediate: true })
onMounted(() => heading.value.focus())
</script>

<template>
  <section aria-labelledby="h5-detail-heading">
    <Button class="h5-back" size="small" plain :disabled="locked" @click="emit('back')"><ArrowLeft class="h5-back-arrow" aria-hidden="true" /> 返回列表</Button>
    <div class="h5-heading">
      <h1 id="h5-detail-heading" ref="heading" tabindex="-1">申请详情</h1>
    </div>

    <p
      v-if="feedback"
      class="h5-message"
      :class="{ 'h5-message-error': feedback.error }"
      :role="feedback.error ? 'alert' : 'status'"
    >{{ feedback.message }}</p>
    <PageState :loading="loading" :error="error" @retry="load" />

    <template v-if="!loading && request">
      <section class="h5-card h5-block h5-summary" aria-labelledby="request-summary">
        <p class="h5-kicker">{{ typeLabels[request.type] }}</p>
        <div class="h5-record-head">
          <h2 id="request-summary">{{ request.title }}</h2>
          <Tag :type="statusTone(request.status)" plain>{{ statusLabels[request.status] }}</Tag>
        </div>
        <p class="h5-muted">申请人 {{ request.applicantName }}</p>
        <p class="h5-meta">提交时间 <time :datetime="request.createdAt">{{ displayDate(request.createdAt) }}</time></p>
        <dl class="h5-details">
          <div v-for="item in details" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>{{ item.value }}</dd>
          </div>
        </dl>
        <div v-if="request.previousRequestId || canResubmit" class="h5-actions h5-block">
          <Button
            v-if="request.previousRequestId"
            size="small"
            plain
            :disabled="locked"
            @click="emit('open', request.previousRequestId)"
          >查看原申请</Button>
          <Button v-if="canResubmit" type="primary" @click="emit('resubmit', request)">重新提交</Button>
        </div>
      </section>

      <TaskActions
        v-if="activeTask"
        :key="activeTask.id + '-' + activeTask.version"
        :task="activeTask"
        @locked="setLocked"
        @changed="refreshAfterAction"
        @invalidated="refreshAfterConflict"
      />

      <section class="h5-card h5-block" aria-labelledby="request-route">
        <h2 id="request-route">审批路径</h2>
        <p class="h5-muted">提交时保存的审批路径</p>
        <ol class="h5-timeline">
          <li v-for="step in request.route" :key="step.step" :class="{ 'is-current': request.currentStep === step.step }">
            <p><strong>{{ step.step }}. {{ step.name }}</strong> · {{ step.assigneeName }}</p>
            <p v-if="request.currentStep === step.step" class="h5-meta">当前步骤</p>
          </li>
        </ol>
        <ul class="h5-history" aria-label="实际任务">
          <li v-for="task in request.tasks" :key="task.id">
            <p>{{ task.stepName }} · {{ task.assigneeName }} · {{ statusLabels[task.status] }}</p>
          </li>
        </ul>
      </section>

      <section class="h5-card h5-block" aria-labelledby="request-history">
        <h2 id="request-history">操作记录</h2>
        <p v-if="!request.history.length" class="h5-muted">暂无操作记录。</p>
        <ol v-else class="h5-timeline">
          <li v-for="entry in request.history" :key="entry.id">
            <p><strong>{{ entry.actorName }} · {{ actionLabels[entry.action] }}</strong></p>
            <p v-if="entry.targetName">转交给 {{ entry.targetName }}</p>
            <p v-if="entry.comment" class="h5-comment">{{ entry.comment }}</p>
            <time class="h5-time" :datetime="entry.createdAt">{{ displayDate(entry.createdAt) }}</time>
          </li>
        </ol>
      </section>
    </template>
  </section>
</template>
