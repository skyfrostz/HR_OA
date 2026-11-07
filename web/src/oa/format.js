const numberFormat = new Intl.NumberFormat('zh-CN')

export function formatSalary(cents) {
  if (!Number.isSafeInteger(cents) || cents < 0) return '—'
  const remainder = cents % 100
  return '¥' + numberFormat.format((cents - remainder) / 100) + '.' + String(remainder).padStart(2, '0')
}

export function parseSalary(value) {
  const match = /^(\d+)(?:\.(\d{1,2}))?$/.exec(value.trim())
  if (!match) return null
  const cents = Number(match[1] + (match[2] || '').padEnd(2, '0'))
  return Number.isSafeInteger(cents) ? cents : null
}

export const contractStatuses = {
  DRAFT: { label: '草拟', type: 'info' },
  SIGNED: { label: '已签约', type: 'success' },
  EXPIRED: { label: '已到期', type: 'warning' },
  TERMINATED: { label: '已终止', type: 'danger' }
}

export const roleLabels = {
  HR_ADMIN: '人事管理员',
  HR_VIEWER: '人事查阅员',
  EMPLOYEE: '员工',
  APPROVER: '审批人'
}

export function nameInitial(name) {
  return Array.from(name?.trim() || '人')[0]
}
