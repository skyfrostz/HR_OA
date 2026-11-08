import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import LeaveFields from './leave-fields.vue'
import RequestEditor from './request-editor.vue'
import RequestDetail from './request-detail.vue'
import RequestActions from './request-actions.vue'

const { session } = vi.hoisted(() => ({
  session: {
    account: {
      user: { id: 1, employeeId: 11, role: 'EMPLOYEE' },
      permissions: { canSubmitRequests: true }
    },
    authorized: vi.fn()
  }
}))
vi.mock('../oa/session', () => ({ useSession: () => session }))
const form = {
  leaveTypeId: 1,
  startDate: '2026-12-31',
  endDate: '2027-01-01',
  startHalf: 'PM',
  endHalf: 'AM',
  reason: '测试请假'
}
const calculation = {
  ...form,
  leaveTypeName: '年假',
  leaveTypeVersion: 2,
  calendarVersion: 3,
  usesBalance: true,
  totalHalfDays: 2,
  years: [
    { year: 2026, halfDays: 1 },
    { year: 2027, halfDays: 1 }
  ],
  days: [
    { date: '2026-12-31', halves: ['PM'] },
    { date: '2027-01-01', halves: ['AM'] }
  ]
}
const preview = {
  calculation,
  balances: [
    { year: 2026, remainingHalfDays: 10 },
    { year: 2027, remainingHalfDays: 12 }
  ],
  canSubmit: true,
  issues: []
}
let wrapper
function findButton(label) {
  const control = wrapper
    .findAll('button')
    .find((item) => item.text() === label)
  expect(control, label).toBeDefined()
  return control
}
function serve(extra) {
  session.authorized.mockImplementation(async (path, options) => {
    const custom = extra?.(path, options)
    if (custom !== undefined) return custom
    if (path === '/leave-types')
      return {
        list: [
          { id: 1, name: '年假', usesBalance: true, enabled: true, version: 2 }
        ]
      }
    if (path === '/request-templates')
      return {
        list: [
          { type: 'LEAVE', name: '请假申请', description: '请假', route: [] }
        ]
      }
    if (path === '/leave-preview') return preview
    throw new Error('Unexpected request: ' + path)
  })
}
beforeEach(() => {
  session.authorized.mockReset()
})
afterEach(() => {
  wrapper?.unmount()
  vi.restoreAllMocks()
})
async function editor() {
  wrapper = mount(RequestEditor)
  await flushPromises()
  await wrapper.find('button[aria-pressed]').trigger('click')
  await flushPromises()
  await wrapper.get('#h5-request-title').setValue('跨年半天')
  await wrapper.get('#h5-leave-type').setValue('1')
  await wrapper.get('#h5-field-startDate').setValue(form.startDate)
  await wrapper.get('#h5-field-endDate').setValue(form.endDate)
  await wrapper.get('#h5-start-half').setValue('PM')
  await wrapper.get('#h5-end-half').setValue('AM')
  await wrapper.get('#h5-field-reason').setValue(form.reason)
}
async function calculate() {
  await findButton('预览请假天数').trigger('click')
  await flushPromises()
}

describe('M5 leave and request interactions', () => {
  it('invalidates a preview when inputs change back and rejects a late calculation', async () => {
    let complete
    let pending = false
    serve((path) =>
      path === '/leave-preview' && pending
        ? new Promise((resolve) => {
            complete = resolve
          })
        : undefined
    )
    wrapper = mount(
      defineComponent({
        setup() {
          const value = ref({ ...form })
          return () =>
            h(LeaveFields, {
              modelValue: value.value,
              'onUpdate:modelValue': (next) => {
                value.value = next
              }
            })
        }
      })
    )
    await flushPromises()
    await calculate()
    expect(wrapper.text()).toContain('2026 年：0.5 天')
    await wrapper.get('#h5-field-endDate').setValue('2027-01-02')
    await wrapper.get('#h5-field-endDate').setValue(form.endDate)
    expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
    pending = true
    await findButton('预览请假天数').trigger('click')
    await wrapper.get('#h5-start-half').setValue('AM')
    await wrapper.get('#h5-start-half').setValue('PM')
    complete(preview)
    await flushPromises()
    expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
    expect(
      wrapper.findComponent(LeaveFields).emitted('validated').at(-1)
    ).toEqual([null])
  })

  it('keeps the original intent on an unknown write and requires a fresh preview after stale replay', async () => {
    let writes = 0
    serve((path) => {
      if (path !== '/requests') return
      writes++
      return Promise.reject(
        writes === 1
          ? new TypeError('lost response')
          : { status: 409, errorCode: 'STALE_RECORD', message: 'stale' }
      )
    })
    await editor()
    expect(findButton('提交申请').attributes('disabled')).toBeDefined()
    await calculate()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(
      wrapper.get('#h5-field-startDate').attributes('disabled')
    ).toBeDefined()
    await findButton('重试原提交').trigger('click')
    await flushPromises()
    const requests = session.authorized.mock.calls.filter(
      ([path]) => path === '/requests'
    )
    expect(requests).toHaveLength(2)
    expect(requests[1][1]).toEqual(requests[0][1])
    expect(requests[0][1].data.form).toEqual({
      ...form,
      leaveTypeVersion: 2,
      calendarVersion: 3
    })
    expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('重新预览后再提交')
    expect(findButton('提交申请').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(writes).toBe(2)
    await calculate()
    expect(findButton('提交申请').attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).not.toContain('重新预览后再提交')
  })

  it.each(['LEAVE_OVERLAP', 'INSUFFICIENT_LEAVE'])(
    'clears the preview after %s at submit',
    async (code) => {
      serve((path) =>
        path === '/requests'
          ? Promise.reject({ status: 409, errorCode: code })
          : undefined
      )
      await editor()
      await calculate()
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
      expect(findButton('提交申请').attributes('disabled')).toBeDefined()
    }
  )

  it('retries withdrawal with one frozen key and reason', async () => {
    let writes = 0
    serve((path) =>
      path.endsWith('/actions')
        ? ++writes === 1
          ? Promise.reject(new TypeError('lost response'))
          : { id: 7, status: 'WITHDRAWN' }
        : undefined
    )
    wrapper = mount(RequestActions, {
      props: { requestId: 7, operations: ['WITHDRAW', 'REMIND'] }
    })
    await findButton('撤回').trigger('click')
    await wrapper.get('textarea').setValue('调整时间')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('textarea').attributes('disabled')).toBeDefined()
    expect(findButton('取消').attributes('disabled')).toBeDefined()
    await findButton('重试原操作').trigger('click')
    await flushPromises()
    const calls = session.authorized.mock.calls
    expect(calls).toHaveLength(2)
    expect(calls[0][1]).toEqual(calls[1][1])
    expect(calls[0][1].data).toEqual({ action: 'WITHDRAW', reason: '调整时间' })
    expect(wrapper.emitted('changed')).toEqual([['WITHDRAW']])
  })

  it('shows reminder throttling as a definite result and unlocks cancellation', async () => {
    serve(() =>
      Promise.reject({ status: 429, errorCode: 'REMINDER_RATE_LIMIT' })
    )
    wrapper = mount(RequestActions, {
      props: { requestId: 7, operations: ['REMIND'] }
    })
    await findButton('催办').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('不足 5 分钟')
    expect(wrapper.text()).not.toContain('重试原操作')
    expect(findButton('取消').attributes('disabled')).toBeUndefined()
    expect(session.authorized).toHaveBeenCalledTimes(1)
  })
})

it('distinguishes a future handoff from the immutable route snapshot', async () => {
  serve((path) =>
    path === '/requests/7'
      ? {
          id: 7,
          type: 'GENERAL',
          title: '测试申请',
          status: 'IN_REVIEW',
          applicantId: 9,
          applicantName: '申请人',
          createdAt: '2026-11-07T00:00:00Z',
          form: { content: '测试' },
          currentStep: 1,
          tasks: [],
          route: [{ step: 2, name: '复核', assigneeName: '原复核人' }],
          history: [
            {
              id: 5,
              action: 'HANDOFF',
              actorName: '管理员',
              routeStep: 2,
              taskId: null,
              previousAssigneeId: 3,
              targetUserId: 4,
              targetName: '接手人',
              createdAt: '2026-11-07T01:00:00Z'
            }
          ]
        }
      : undefined
  )
  wrapper = mount(RequestDetail, { props: { requestId: 7 } })
  await flushPromises()
  expect(wrapper.text()).toContain('后续第 2 步')
  expect(wrapper.text()).toContain('原账号 #3 → 接手人')
  expect(wrapper.text()).toContain('原复核人')
})

it('associates calculation errors with the first invalid control, focuses it and clears corrected errors', async () => {
  serve()
  const host = document.createElement('div')
  document.body.append(host)
  try {
    wrapper = mount(
      defineComponent({
        setup() {
          const value = ref({
            ...form,
            leaveTypeId: null,
            startDate: '',
            endDate: ''
          })
          return () =>
            h(LeaveFields, {
              modelValue: value.value,
              'onUpdate:modelValue': (next) => {
                value.value = next
              }
            })
        }
      }),
      { attachTo: host }
    )
    await flushPromises()
    for (const group of wrapper.findAll('.h5-leave-field')) {
      group.element.scrollIntoView = vi.fn()
    }
    const expectAdjacentError = (control) => {
      const group = control.element.closest('.h5-leave-field')
      expect(wrapper.findAll('#h5-leave-error')).toHaveLength(1)
      expect(group.contains(wrapper.get('#h5-leave-error').element)).toBe(true)
      expect(group.scrollIntoView).toHaveBeenLastCalledWith({
        block: 'center',
        inline: 'nearest'
      })
      expect(wrapper.get('#h5-field-reason').element.value).toBe(form.reason)
    }
    await calculate()
    const type = wrapper.get('#h5-leave-type')
    expect(type.attributes('aria-invalid')).toBe('true')
    expect(type.attributes('aria-describedby')).toBe('h5-leave-error')
    expect(document.activeElement).toBe(type.element)
    expectAdjacentError(type)
    expect(wrapper.get('#h5-leave-error').text()).toBe('请选择请假类型。')
    await type.setValue('1')
    await flushPromises()
    expect(type.attributes('aria-invalid')).toBeUndefined()
    expect(wrapper.find('#h5-leave-error').exists()).toBe(false)

    await calculate()
    const start = wrapper.get('#h5-field-startDate')
    expect(start.attributes('aria-invalid')).toBe('true')
    expect(start.attributes('aria-describedby')).toBe('h5-leave-error')
    expect(document.activeElement).toBe(start.element)
    expectAdjacentError(start)
    await wrapper.get('#h5-field-startDate').setValue('2026-12-31')
    expect(start.attributes('aria-invalid')).toBeUndefined()
    expect(start.attributes('aria-describedby')).toBeUndefined()
    expect(wrapper.find('#h5-leave-error').exists()).toBe(false)
    await calculate()
    const end = wrapper.get('#h5-field-endDate')
    expect(end.attributes('aria-invalid')).toBe('true')
    expect(end.attributes('aria-describedby')).toBe('h5-leave-error')
    expect(document.activeElement).toBe(end.element)
    expectAdjacentError(end)
    expect(wrapper.get('#h5-leave-error').text()).toBe('请选择结束日期。')
    await wrapper.get('#h5-field-endDate').setValue('2026-12-30')
    await calculate()
    expect(wrapper.get('#h5-leave-error').text()).toBe(
      '结束日期不能早于开始日期。'
    )
    expect(document.activeElement).toBe(end.element)
    expectAdjacentError(end)
    await wrapper.get('#h5-field-endDate').setValue('2026-12-31')
    expect(end.attributes('aria-invalid')).toBeUndefined()
    expect(end.attributes('aria-describedby')).toBeUndefined()
    expect(wrapper.find('#h5-leave-error').exists()).toBe(false)
    await wrapper.get('#h5-start-half').setValue('PM')
    await wrapper.get('#h5-end-half').setValue('AM')
    await calculate()
    const half = wrapper.get('#h5-end-half')
    expect(half.attributes('aria-invalid')).toBe('true')
    expect(half.attributes('aria-describedby')).toBe('h5-leave-error')
    expect(document.activeElement).toBe(half.element)
    expectAdjacentError(half)
    await half.setValue('PM')
    await flushPromises()
    expect(half.attributes('aria-invalid')).toBeUndefined()
    expect(wrapper.find('#h5-leave-error').exists()).toBe(false)
    expect(
      session.authorized.mock.calls.some(([path]) => path === '/leave-preview')
    ).toBe(false)
  } finally {
    host.remove()
  }
})

it('keeps failed preview errors by the calculation action and preserves all entered fields', async () => {
  serve((path) =>
    path === '/leave-preview'
      ? Promise.reject(new Error('暂时无法计算，请重试。'))
      : undefined
  )
  wrapper = mount(LeaveFields, { props: { modelValue: { ...form } } })
  await flushPromises()
  await calculate()
  const error = wrapper.get('#h5-leave-preview-error')
  expect(error.text()).toBe('暂时无法计算，请重试。')
  expect(error.element.closest('.h5-leave-field')).toBeNull()
  expect(findButton('预览请假天数').attributes('aria-describedby')).toBe(
    'h5-leave-preview-error'
  )
  expect(wrapper.find('#h5-leave-error').exists()).toBe(false)
  expect(wrapper.findAll('[role="alert"]')).toHaveLength(1)
  expect(wrapper.get('#h5-leave-type').element.value).toBe(
    String(form.leaveTypeId)
  )
  expect(wrapper.get('#h5-field-startDate').element.value).toBe(form.startDate)
  expect(wrapper.get('#h5-field-endDate').element.value).toBe(form.endDate)
  expect(wrapper.get('#h5-start-half').element.value).toBe(form.startHalf)
  expect(wrapper.get('#h5-end-half').element.value).toBe(form.endHalf)
  expect(wrapper.get('#h5-field-reason').element.value).toBe(form.reason)
})
