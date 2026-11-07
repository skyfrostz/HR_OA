<script setup>
import { computed, shallowRef, watch } from 'vue'
import { ElButton, ElTag } from 'element-plus'
import { listPage } from './api'
import { contractStatuses } from './format'
import { useSession } from './session'
import { useRemote } from './use-remote'
import AttachmentForm from './attachment-form.vue'
import ContractFiles from './contract-files.vue'
import ContractForm from './contract-form.vue'
import PageNav from './page-nav.vue'
import RemoteState from './remote-state.vue'

const props = defineProps({ employeeId: { type: Number, required: true } })
const emit = defineEmits(['editing'])
const session = useSession()
const page = shallowRef(1)
const edit = shallowRef(null)
const feedback = shallowRef('')
const path = computed(() => '/employees/' + props.employeeId + '/contracts')
const { result, loading, error, load } = useRemote((signal) =>
  session.authorized(listPage(path.value, page.value), { signal })
)

watch(page, load, { immediate: true })

function startEdit(kind, contract = null) {
  if (edit.value || !session.canWrite) return
  feedback.value = ''
  edit.value = { kind, contract }
  emit('editing', true)
}

function stopEdit() {
  edit.value = null
  emit('editing', false)
}

async function refreshAttachments() {
  const refreshed = await load()
  if (!refreshed) stopEdit()
  return refreshed
}

async function saved(message) {
  stopEdit()
  feedback.value = message
  await load()
}
</script>

<template>
  <section class="oa-surface" aria-labelledby="contracts-heading">
    <div class="oa-section-head">
      <div>
        <h2 id="contracts-heading">劳动合同</h2>
        <p class="oa-muted">查看合同期限、续约关系和私有附件。</p>
      </div>
      <ElButton
        v-if="session.canWrite"
        type="primary"
        :disabled="Boolean(edit)"
        @click="startEdit('contract')"
      >新建合同</ElButton>
    </div>

    <ContractForm
      v-if="edit?.kind === 'contract'"
      :path="path"
      :previous="edit.contract"
      @cancel="stopEdit"
      @created="saved('合同已保存。')"
    />
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>

    <RemoteState
      :loading="loading"
      :error="error"
      :empty="Boolean(result && !result.list?.length)"
      empty-text="暂无劳动合同。"
      @retry="load"
    />

    <div v-if="result?.list?.length" class="oa-contracts" :aria-busy="loading">
      <article v-for="contract in result.list" :key="contract.id" class="oa-contract">
        <div class="oa-section-head">
          <div>
            <h3>{{ contract.contractNo }}</h3>
            <p class="oa-muted">{{ contract.startDate }} 至 {{ contract.endDate }}</p>
          </div>
          <ElTag :type="contractStatuses[contract.status]?.type" effect="plain">
            {{ contractStatuses[contract.status]?.label || '未知状态' }}
          </ElTag>
        </div>
        <p v-if="contract.previousContractId" class="oa-muted">
          续约来源：{{ result.list.find((item) => item.id === contract.previousContractId)?.contractNo
            || '记录 #' + contract.previousContractId }}
        </p>

        <ContractFiles :files="contract.files || []" />

        <AttachmentForm
          v-if="edit?.kind === 'attachment' && edit.contract.id === contract.id"
          :contract-id="contract.id"
          :refresh="refreshAttachments"
          @cancel="stopEdit"
          @uploaded="saved('附件已上传。')"
        />
        <div v-if="session.canWrite" class="oa-actions">
          <ElButton :disabled="loading || Boolean(edit)" @click="startEdit('contract', contract)">续约</ElButton>
          <ElButton :disabled="loading || Boolean(edit)" @click="startEdit('attachment', contract)">添加附件</ElButton>
        </div>
      </article>
    </div>
    <PageNav
      v-if="result"
      :page="page"
      :total="result.total"
      :page-size="result.pageSize"
      :disabled="loading || Boolean(edit)"
      label="劳动合同分页"
      @change="page = $event"
    />
  </section>
</template>
