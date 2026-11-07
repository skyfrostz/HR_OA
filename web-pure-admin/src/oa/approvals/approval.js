import { parseSalary } from '../format'

export const typeLabels = {
  LEAVE: '请假申请',
  EXPENSE: '报销申请',
  GENERAL: '通用申请'
}

export const statusLabels = {
  IN_REVIEW: '审批中',
  PENDING: '待审批',
  APPROVED: '已通过',
  REJECTED: '已驳回',
  WITHDRAWN: '已撤回',
  CANCELLED: '已撤销'
}

export const actionLabels = {
  SUBMIT: '发起申请',
  APPROVE: '通过',
  REJECT: '驳回',
  TRANSFER: '转交',
  HANDOFF: '交接',
  WITHDRAW: '撤回',
  CANCEL: '撤销请假',
  REMIND: '催办'
}

export const formFields = {
  LEAVE: [
    { name: 'startDate', label: '开始日期', input: 'date' },
    { name: 'endDate', label: '结束日期', input: 'date' },
    { name: 'reason', label: '请假事由', input: 'textarea', limit: 1000 }
  ],
  EXPENSE: [
    { name: 'expenseDate', label: '费用日期', input: 'date' },
    { name: 'amount', label: '报销金额（元）', input: 'decimal' },
    { name: 'reason', label: '报销事由', input: 'textarea', limit: 1000 }
  ],
  GENERAL: [
    { name: 'content', label: '申请内容', input: 'textarea', limit: 2000 }
  ]
}

const dateFormat = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Shanghai' })

export function displayDate(value) {
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? dateFormat.format(timestamp) : '—'
}

export function initialDraft(type, previous) {
  const fields = formFields[type] || []
  const form = Object.fromEntries(fields.map(({ name }) => [name, previous?.form[name] || '']))
  if (type === 'EXPENSE' && Number.isSafeInteger(previous?.form.amountCents)) {
    const cents = previous.form.amountCents
    const remainder = cents % 100
    form.amount = String((cents - remainder) / 100) + '.' + String(remainder).padStart(2, '0')
  }
  if (type === 'LEAVE') Object.assign(form, { leaveTypeId: previous?.form.leaveTypeId || null, startHalf: previous?.form.startHalf || 'AM', endHalf: previous?.form.endHalf || 'PM' })
  return { title: previous?.title || '', form }
}

export function requestPayload(type, draft, previousId) {
  const title = draft.title.trim()
  const fields = formFields[type]
  if (!title || [...title].length > 100 || !fields) {
    throw new Error('请填写标题，最多 100 字。')
  }

  const form = {}
  for (const field of fields) {
    const value = draft.form[field.name].trim()
    if (field.input === 'decimal') {
      const amountCents = parseSalary(value)
      if (!amountCents || amountCents < 1) throw new Error('请填写大于零且最多两位小数的金额。')
      form.amountCents = amountCents
    } else if (field.input === 'date') {
      if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) throw new Error('请选择' + field.label + '。')
      form[field.name] = value
    } else {
      if (!value || [...value].length > field.limit) {
        throw new Error('请填写' + field.label + '，最多 ' + field.limit + ' 字。')
      }
      form[field.name] = value
    }
  }

  if (type === 'LEAVE' && form.startDate > form.endDate) {
    throw new Error('结束日期不能早于开始日期。')
  }

  if (type === 'LEAVE') {
    if (!Number.isSafeInteger(draft.form.leaveTypeId) || draft.form.leaveTypeId < 1) throw new Error('请选择请假类型。')
    if (!['AM', 'PM'].includes(draft.form.startHalf) || !['AM', 'PM'].includes(draft.form.endHalf)) throw new Error('请选择开始和结束时段。')
    Object.assign(form, { leaveTypeId: draft.form.leaveTypeId, startHalf: draft.form.startHalf, endHalf: draft.form.endHalf })
    if (form.startDate === form.endDate && form.startHalf === 'PM' && form.endHalf === 'AM') throw new Error('结束时段不能早于开始时段。')
  }
  const payload = { type, title, form }
  if (previousId) payload.previousRequestId = previousId
  return payload
}

const actionErrorMessages = {
  STALE_RECORD: '配置或记录已变化。请重新预览并确认后再提交。',
  LEAVE_OVERLAP: '请假时段与已有申请重叠，请调整日期后重新预览。',
  INSUFFICIENT_LEAVE: '年度可用余额不足，请调整日期或联系管理员核对额度。',
  REMINDER_RATE_LIMIT: '距上次催办不足 5 分钟，请稍后再试。',
  INVALID_STATE: '申请状态已变化，请重新读取后再操作。',
  STALE_TASK: '待办已变化。请核对重新加载的记录后再操作。',
  ROUTE_UNAVAILABLE: '当前审批路径有节点不可用，请稍后再试。',
  TRANSFER_LIMIT: '该申请已达转交次数上限。',
  IDEMPOTENCY_KEY_REUSED: '本次提交与原操作内容冲突，请先核对当前记录。'
}

export function approvalError(cause) {
  if (cause.errorCode === 'REMINDER_RATE_LIMIT') return { retryable: false, message: actionErrorMessages.REMINDER_RATE_LIMIT }
  if (cause.errorCode === 'IDEMPOTENCY_IN_PROGRESS') {
    return { retryable: true, message: '系统仍在处理原操作。稍后重试会继续核对同一次提交。' }
  }
  if (!cause.status || cause.status >= 500 || cause.status === 429) {
    return { retryable: true, message: '暂时无法确认提交结果。重试会沿用原提交内容。' }
  }
  return { retryable: false, message: actionErrorMessages[cause.errorCode] || cause.message }
}

export function statusTone(status) {
  if (status === 'APPROVED') return 'success'
  if (status === 'REJECTED') return 'danger'
  return 'warning'
}
