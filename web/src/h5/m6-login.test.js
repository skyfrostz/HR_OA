import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { useSession } from '../oa/session'
import WeComLogin from './wecom-login.vue'
import RecordList from './record-list.vue'
let wrapper, session
const json = (data, status = 200) => new Response(JSON.stringify({ code: status === 200 ? 0 : 7, data }), { status, headers: { 'Content-Type': 'application/json' } })
const account = { token: 'memory-ticket', expiresAt: new Date(Date.now() + 600000).toISOString(), user: { role: 'EMPLOYEE', employeeId: 11 }, permissions: {} }
beforeEach(() => { setActivePinia(createPinia()); session = useSession(); history.replaceState({}, '', '/h5.html'); localStorage.clear(); sessionStorage.clear() })
afterEach(async () => { wrapper?.unmount(); session.logout(); await flushPromises(); history.replaceState({}, '', '/h5.html'); vi.restoreAllMocks(); vi.unstubAllGlobals() })
it('exchanges a ticket with cookies into memory, clears URL, then keeps bearer requests cookie-free', async () => {
  const fetchMock = vi.fn(async path => json(path.endsWith('/exchange') ? account : { ok: true })); vi.stubGlobal('fetch', fetchMock)
  history.replaceState({}, '', '/h5.html?wecom=ready'); wrapper = mount(WeComLogin); await flushPromises()
  expect(session.account.user.employeeId).toBe(11); expect(location.search).toBe(''); expect(fetchMock.mock.calls[0][1].credentials).toBe('same-origin')
  await session.authorized('/dashboard'); expect(fetchMock.mock.calls[1][1].headers.get('x-token')).toBe('memory-ticket'); expect(fetchMock.mock.calls[1][1].credentials).toBe('omit')
  expect(localStorage.length).toBe(0); expect(sessionStorage.length).toBe(0)
})
it('does not create a session when a ticket is expired or already consumed', async () => {
  vi.stubGlobal('fetch', vi.fn(async path => path.endsWith('/exchange') ? json({ errorCode: 'UNAUTHENTICATED' }, 401) : json({ wecomLogin: 'mock' })))
  history.replaceState({}, '', '/h5.html?wecom=ready'); wrapper = mount(WeComLogin); await flushPromises()
  expect(session.account).toBeNull(); expect(wrapper.text()).toContain('登录未完成'); expect(wrapper.get('a').attributes('href')).toBe('/api/oa/v1/auth/wecom/start?entry=h5')
})
it('distinguishes provider-accepted inbox records from local simulation', async () => {
  vi.stubGlobal('fetch', vi.fn(async path => path.endsWith('/auth/login') ? json(account) : json({ list: [
    { id: 1, requestId: 1, title: '本地通知', deliveryMode: 'mock' },
    { id: 2, requestId: 2, title: '模拟企业通知', deliveryMode: 'wecom-mock' },
    { id: 3, requestId: 3, title: '企业通知', deliveryMode: 'wecom' }
  ], total: 3, page: 1, pageSize: 20 })))
  await session.login('fixture', 'fixture'); wrapper = mount(RecordList, { props: { view: 'notifications' } }); await flushPromises()
  expect(wrapper.text()).toContain('模拟 API 已接受'); expect(wrapper.text()).toContain('企业 API 已接受'); expect(wrapper.text()).toContain('不代表已送达或已读')
})

it('routes contract reminders to the existing PC entry instead of request zero', async () => {
  const fetchMock = vi.fn(async path => path.endsWith('/auth/login') ? json(account) : json({ list: [{ id: 8, requestId: 0, contractId: 21, kind: 'CONTRACT_EXPIRING', title: '合同即将到期', deliveryMode: 'mock' }], total: 1, page: 1, pageSize: 20 }))
  vi.stubGlobal('fetch', fetchMock); await session.login('fixture', 'fixture'); wrapper = mount(RecordList, { props: { view: 'notifications' } }); await flushPromises()
  expect(wrapper.get('article a').attributes('href')).toBe('./oa.html#/contract-reminders'); expect(wrapper.find('article button').exists()).toBe(false); expect(wrapper.emitted('open')).toBeUndefined(); expect(fetchMock.mock.calls.some(([path]) => path.endsWith('/requests/0'))).toBe(false)
})
