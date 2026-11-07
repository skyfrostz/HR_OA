import { shallowRef } from 'vue'
import { useSession } from './session'

const messages = {
  RULE_CONFLICT: '同类型、同优先级的启用规则有重叠，请调整部门、金额范围或优先级。',
  INSUFFICIENT_LEAVE: '额度不足以覆盖人工已用、审批已用和预占，请核对后调整。',
  STALE_TASK: '任务已变化，请重新检查异常后再交接。',
  ACCOUNT_CONFLICT: '用户名已使用，或此账号已经绑定到其他员工。',
  HANDOFF_REQUIRED: '此操作会影响待办或后续审批，请选择交接人。',
  HANDOFF_CONFLICT: '该交接人目前不可用或与审批路径冲突，请重新选择。',
  INVALID_STATE: '记录状态已变化或不允许这项操作，请重新读取。'
}

export function useVersionedWrite() {
  const session = useSession()
  const busy = shallowRef(false)
  const blocked = shallowRef('')
  const error = shallowRef('')

  async function send(path, method, data) {
    if (busy.value || blocked.value) return null
    busy.value = true
    error.value = ''
    try {
      return await session.authorized(path, { method, data })
    } catch (cause) {
      if (!cause.status || cause.status >= 500 || cause.status === 429) {
        blocked.value = 'unknown'
        error.value = '无法确认操作结果。请重新读取并核对状态及变更记录，系统不会自动重试。'
      } else if (['STALE_RECORD', 'STALE_TASK', 'INVALID_STATE'].includes(cause.errorCode)) {
        blocked.value = 'stale'
        error.value = messages[cause.errorCode] || '记录已变化，请重新读取并核对后再操作。'
      } else {
        error.value = messages[cause.errorCode] || cause.message
      }
      return null
    } finally {
      busy.value = false
    }
  }

  async function reconcile(read, afterRead) {
    if (busy.value) return
    busy.value = true
    let refreshed
    try {
      refreshed = await read()
    } finally {
      busy.value = false
    }
    if (refreshed) {
      const previous = blocked.value
      blocked.value = ''
      error.value = ''
      afterRead?.(previous)
    }
  }

  return { busy, blocked, error, send, reconcile }
}
