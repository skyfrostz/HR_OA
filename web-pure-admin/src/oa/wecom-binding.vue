<script setup>
import { computed, shallowRef, watch } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { request } from './api'
import { useSession } from './session'
import { useRemote } from './use-remote'
import { useVersionedWrite } from './use-versioned-write'
import RemoteState from './remote-state.vue'
const props = defineProps({ employeeId: { type: Number, required: true }, unavailable: Boolean })
const emit = defineEmits(['editing', 'changed'])
const session = useSession()
const editing = shallowRef(false)
const memberId = shallowRef('')
const reason = shallowRef('')
const operation = shallowRef('bind')
const feedback = shallowRef('')
const path = '/employees/' + props.employeeId + '/wecom-binding'
const { result, loading, error, load } = useRemote(signal => session.authorized(path, { signal }))
const status = useRemote(signal => request('/integration/status', { signal }))
const { busy, blocked, error: writeError, send, reconcile } = useVersionedWrite()
const enabled = computed(() => ['mock', 'configured'].includes(status.result.value?.wecomLogin))
watch(editing, value => emit('editing', value), { flush: 'sync' })
function edit() { memberId.value = result.value.memberId || ''; reason.value = ''; operation.value = 'bind'; feedback.value = ''; editing.value = true }
async function submit() {
  if (busy.value || blocked.value || !enabled.value || !reason.value.trim() || (operation.value === 'bind' && !memberId.value.trim())) return
  const updated = await send(path, 'PUT', { version: result.value.version, memberId: operation.value === 'unbind' ? null : memberId.value.trim(), reason: reason.value.trim() })
  if (updated) { result.value = updated; editing.value = false; memberId.value = ''; reason.value = ''; feedback.value = '企业微信绑定已更新，原登录会话已失效。'; emit('changed') }
}
function refreshed() { editing.value = false; memberId.value = ''; reason.value = ''; feedback.value = '已重新读取，请核对当前绑定再决定是否修改。'; emit('changed') }
void load(); void status.load()
</script>
<template>
  <section class="oa-surface" aria-labelledby="oa-wecom-binding">
    <header class="oa-section-head"><div><h2 id="oa-wecom-binding">企业微信身份绑定</h2><p class="oa-muted">将已有员工账号与企业成员 ID 显式关联；绑定操作不会创建账号或变更角色。</p></div><ElButton v-if="result && !editing" :disabled="unavailable || loading || !enabled" @click="edit">修改绑定</ElButton></header>
    <RemoteState :loading="loading || status.loading.value" :error="error || status.error.value" @retry="load(); status.load()" />
    <p v-if="!status.loading.value && !enabled" class="oa-muted">企业微信尚未配置或状态不可用；配置完成后才能修改绑定。</p>
    <p v-if="status.result.value?.wecomLogin === 'mock'" class="oa-muted">当前连接模拟授权服务。</p>
    <dl v-if="result && !editing" class="oa-description"><div><dt>企业成员 ID</dt><dd>{{ result.memberId || '未绑定' }}</dd></div><div><dt>版本</dt><dd>{{ result.version }}</dd></div></dl>
    <p v-if="feedback" class="oa-status" role="status">{{ feedback }}</p>
    <ElAlert v-if="writeError" class="oa-inline-alert" :title="writeError" type="warning" :closable="false" />
    <form v-if="editing" class="oa-form" autocomplete="off" @submit.prevent="submit">
      <div class="oa-fields"><label for="oa-wecom-operation">操作<select id="oa-wecom-operation" v-model="operation" :disabled="busy || Boolean(blocked)"><option value="bind">绑定或更换成员</option><option value="unbind">解除绑定</option></select></label>
      <label v-if="operation === 'bind'" for="oa-wecom-member">企业成员 ID<input id="oa-wecom-member" v-model="memberId" required maxlength="128" autocapitalize="off" :disabled="busy || Boolean(blocked)"></label>
      <label for="oa-wecom-reason">变更原因<input id="oa-wecom-reason" v-model="reason" required maxlength="500" :disabled="busy || Boolean(blocked)"></label></div>
      <p class="oa-muted">成员 ID 保存后统一为小写，且在同一企业及接入模式内必须唯一。请先核对企业通讯录。保存后该员工现有登录会话将失效。</p>
      <div class="oa-actions"><ElButton v-if="blocked" :loading="busy" @click="reconcile(load, refreshed)">重新读取并核对</ElButton><ElButton v-else type="primary" native-type="submit" :loading="busy">{{ operation === 'unbind' ? '确认解除绑定' : '保存绑定' }}</ElButton><ElButton :disabled="busy || Boolean(blocked)" @click="editing = false; memberId = ''; reason = ''">取消</ElButton></div>
    </form>
  </section>
</template>
