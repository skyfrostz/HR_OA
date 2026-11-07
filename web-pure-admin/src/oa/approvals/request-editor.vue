<script setup>
import { computed, onMounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { ElButton as Button } from 'element-plus'
import { ArrowRight, Calendar, Document, Wallet } from '@element-plus/icons-vue'
import { useSession } from '../session'
import { useRemote } from '../use-remote'
import { approvalError, formFields, initialDraft, requestPayload, typeLabels } from './approval'
import { useIntent } from './use-intent'
import PageState from '../remote-state.vue'

const props = defineProps({ previous: { type: Object, default: null } })
const emit = defineEmits(['locked', 'saved', 'back'])
const session = useSession()
const selectedType = shallowRef(props.previous?.type || '')
const draft = ref(initialDraft(selectedType.value, props.previous))
const validationError = shallowRef('')
const heading = useTemplateRef('heading')
const templates = useRemote((signal) => session.authorized('/request-templates', { signal }))
const { result, loading, error: loadError, load } = templates
const available = computed(() => (result.value?.list || []).filter((item) =>
  Object.hasOwn(formFields, item.type)
))
const selected = computed(() => available.value.find((item) => item.type === selectedType.value))
const templateIcons = { LEAVE: Calendar, EXPENSE: Wallet, GENERAL: Document }
const fields = computed(() => formFields[selectedType.value] || [])
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
  const detail = await mutation.submit('/requests', data)
  if (detail) emit('saved', detail.id)
}

async function retry() {
  const detail = await mutation.retry()
  if (detail) emit('saved', detail.id)
}
</script>

<template>
  <section aria-labelledby="pc-editor-title">
    <Button class="pc-back" size="small" plain :disabled="locked" @click="emit('back')">{{ previous ? '返回原申请' : '返回列表' }}</Button>
    <div class="pc-heading pc-editor-intro">
      <p class="pc-kicker">申请工作台</p>
      <h1 id="pc-editor-title" ref="heading" tabindex="-1">{{ previous ? '重新提交申请' : '发起申请' }}</h1>
      <p class="pc-muted">{{ previous ? '修改后会生成一条新的申请记录。' : '选择申请类型，提交前可查看审批路径。' }}</p>
    </div>

    <PageState
      :loading="loading"
      :error="loadError"
      :empty="Boolean(result && !available.length)" empty-text="暂无可用的申请表单。"
      @retry="load"
    />
    <ul v-if="available.length && !previous" class="pc-list pc-template-list" aria-label="申请类型">
      <li v-for="template in available" :key="template.type">
        <button
          type="button"
          class="pc-card pc-record-button pc-template-button"
          :aria-pressed="selectedType === template.type"
          :disabled="locked"
          @click="selectType(template.type)"
        >
          <span class="pc-template-icon" :data-type="template.type" aria-hidden="true">
            <component :is="templateIcons[template.type]" />
          </span>
          <span>
            <strong>{{ typeLabels[template.type] }}</strong>
            <span class="pc-template-description">{{ template.description }}</span>
          </span>
          <ArrowRight class="pc-template-arrow" aria-hidden="true" />
        </button>
      </li>
    </ul>

    <div v-if="selected" class="pc-card pc-block pc-editor-card">
      <h2>{{ typeLabels[selected.type] }}</h2>
      <p v-if="selected.route.length" class="pc-route-preview">
        {{ selected.route.map((step) => step.name).join(' → ') }}
      </p>
      <form class="oa-form pc-form" autocomplete="off" @submit.prevent="submit">
        <label for="pc-request-title">
          标题
          <input id="pc-request-title" v-model="draft.title" required :disabled="locked">
        </label>
        <label v-for="field in fields" :key="field.name" :for="'pc-field-' + field.name">
          {{ field.label }}
          <textarea
            v-if="field.input === 'textarea'"
            :id="'pc-field-' + field.name"
            v-model="draft.form[field.name]"
            required
            :disabled="locked"
          />
          <input
            v-else
            :id="'pc-field-' + field.name"
            v-model="draft.form[field.name]"
            :type="field.input === 'date' ? 'date' : 'text'"
            :inputmode="field.input === 'decimal' ? 'decimal' : undefined"
            required
            :disabled="locked"
          >
        </label>
        <p v-if="validationError || error" class="pc-message pc-message-error" role="alert">
          {{ validationError || error }}
        </p>
        <Button
          v-if="retryable"
          type="primary"
          :disabled="busy"
          :loading="busy"
          @click="retry"
        >重试原提交</Button>
        <Button v-else type="primary" native-type="submit" :disabled="busy" :loading="busy">提交申请</Button>
      </form>
    </div>
  </section>
</template>
