<script setup>
import { computed, onMounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { Button } from 'vant'
import { ArrowRight, Calendar, Document, Wallet } from '@element-plus/icons-vue'
import { useSession } from '../oa/session'
import { useRemote } from '../oa/use-remote'
import { approvalError, formFields, initialDraft, requestPayload, typeLabels } from './approval'
import { useIntent } from './use-intent'
import LeaveFields from './leave-fields.vue'
import PageState from './page-state.vue'

const props = defineProps({ previous: { type: Object, default: null } })
const emit = defineEmits(['locked', 'saved', 'back'])
const session = useSession()
const selectedType = shallowRef(props.previous?.type || '')
const draft = ref(initialDraft(selectedType.value, props.previous))
const validationError = shallowRef('')
const leavePreview = shallowRef(null)
const previewRevision = shallowRef(0)
const heading = useTemplateRef('heading')
const templates = useRemote((signal) => session.authorized('/request-templates', { signal }))
const { result, loading, error: loadError, load } = templates
const available = computed(() => (result.value?.list || []).filter((item) =>
  Object.hasOwn(formFields, item.type)
))
const selected = computed(() => available.value.find((item) => item.type === selectedType.value))
const templateIcons = { LEAVE: Calendar, EXPENSE: Wallet, GENERAL: Document }
const fields = computed(() => selectedType.value === 'LEAVE' ? [] : formFields[selectedType.value] || [])
const mutation = useIntent(approvalError)
const { busy, retryable, error } = mutation
const locked = computed(() => busy.value || retryable.value)

onMounted(() => {
  heading.value.focus()
  void load()
})
watch(locked, (value) => emit('locked', value), { flush: 'sync' })

function selectType(type) {
  if (locked.value || type === selectedType.value) return
  selectedType.value = type
  draft.value = initialDraft(type)
  validationError.value = ''
}

async function submit() {
  if (locked.value || !selected.value) return
  validationError.value = ''
  let data
  try {
    data = requestPayload(selected.value.type, draft.value, props.previous?.id)
  } catch (cause) {
    validationError.value = cause.message
    return
  }
  if (selectedType.value === 'LEAVE' && !leavePreview.value?.canSubmit) { validationError.value = '请先预览请假天数并处理余额或时段问题。'; return }
  if (selectedType.value === 'LEAVE') Object.assign(data.form, { leaveTypeVersion: leavePreview.value.calculation.leaveTypeVersion, calendarVersion: leavePreview.value.calculation.calendarVersion })
  const detail = await mutation.submit('/requests', data)
  finishSubmission(detail)
}

function acceptPreview(value) {
  leavePreview.value = value
  if (value?.canSubmit) { validationError.value = ''; error.value = '' }
}
function finishSubmission(detail) {
  if (detail) emit('saved', detail.id)
  else if (['STALE_RECORD', 'LEAVE_OVERLAP', 'INSUFFICIENT_LEAVE'].includes(mutation.failure.value?.code)) {
    leavePreview.value = null
    previewRevision.value++
    validationError.value = '配置、余额或可用时段已变化，请重新预览后再提交。'
  }
}
async function retry() {
  validationError.value = ''
  finishSubmission(await mutation.retry())
}
</script>

<template>
  <section aria-labelledby="h5-editor-title">
    <Button v-if="previous" class="h5-back" size="small" plain :disabled="locked" @click="emit('back')">返回原申请</Button>
    <div class="h5-heading h5-editor-intro">
      <p class="h5-kicker">申请工作台</p>
      <h1 id="h5-editor-title" ref="heading" tabindex="-1">{{ previous ? '重新提交申请' : '发起申请' }}</h1>
      <p class="h5-muted">{{ previous ? '修改后会生成一条新的申请记录。' : '选择一种演示表单，按当前配置的规则审批。' }}</p>
    </div>

    <PageState
      :loading="loading"
      :error="loadError"
      :empty="result && !available.length ? '暂无可用的演示表单。' : ''"
      @retry="load"
    />
    <ul v-if="available.length && !previous" class="h5-list h5-template-list" aria-label="申请类型">
      <li v-for="template in available" :key="template.type">
        <button
          type="button"
          class="h5-card h5-record-button h5-template-button"
          :aria-pressed="selectedType === template.type"
          :disabled="locked"
          @click="selectType(template.type)"
        >
          <span class="h5-template-icon" :data-type="template.type" aria-hidden="true">
            <component :is="templateIcons[template.type]" />
          </span>
          <span>
            <strong>{{ typeLabels[template.type] }}</strong>
            <span class="h5-template-description">{{ template.description }}</span>
          </span>
          <ArrowRight class="h5-template-arrow" aria-hidden="true" />
        </button>
      </li>
    </ul>

    <div v-if="selected" class="h5-card h5-block h5-editor-card">
      <h2>{{ typeLabels[selected.type] }}</h2>
      <p v-if="selected.route?.length" class="h5-route-preview">
        审批路径提示：{{ selected.route.map((step) => step.name).join(' → ') }}。金额规则在提交时确定，结果以申请保存的路径为准。
      </p>
      <form class="h5-form" autocomplete="off" @submit.prevent="submit">
        <label for="h5-request-title">
          标题
          <input id="h5-request-title" v-model="draft.title" required :disabled="locked">
        </label>
        <LeaveFields :key="previewRevision" v-if="selectedType === 'LEAVE'" v-model="draft.form" :disabled="locked" @validated="acceptPreview" />
        <label v-for="field in fields" :key="field.name" :for="'h5-field-' + field.name">
          {{ field.label }}
          <textarea
            v-if="field.input === 'textarea'"
            :id="'h5-field-' + field.name"
            v-model="draft.form[field.name]"
            required
            :disabled="locked"
          />
          <input
            v-else
            :id="'h5-field-' + field.name"
            v-model="draft.form[field.name]"
            :type="field.input === 'date' ? 'date' : 'text'"
            :inputmode="field.input === 'decimal' ? 'decimal' : undefined"
            required
            :disabled="locked"
          >
        </label>
        <p v-if="validationError || error" class="h5-message h5-message-error" role="alert">
          {{ validationError || error }}
        </p>
        <Button
          v-if="retryable"
          type="primary"
          block
          :disabled="busy"
          :loading="busy"
          @click="retry"
        >重试原提交</Button>
        <Button v-else type="primary" native-type="submit" block :disabled="busy || (selectedType === 'LEAVE' && !leavePreview?.canSubmit)" :loading="busy">提交申请</Button>
      </form>
    </div>
  </section>
</template>
