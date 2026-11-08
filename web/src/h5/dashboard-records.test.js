import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DashboardRecords from './dashboard-records.vue'

const { session } = vi.hoisted(() => ({ session: { authorized: vi.fn() } }))
vi.mock('../oa/session', () => ({ useSession: () => session }))
let wrapper
beforeEach(() => session.authorized.mockReset())
afterEach(() => wrapper?.unmount())

it('opens the request ID from a real assigned task, not the task ID', async () => {
  session.authorized.mockResolvedValue({
    list: [
      {
        id: 73,
        requestId: 12,
        requestTitle: '待审核的中文申请',
        status: 'PENDING',
        applicantName: '测试员工',
        stepName: '负责人审核'
      }
    ]
  })
  wrapper = mount(DashboardRecords, { props: { kind: 'pending' } })
  await flushPromises()
  expect(session.authorized.mock.calls[0][0]).toBe('/tasks?page=1&pageSize=5')
  await wrapper.get('.h5-record-button').trigger('click')
  expect(wrapper.emitted('open')).toEqual([[12]])
  expect(wrapper.text()).toContain('待审核的中文申请')
})

it('uses the applicant endpoint for personal records and reloads on refresh', async () => {
  session.authorized.mockResolvedValue({
    list: [{ id: 19, title: '我的申请', status: 'IN_REVIEW' }]
  })
  wrapper = mount(DashboardRecords, { props: { kind: 'mine', revision: 0 } })
  await flushPromises()
  expect(session.authorized.mock.calls[0][0]).toBe(
    '/requests?page=1&pageSize=5'
  )
  await wrapper.get('.h5-record-button').trigger('click')
  expect(wrapper.emitted('open')).toEqual([[19]])
  await wrapper.setProps({ revision: 1 })
  await flushPromises()
  expect(session.authorized).toHaveBeenCalledTimes(2)
})

it('keeps contract actions at the separately authenticated PC entry and never invents an approval ID', async () => {
  session.authorized.mockResolvedValue({
    list: [
      {
        contractId: 8,
        contractNo: '合同-测试',
        employeeName: '测试员工',
        endDate: '2026-11-10',
        daysRemaining: 2
      }
    ]
  })
  wrapper = mount(DashboardRecords, { props: { kind: 'contracts' } })
  await flushPromises()
  expect(session.authorized.mock.calls[0][0]).toBe(
    '/contract-reminders?page=1&pageSize=5&days=30'
  )
  expect(wrapper.get('a').attributes('href')).toBe(
    './oa.html#/contract-reminders'
  )
  expect(wrapper.find('.h5-record-button').exists()).toBe(false)
  expect(wrapper.emitted('open')).toBeUndefined()
})

it('shows a recoverable failure without pretending the queue is empty', async () => {
  session.authorized
    .mockRejectedValueOnce(new Error('无法读取待办，请重试。'))
    .mockResolvedValueOnce({ list: [] })
  wrapper = mount(DashboardRecords, { props: { kind: 'pending' } })
  await flushPromises()
  expect(wrapper.text()).toContain('无法读取待办')
  expect(wrapper.text()).not.toContain('当前没有需要你处理的申请')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === '重新加载')
    .trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('当前没有需要你处理的申请')
})
