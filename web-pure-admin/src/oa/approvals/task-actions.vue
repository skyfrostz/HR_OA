<script setup>
import { computed, nextTick, shallowRef, useTemplateRef, watch } from 'vue'
import { ElButton as Button } from 'element-plus'
import { useSession } from '../session'
import { useRemote } from '../use-remote'
import { actionLabels, approvalError } from './approval'
import { useIntent } from './use-intent'
import PageState from '../remote-state.vue'

const props = defineProps({ task: { type: Object, required: true } })
const emit = defineEmits(['locked', 'changed', 'invalidated'])
const session = useSession()
const action = shallowRef('')
const actionForm = useTemplateRef('actionForm')
const comment = shallowRef('')
const targetId = shallowRef(null)
const validationError = shallowRef('')
const path = computed(() => '/tasks/' + props.task.id)
const { result: candidates, loading, error: candidateError, load } = useRemote((signal) =>
  session.authorized(path.value + '/transfer-targets', { signal })
)
const mutation = useIntent(approvalError)
const { busy, error, retryable, failure } = mutation
const locked = computed(() => busy.value || retryable.value)

watch(locked, (value) => emit('locked', value), { flush: 'sync' })

function begin(value) {
  if (locked.value) return
  action.value = value
  comment.value = ''
  targetId.value = null
  validationError.value = ''
  if (value === 'TRANSFER') void load()
  if (value) void nextTick(() => actionForm.value?.focus())
}

function finish(detail) {
  if (detail) {
    emit('changed', action.value)
  } else if (failure.value?.status === 403 || ['STALE_TASK', 'IDEMPOTENCY_KEY_REUSED'].includes(failure.value?.code)) {
    emit('invalidated', error.value)
  }
}

async function submit() {
  if (locked.value || !action.value) return
  validationError.value = ''
  const note = comment.value.trim()
  if ([...note].length > 1000 || (!note && action.value !== 'APPROVE')) {
    validationError.value = '请填写处理意见，最多 1000 字。'
    return
  }
  const data = { action: action.value, version: props.task.version, comment: note }
  if (action.value === 'TRANSFER') {
    if (!candidates.value?.list.some(({ id }) => id === targetId.value)) {
      validationError.value = '请选择转交对象。'
      return
    }
    data.targetUserId = targetId.value
  }
  finish(await mutation.submit(path.value + '/actions', data))
}

async function retry() {
  finish(await mutation.retry())
}
</script>

<template>
  <section class="pc-card pc-block" aria-labelledby="task-actions-title">
    <h2 id="task-actions-title">处理待办</h2>
    <p class="pc-muted">{{ task.stepName }} · 当前由你处理</p>
    <div v-if="!action" class="pc-task-dock" role="group" aria-label="当前任务操作">
      <Button type="primary" @click="begin('APPROVE')">通过</Button>
      <Button type="danger" plain @click="begin('REJECT')">驳回</Button>
      <Button type="primary" plain @click="begin('TRANSFER')">转交</Button>
    </div>

    <form v-else ref="actionForm" class="oa-form pc-form pc-task-form" tabindex="-1" autocomplete="off" @submit.prevent="submit">
      <h3>确认{{ actionLabels[action] }}</h3>
      <template v-if="action === 'TRANSFER'">
        <PageState
          :loading="loading"
          :error="candidateError"
          :empty="Boolean(candidates && !candidates.list.length)" empty-text="当前没有可转交的审批人。"
          @retry="load"
        />
        <label v-if="!loading && candidates?.list.length" for="pc-transfer-target">
          转交给
          <select id="pc-transfer-target" v-model="targetId" required :disabled="locked">
            <option :value="null" disabled>请选择审批人</option>
            <option v-for="candidate in candidates.list" :key="candidate.id" :value="candidate.id">
              {{ candidate.displayName }}
            </option>
          </select>
        </label>
      </template>
      <label for="pc-action-comment">
        处理意见{{ action === 'APPROVE' ? '（选填）' : '（必填）' }}
        <textarea id="pc-action-comment" v-model="comment" :required="action !== 'APPROVE'" :disabled="locked" />
      </label>
      <p v-if="validationError || error" class="pc-message pc-message-error" role="alert">
        {{ validationError || error }}
      </p>
      <div class="pc-task-dock pc-task-confirm" role="group" aria-label="确认当前操作">
        <Button
          v-if="retryable"
          type="primary"
          :disabled="busy"
          :loading="busy"
          @click="retry"
        >重试原操作</Button>
        <Button
          v-else
          type="primary"
          native-type="submit"
          :disabled="busy || (action === 'TRANSFER' && (loading || !candidates?.list.length))"
          :loading="busy"
        >确认{{ actionLabels[action] }}</Button>
        <Button plain :disabled="locked" @click="begin('')">取消</Button>
      </div>
    </form>
  </section>
</template>
