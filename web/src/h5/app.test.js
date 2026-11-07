import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import App from './app.vue'

const timestamp = '2026-11-06T00:00:00Z'
const applicant = { id: 1, displayName: '演示申请人', role: 'EMPLOYEE' }
const manager = { id: 2, displayName: '演示负责人', role: 'APPROVER' }
const delegate = { id: 4, displayName: '演示代理审批人', role: 'APPROVER' }
const route = [
  { step: 1, name: '负责人审批', assigneeId: 2, assigneeName: manager.displayName },
  { step: 2, name: '复核审批', assigneeId: 3, assigneeName: '演示复核人' }
]
const templates = [
  { type: 'LEAVE', name: '请假申请', version: 1, description: '提交请假日期', route },
  { type: 'EXPENSE', name: '报销申请', version: 1, description: '提交报销金额', route },
  { type: 'GENERAL', name: '通用申请', version: 1, description: '提交一般事项', route }
]
const summary = {
  id: 7, type: 'GENERAL', title: '模拟设备领用', status: 'IN_REVIEW',
  applicantId: 1, applicantName: applicant.displayName, previousRequestId: null,
  currentStep: 1, createdAt: timestamp, updatedAt: timestamp
}
const task = {
  id: 9, requestId: 7, requestTitle: summary.title, requestType: summary.type,
  applicantName: applicant.displayName, step: 1, stepName: '负责人审批', assigneeId: 2,
  assigneeName: manager.displayName, status: 'PENDING', version: 1, createdAt: timestamp, completedAt: null
}
const submitAction = {
  id: 11, requestId: 7, requestTitle: summary.title, requestType: summary.type, taskId: null,
  actorId: 1, actorName: applicant.displayName, action: 'SUBMIT', comment: '', targetUserId: null,
  targetName: null, createdAt: timestamp
}
const detail = {
  ...summary, templateVersion: 1, form: { content: '需要一套测试设备' },
  route, tasks: [task], history: [submitAction]
}

function json(data, status = 200) {
  return new Response(JSON.stringify({ code: status < 400 ? 0 : 7, data }), {
    status,
    headers: { 'Content-Type': 'application/json' }
  })
}

function page(list) {
  return json({ list, total: list.length, page: 1, pageSize: 20 })
}

function fixtureServer({ user = applicant, onRequest } = {}) {
  const mock = vi.fn(async (url, options) => {
    const path = url.replace('/api/oa/v1', '')
    if (path === '/auth/login') {
      return json({
        user, token: 'fixture', expiresAt: new Date(Date.now() + 60_000).toISOString(),
        permissions: {
          canSubmitRequests: user.role === 'EMPLOYEE' || user.role === 'APPROVER',
          canApproveRequests: user.role === 'APPROVER'
        }
      })
    }
    const result = onRequest?.(path, options)
    if (result) return result
    if (path === '/request-templates') return json({ list: templates, integrationMode: 'mock' })
    if (path === '/requests/7') return json(detail)
    if (path === '/requests?page=1&pageSize=20') return page([summary])
    if (path === '/tasks?page=1&pageSize=20') return page([task])
    return json({ errorCode: 'NOT_FOUND' }, 404)
  })
  vi.stubGlobal('fetch', mock)
  return mock
}

function findButton(wrapper, label) {
  const match = wrapper.findAll('button').find((item) => item.text() === label)
  expect(match, 'button ' + label).toBeDefined()
  return match
}

async function clickTab(wrapper, label) {
  const match = wrapper.findAll('[role="tab"]').find((item) => item.text() === label)
  expect(match, 'tab ' + label).toBeDefined()
  await match.trigger('click')
  await flushPromises()
}

async function openRecord(wrapper, title = summary.title) {
  const match = wrapper.findAll('button').find((item) => item.text().includes(title))
  expect(match, 'record ' + title).toBeDefined()
  await match.trigger('click')
  await flushPromises()
}

async function login(wrapper) {
  await wrapper.get('#h5-username').setValue('fixture-account')
  await wrapper.get('#h5-password').setValue('fixture-password')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}

describe('mobile approval entry', () => {
  let wrapper
  beforeEach(() => {
    // Keep Vant's deferred layout measurements inside this test's DOM lifetime.
    // Leave setImmediate real so flushPromises can still settle API responses.
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'requestAnimationFrame', 'cancelAnimationFrame'],
      loopLimit: 100
    })
    localStorage.clear()
    sessionStorage.clear()
    wrapper = mount(App, { global: { plugins: [createPinia()] } })
  })
  afterEach(async () => {
    try {
      wrapper.unmount()
      await flushPromises()
      // Execute pending measurements while window still exists; do not silence
      // their exceptions or discard them with clearAllTimers.
      await vi.runAllTimersAsync()
      await flushPromises()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      vi.useRealTimers()
      vi.restoreAllMocks()
      vi.unstubAllGlobals()
    }
  })

  it('refreshes an empty queue and keeps the active page when resuming foreground', async () => {
    let assigned = false
    let slow
    let slowSignal
    let secondPageCalls = 0
    const mock = fixtureServer({
      user: manager,
      onRequest(path, options) {
        if (path === '/tasks?page=1&pageSize=20') {
          return json({ list: assigned ? [task] : [], total: assigned ? 21 : 0, page: 1, pageSize: 20 })
        }
        if (path === '/tasks?page=2&pageSize=20') {
          if (++secondPageCalls === 2) {
            slowSignal = options.signal
            return new Promise((resolve) => { slow = resolve })
          }
          return json({ list: [task], total: 21, page: 2, pageSize: 20 })
        }
      }
    })
    await login(wrapper)
    await clickTab(wrapper, '待办')
    expect(wrapper.text()).toContain('当前没有待审批任务')
    assigned = true
    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain(summary.title)

    await findButton(wrapper, '下一页').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('第 2 / 2 页')
    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(slowSignal.aborted).toBe(false)
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(slowSignal.aborted).toBe(true)
    slow(json({ list: [], total: 0, page: 2, pageSize: 20 }))
    await flushPromises()
    expect(wrapper.text()).toContain(summary.title)
    expect(wrapper.text()).toContain('第 2 / 2 页')
    expect(mock.mock.calls.filter(([url]) => url.includes('/tasks?page=2'))).toHaveLength(3)
  })

  it('submits leave with a frozen intent through a network failure and an in-progress replay', async () => {
    const submitted = {
      ...detail, type: 'LEAVE', title: '演示请假',
      form: { startDate: '2026-11-09', endDate: '2026-11-10', reason: '演示事由' }
    }
    let attempts = 0
    const mock = fixtureServer({
      onRequest(path) {
        if (path === '/requests/7') return json(submitted)
        if (path !== '/requests') return
        attempts++
        if (attempts === 1) throw new TypeError('fixture network error')
        if (attempts === 2) return json({ errorCode: 'IDEMPOTENCY_IN_PROGRESS' }, 409)
        return json(submitted, 201)
      }
    })
    await login(wrapper)
    expect(wrapper.text()).toContain('选择一种演示表单')
    expect(wrapper.findAll('[role="tab"]').map((item) => item.text())).not.toContain('待办')
    await openRecord(wrapper, '请假申请')
    await wrapper.get('#h5-request-title').setValue('演示请假')
    await wrapper.get('#h5-field-startDate').setValue('2026-11-09')
    await wrapper.get('#h5-field-endDate').setValue('2026-11-08')
    await wrapper.get('#h5-field-reason').setValue('演示事由')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('结束日期不能早于开始日期')
    expect(attempts).toBe(0)
    await wrapper.get('#h5-field-endDate').setValue('2026-11-10')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('暂时无法确认提交结果')
    expect(wrapper.get('#h5-field-reason').attributes('disabled')).toBeDefined()
    await clickTab(wrapper, '我的申请')
    expect(wrapper.text()).toContain('重试原提交')
    await findButton(wrapper, '重试原提交').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('系统仍在处理原操作')
    await findButton(wrapper, '重试原提交').trigger('click')
    await flushPromises()

    const writes = mock.mock.calls.filter(([url]) => url === '/api/oa/v1/requests')
    expect(writes).toHaveLength(3)
    expect(new Set(writes.map(([, options]) => options.headers.get('Idempotency-Key'))).size).toBe(1)
    expect(writes[0][1].headers.get('Idempotency-Key')).toMatch(/^[0-9a-f]{32}$/)
    expect(new Set(writes.map(([, options]) => options.body)).size).toBe(1)
    expect(JSON.parse(writes[0][1].body)).toEqual({
      type: 'LEAVE', title: submitted.title, form: submitted.form
    })
    expect(wrapper.text()).toContain('申请详情')
    expect(wrapper.text()).toContain('负责人审批')
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
    expect(document.cookie).toBe('')
  })

  it('returns to the active application list with one fresh request and reopens the updated record', async () => {
    let approved = false
    const listPath = '/api/oa/v1/requests?page=1&pageSize=20'
    const mock = fixtureServer({
      onRequest(path) {
        const status = approved ? 'APPROVED' : 'IN_REVIEW'
        if (path === '/requests?page=1&pageSize=20') return page([{ ...summary, status }])
        if (path === '/requests/7') return json({ ...detail, status })
      }
    })
    const listRequests = () => mock.mock.calls.filter(([url]) => url === listPath)

    await login(wrapper)
    await clickTab(wrapper, '我的申请')
    await openRecord(wrapper)
    expect(wrapper.get('#h5-detail-heading').text()).toBe('申请详情')
    expect(listRequests()).toHaveLength(1)

    approved = true
    await clickTab(wrapper, '我的申请')
    expect(wrapper.get('#h5-list-heading').text()).toBe('我的申请')
    expect(wrapper.text()).toContain('已通过')
    expect(listRequests()).toHaveLength(2)
    await clickTab(wrapper, '我的申请')
    expect(listRequests()).toHaveLength(2)
    await openRecord(wrapper)
    expect(wrapper.get('#h5-detail-heading').text()).toBe('申请详情')
    expect(wrapper.text()).toContain('已通过')
  })

  it('reloads stale decisions, approves with the new task version and refreshes the queue', async () => {
    let attempts = 0
    const mock = fixtureServer({
      user: manager,
      onRequest(path) {
        if (path === '/requests/7') {
          return json(attempts >= 2 ? { ...detail, status: 'APPROVED', tasks: [] } :
            { ...detail, tasks: [{ ...task, version: 1 + attempts }] })
        }
        if (path === '/tasks?page=1&pageSize=20') return page(attempts >= 2 ? [] : [task])
        if (path !== '/tasks/9/actions') return
        attempts++
        return attempts === 1 ? json({ errorCode: 'STALE_TASK' }, 409) : json({ ...detail, status: 'APPROVED' })
      }
    })
    await login(wrapper)
    await clickTab(wrapper, '待办')
    await openRecord(wrapper)
    await findButton(wrapper, '通过').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('待办已变化')
    await findButton(wrapper, '通过').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('已记录「通过」')
    expect(wrapper.text()).not.toContain('处理待办')
    await findButton(wrapper, '返回列表').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('当前没有待审批任务。')

    const actions = mock.mock.calls.filter(([url]) => url === '/api/oa/v1/tasks/9/actions')
    expect(actions.map(([, options]) => JSON.parse(options.body))).toEqual([
      { action: 'APPROVE', version: 1, comment: '' },
      { action: 'APPROVE', version: 2, comment: '' }
    ])
    expect(actions[0][1].headers.get('Idempotency-Key')).not.toBe(actions[1][1].headers.get('Idempotency-Key'))
    expect(mock.mock.calls.filter(([url]) => url === '/api/oa/v1/requests/7')).toHaveLength(3)
  })

  it('loads transfer candidates, records the transfer, and opens historical detail from the mock inbox', async () => {
    let moved = false
    const transferred = {
      ...submitAction, id: 12, taskId: 9, actorId: 2, actorName: manager.displayName,
      action: 'TRANSFER', comment: '演示转交', targetUserId: 4, targetName: delegate.displayName
    }
    const mock = fixtureServer({
      user: manager,
      onRequest(path) {
        if (path === '/tasks/9/transfer-targets') return json({ list: [{ id: 4, displayName: delegate.displayName }] })
        if (path === '/tasks/9/actions') {
          moved = true
          return json(detail)
        }
        if (path === '/tasks/history?page=1&pageSize=20') return page([transferred])
        if (path === '/notifications?page=1&pageSize=20') {
          return page([{ id: 1, eventId: 2, requestId: 7, title: '有新的待办申请', kind: 'TASK_ASSIGNED', createdAt: timestamp, deliveryMode: 'mock' }])
        }
        if (path === '/requests/7' && moved) return json({
          ...detail, tasks: [{ ...task, version: 2, assigneeId: 4, assigneeName: delegate.displayName }],
          history: [submitAction, transferred]
        })
      }
    })
    await login(wrapper)
    await clickTab(wrapper, '待办')
    await openRecord(wrapper)
    await findButton(wrapper, '转交').trigger('click')
    await flushPromises()
    await wrapper.get('#h5-transfer-target').setValue('4')
    await wrapper.get('#h5-action-comment').setValue('演示转交')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain(delegate.displayName)
    expect(wrapper.text()).not.toContain('处理待办')
    await clickTab(wrapper, '已办')
    expect(wrapper.text()).toContain('转交')
    await clickTab(wrapper, '站内通知')
    expect(wrapper.text()).toContain('仅接收模拟站内通知')
    await openRecord(wrapper, '有新的待办申请')
    expect(wrapper.text()).toContain('申请详情')
    expect(wrapper.text()).not.toContain('处理待办')
    const [, options] = mock.mock.calls.find(([url]) => url === '/api/oa/v1/tasks/9/actions')
    expect(JSON.parse(options.body)).toEqual({ action: 'TRANSFER', version: 1, comment: '演示转交', targetUserId: 4 })
  })

  it('requires a reason to reject and shows the resulting approval history', async () => {
    let rejected = false
    const closed = {
      ...detail, status: 'REJECTED', currentStep: null, tasks: [{ ...task, status: 'REJECTED' }],
      history: [...detail.history, { ...submitAction, id: 12, action: 'REJECT', comment: '演示驳回意见' }]
    }
    const mock = fixtureServer({
      user: manager,
      onRequest(path) {
        if (path === '/requests/7' && rejected) return json(closed)
        if (path === '/tasks/9/actions') {
          rejected = true
          return json(closed)
        }
      }
    })
    await login(wrapper)
    await clickTab(wrapper, '待办')
    await openRecord(wrapper)
    await findButton(wrapper, '驳回').trigger('click')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('请填写处理意见')
    expect(rejected).toBe(false)
    await wrapper.get('#h5-action-comment').setValue('演示驳回意见')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('已驳回')
    expect(wrapper.text()).toContain('演示驳回意见')
    expect(wrapper.text()).not.toContain('重新提交')
    const [, options] = mock.mock.calls.find(([url]) => url === '/api/oa/v1/tasks/9/actions')
    expect(JSON.parse(options.body)).toEqual({ action: 'REJECT', version: 1, comment: '演示驳回意见' })
  })

  it('shows a key conflict and leaves the user free to check existing applications', async () => {
    const mock = fixtureServer({
      onRequest(path) {
        if (path === '/requests') return json({ errorCode: 'IDEMPOTENCY_KEY_REUSED' }, 422)
      }
    })
    await login(wrapper)
    await openRecord(wrapper, '通用申请')
    await wrapper.get('#h5-request-title').setValue('演示冲突')
    await wrapper.get('#h5-field-content').setValue('演示内容')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('本次提交与原操作内容冲突')
    expect(wrapper.text()).not.toContain('重试原提交')
    await clickTab(wrapper, '我的申请')
    expect(wrapper.text()).toContain(summary.title)
    expect(mock.mock.calls.filter(([url]) => url === '/api/oa/v1/requests')).toHaveLength(1)
  })

  it('resubmits an edited rejected expense with exact cents and keeps the original ID', async () => {
    const rejected = {
      ...detail, type: 'EXPENSE', title: '原报销', status: 'REJECTED',
      form: { expenseDate: '2026-11-05', amountCents: 1009, reason: '演示费用' }
    }
    const revised = { ...rejected, id: 8, title: '补充报销', status: 'IN_REVIEW', previousRequestId: 7 }
    const mock = fixtureServer({
      onRequest(path) {
        if (path === '/requests?page=1&pageSize=20') return page([rejected])
        if (path === '/requests/7') return json(rejected)
        if (path === '/requests/8') return json(revised)
        if (path === '/requests') return json(revised, 201)
      }
    })
    await login(wrapper)
    await clickTab(wrapper, '我的申请')
    await openRecord(wrapper, '原报销')
    await findButton(wrapper, '重新提交').trigger('click')
    await flushPromises()
    expect(wrapper.get('#h5-field-amount').element.value).toBe('10.09')
    await wrapper.get('#h5-request-title').setValue('补充报销')
    await wrapper.get('#h5-field-amount').setValue('12.01')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('补充报销')
    await findButton(wrapper, '查看原申请').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('原报销')

    const [, options] = mock.mock.calls.find(([url]) => url === '/api/oa/v1/requests')
    expect(JSON.parse(options.body)).toEqual({
      type: 'EXPENSE', title: '补充报销', previousRequestId: 7,
      form: { expenseDate: '2026-11-05', amountCents: 1201, reason: '演示费用' }
    })
  })
})
