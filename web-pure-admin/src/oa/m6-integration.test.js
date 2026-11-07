import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import WeComLogin from './wecom-login.vue'
import WeComBinding from './wecom-binding.vue'
import Integrations from './integrations.vue'
const { session, request } = vi.hoisted(() => ({ session: { account: null, loginWeCom: vi.fn(), authorized: vi.fn() }, request: vi.fn() }))
vi.mock('./session', () => ({ useSession: () => session }))
vi.mock('./api', () => ({ request }))
const navigation = {}
vi.mock('./navigation', () => ({ employeeNavigation: () => navigation }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn().mockResolvedValue(undefined) }) }))
let wrapper
beforeEach(() => { session.account = null; session.loginWeCom.mockReset(); session.authorized.mockReset(); request.mockReset(); history.replaceState({}, '', '/oa.html') })
afterEach(() => { wrapper?.unmount(); history.replaceState({}, '', '/oa.html'); vi.restoreAllMocks() })
it('clears callback flag before consuming a one-time ticket and does not replay on remount', async () => {
  let finish; session.loginWeCom.mockImplementation(() => new Promise(resolve => { finish = resolve })); request.mockResolvedValue({ wecomLogin: 'mock' })
  history.replaceState({}, '', '/oa.html?wecom=ready#/login'); wrapper = mount(WeComLogin)
  expect(location.search).toBe(''); expect(wrapper.emitted('busy')[0]).toEqual([true]); expect(session.loginWeCom).toHaveBeenCalledTimes(1)
  finish(); await flushPromises(); expect(wrapper.emitted('authenticated')).toHaveLength(1)
  wrapper.unmount(); wrapper = mount(WeComLogin); await flushPromises(); expect(session.loginWeCom).toHaveBeenCalledTimes(1)
  expect(wrapper.get('a').attributes('href')).toBe('/api/oa/v1/auth/wecom/start?entry=pc'); expect(wrapper.text()).toContain('模拟授权')
})
it('shows failed callback without an exchange request or a false success', async () => {
  request.mockResolvedValue({ wecomLogin: 'configured' }); history.replaceState({}, '', '/oa.html?wecom=failed')
  wrapper = mount(WeComLogin); await flushPromises(); expect(wrapper.text()).toContain('授权失败'); expect(session.loginWeCom).not.toHaveBeenCalled(); expect(wrapper.emitted('authenticated')).toBeUndefined()
})
it('keeps an uncertain identity binding locked until readback and sends exact case', async () => {
  request.mockResolvedValue({ wecomLogin: 'mock' }); let changed = false
  session.authorized.mockImplementation(async (path, options) => {
    if (options.method === 'PUT') { changed = true; throw new TypeError('lost') }
    return { employeeId: 11, version: changed ? 4 : 3, memberId: changed ? 'CaseSensitive.ID' : null }
  }); wrapper = mount(WeComBinding, { props: { employeeId: 11 } }); await flushPromises()
  const button = text => wrapper.findAll('button').find(item => item.text() === text)
  await button('修改绑定').trigger('click'); await wrapper.get('#oa-wecom-member').setValue('CaseSensitive.ID'); await wrapper.get('#oa-wecom-reason').setValue('核对企业成员'); await wrapper.get('form').trigger('submit'); await flushPromises()
  expect(button('取消').attributes('disabled')).toBeDefined(); expect(wrapper.emitted('editing').at(-1)).toEqual([true])
  expect(session.authorized.mock.calls.find(([, options]) => options.method === 'PUT')[1].data).toEqual({ version: 3, memberId: 'CaseSensitive.ID', reason: '核对企业成员' })
  await button('重新读取并核对').trigger('click'); await flushPromises(); expect(wrapper.text()).toContain('CaseSensitive.ID'); expect(wrapper.emitted('editing').at(-1)).toEqual([false])
})
it('labels configured and mock modes honestly and keeps uncertain delivery distinct', async () => {
  request.mockResolvedValue({ wecomLogin: 'configured', notifications: 'wecom-mock', attachmentStorage: 'oss-mock' })
  session.authorized.mockResolvedValue({ list: [{ id: 4, mode: 'wecom-mock', status: 'UNCERTAIN', attempts: 1, createdAt: '2026-11-07T00:00:00Z' }], total: 1, page: 1, pageSize: 20 })
  wrapper = mount(Integrations); await flushPromises(); expect(wrapper.text()).toContain('已配置 · 待企业验收'); expect(wrapper.text()).toContain('模拟 OSS'); expect(wrapper.text()).toContain('结果未知'); expect(wrapper.text()).toContain('不代表成员已收到或已读')
})

it('opens a contract notification through authorized contract lookup and memory-only navigation', async () => {
  const { default: Notifications } = await import('./notifications.vue')
  const { employeeNavigation } = await import('./navigation')
  session.authorized.mockImplementation(async path => path.startsWith('/notifications?') ? { list: [{ id: 8, kind: 'CONTRACT_EXPIRING', contractId: 21, requestId: 0, title: '合同即将到期', deliveryMode: 'mock', createdAt: '2026-11-07T00:00:00Z' }], total: 1, page: 1, pageSize: 20 } : { id: 21, employeeId: 11 })
  wrapper = mount(Notifications); await flushPromises()
  await wrapper.findAll('button').find(item => item.text() === '查看合同详情').trigger('click'); await flushPromises()
  expect(session.authorized.mock.calls.some(([path]) => path === '/contracts/21')).toBe(true)
  expect(session.authorized.mock.calls.some(([path]) => path === '/requests/0')).toBe(false)
  expect(employeeNavigation()).toMatchObject({ employeeId: 11, contractId: 21 })
})
