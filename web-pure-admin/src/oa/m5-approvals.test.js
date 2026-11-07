import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import LeaveFields from './approvals/leave-fields.vue'
import RequestEditor from './approvals/request-editor.vue'
import RequestDetail from './approvals/request-detail.vue'
import RequestActions from './approvals/request-actions.vue'

const { session } = vi.hoisted(() => ({ session: { account: { user: { id: 1, employeeId: 11, role: 'EMPLOYEE' }, permissions: { canSubmitRequests: true } }, authorized: vi.fn() } }))
vi.mock('./session', () => ({ useSession: () => session }))
const form = { leaveTypeId: 1, startDate: '2026-12-31', endDate: '2027-01-01', startHalf: 'PM', endHalf: 'AM', reason: '测试请假' }
const calculation = { ...form, leaveTypeName: '年假', leaveTypeVersion: 2, calendarVersion: 3, usesBalance: true, totalHalfDays: 2, years: [{ year: 2026, halfDays: 1 }, { year: 2027, halfDays: 1 }], days: [{ date: '2026-12-31', halves: ['PM'] }, { date: '2027-01-01', halves: ['AM'] }] }
const preview = { calculation, balances: [{ year: 2026, remainingHalfDays: 10 }, { year: 2027, remainingHalfDays: 12 }], canSubmit: true, issues: [] }
let wrapper
function findButton(label) { const control = wrapper.findAll('button').find(item => item.text() === label); expect(control, label).toBeDefined(); return control }
function serve(extra) {
  session.authorized.mockImplementation(async (path, options) => {
    const custom = extra?.(path, options)
    if (custom !== undefined) return custom
    if (path === '/leave-types') return { list: [{ id: 1, name: '年假', usesBalance: true, enabled: true, version: 2 }] }
    if (path === '/request-templates') return { list: [{ type: 'LEAVE', name: '请假申请', description: '请假', route: [] }] }
    if (path === '/leave-preview') return preview
    throw new Error('Unexpected request: ' + path)
  })
}
beforeEach(() => { session.authorized.mockReset() })
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks() })
async function editor() {
  wrapper = mount(RequestEditor)
  await flushPromises()
  await wrapper.find('button[aria-pressed]').trigger('click')
  await flushPromises()
  await wrapper.get('#pc-request-title').setValue('跨年半天')
  await wrapper.get('#pc-leave-type').setValue('1')
  await wrapper.get('#pc-field-startDate').setValue(form.startDate)
  await wrapper.get('#pc-field-endDate').setValue(form.endDate)
  await wrapper.get('#pc-start-half').setValue('PM')
  await wrapper.get('#pc-end-half').setValue('AM')
  await wrapper.get('#pc-field-reason').setValue(form.reason)
}
async function calculate() { await findButton('预览请假天数').trigger('click'); await flushPromises() }

describe('M5 leave and request interactions', () => {
  it('invalidates a preview when inputs change back and rejects a late calculation', async () => {
    let complete
    let pending = false
    serve(path => path === '/leave-preview' && pending ? new Promise(resolve => { complete = resolve }) : undefined)
    wrapper = mount(defineComponent({ setup() { const value = ref({ ...form }); return () => h(LeaveFields, { modelValue: value.value, 'onUpdate:modelValue': next => { value.value = next } }) } }))
    await flushPromises()
    await calculate()
    expect(wrapper.text()).toContain('2026 年：0.5 天')
    await wrapper.get('#pc-field-endDate').setValue('2027-01-02')
    await wrapper.get('#pc-field-endDate').setValue(form.endDate)
    expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
    pending = true
    await findButton('预览请假天数').trigger('click')
    await wrapper.get('#pc-start-half').setValue('AM')
    await wrapper.get('#pc-start-half').setValue('PM')
    complete(preview)
    await flushPromises()
    expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
    expect(wrapper.findComponent(LeaveFields).emitted('validated').at(-1)).toEqual([null])
  })

  it('keeps the original intent on an unknown write and requires a fresh preview after stale replay', async () => {
    let writes = 0
    serve(path => {
      if (path !== '/requests') return
      writes++
      return Promise.reject(writes === 1 ? new TypeError('lost response') : { status: 409, errorCode: 'STALE_RECORD', message: 'stale' })
    })
    await editor()
    expect(findButton('提交申请').attributes('disabled')).toBeDefined()
    await calculate()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('#pc-field-startDate').attributes('disabled')).toBeDefined()
    await findButton('重试原提交').trigger('click')
    await flushPromises()
    const requests = session.authorized.mock.calls.filter(([path]) => path === '/requests')
    expect(requests).toHaveLength(2)
    expect(requests[1][1]).toEqual(requests[0][1])
    expect(requests[0][1].data.form).toEqual({ ...form, leaveTypeVersion: 2, calendarVersion: 3 })
    expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('重新预览后再提交')
    expect(findButton('提交申请').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(writes).toBe(2)
    await calculate()
    expect(findButton('提交申请').attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).not.toContain('重新预览后再提交')
  })

  it.each(['LEAVE_OVERLAP', 'INSUFFICIENT_LEAVE'])('clears the preview after %s at submit', async code => {
    serve(path => path === '/requests' ? Promise.reject({ status: 409, errorCode: code }) : undefined)
    await editor(); await calculate()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.find('[aria-label="请假计算预览"]').exists()).toBe(false)
    expect(findButton('提交申请').attributes('disabled')).toBeDefined()
  })

  it('retries withdrawal with one frozen key and reason', async () => {
    let writes = 0
    serve(path => path.endsWith('/actions') ? (++writes === 1 ? Promise.reject(new TypeError('lost response')) : { id: 7, status: 'WITHDRAWN' }) : undefined)
    wrapper = mount(RequestActions, { props: { requestId: 7, operations: ['WITHDRAW', 'REMIND'] } })
    await findButton('撤回').trigger('click')
    await wrapper.get('textarea').setValue('调整时间')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.get('textarea').attributes('disabled')).toBeDefined()
    expect(findButton('取消').attributes('disabled')).toBeDefined()
    await findButton('重试原操作').trigger('click'); await flushPromises()
    const calls = session.authorized.mock.calls
    expect(calls).toHaveLength(2)
    expect(calls[0][1]).toEqual(calls[1][1])
    expect(calls[0][1].data).toEqual({ action: 'WITHDRAW', reason: '调整时间' })
    expect(wrapper.emitted('changed')).toEqual([['WITHDRAW']])
  })

  it('shows reminder throttling as a definite result and unlocks cancellation', async () => {
    serve(() => Promise.reject({ status: 429, errorCode: 'REMINDER_RATE_LIMIT' }))
    wrapper = mount(RequestActions, { props: { requestId: 7, operations: ['REMIND'] } })
    await findButton('催办').trigger('click'); await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('不足 5 分钟')
    expect(wrapper.text()).not.toContain('重试原操作')
    expect(findButton('取消').attributes('disabled')).toBeUndefined()
    expect(session.authorized).toHaveBeenCalledTimes(1)
  })
})

it('distinguishes a future handoff from the immutable route snapshot', async () => {
  serve(path => path === '/requests/7' ? { id: 7, type: 'GENERAL', title: '测试申请', status: 'IN_REVIEW', applicantId: 9, applicantName: '申请人', createdAt: '2026-11-07T00:00:00Z', form: { content: '测试' }, currentStep: 1, tasks: [], route: [{ step: 2, name: '复核', assigneeName: '原复核人' }], history: [{ id: 5, action: 'HANDOFF', actorName: '管理员', routeStep: 2, taskId: null, previousAssigneeId: 3, targetUserId: 4, targetName: '接手人', createdAt: '2026-11-07T01:00:00Z' }] } : undefined)
  wrapper = mount(RequestDetail, { props: { requestId: 7 } }); await flushPromises()
  expect(wrapper.text()).toContain('后续第 2 步')
  expect(wrapper.text()).toContain('原账号 #3 → 接手人')
  expect(wrapper.text()).toContain('原复核人')
})
