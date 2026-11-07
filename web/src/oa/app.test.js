import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import App from './app.vue'

const API = '/api/oa/v1'
const token = 'fixture-token'
const employee = {
  id: 11, employeeNo: 'SIM-011', name: '演示员工', department: '演示部门',
  birthDate: '1995-06-01', hireDate: '2021-07-01', age: 31, tenureYears: 5, salaryCents: 600009
}
const contract = {
  id: 21, employeeId: 11, contractNo: 'SIM-C21', startDate: '2025-01-01',
  endDate: '2026-12-31', status: 'SIGNED', previousContractId: null, files: []
}
const fixtureFile = {
  id: 31, originalName: 'fixture.pdf', size: 12,
  contentType: 'application/pdf', createdAt: '2026-11-06T00:00:00Z'
}

function json(data, status = 200) {
  return new Response(JSON.stringify({
    code: status < 400 ? 0 : 7,
    msg: status < 400 ? '' : '数据库内部信息不得直接展示',
    data
  }), { status, headers: { 'Content-Type': 'application/json' } })
}

function page(list) {
  return { list, total: list.length, page: 1, pageSize: 20 }
}

function button(wrapper, name) {
  const match = wrapper.findAll('button').find((node) => node.text() === name)
  expect(match, 'button ' + name).toBeDefined()
  return match
}

function mountApp() {
  return mount(App, { global: { plugins: [createPinia()] } })
}

async function login(wrapper) {
  await wrapper.get('#oa-username').setValue('demo-account')
  await wrapper.get('#oa-password').setValue('fixture-password')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}

async function chooseFile(wrapper, file) {
  const input = wrapper.get('#oa-attachment-input')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

function fixtureServer({ role = 'HR_ADMIN', employees = [employee], contracts = [contract], onRequest } = {}) {
  const fetchMock = vi.fn(async (url, options) => {
    const path = url.slice(API.length)
    const result = onRequest?.(path, options)
    if (result) return result

    if (path === '/auth/login') {
      return json({
        token, expiresAt: new Date(Date.now() + 60_000).toISOString(),
        user: { id: 3, username: 'demo-account', displayName: '演示账号', role, employeeId: 11 },
        permissions: { canWriteEmployees: role === 'HR_ADMIN', canReadSalary: role !== 'HR_VIEWER' }
      })
    }
    if (path === '/employees?page=1&pageSize=20') return json(page(employees))
    if (path === '/employees/11') return json(employees[0])
    if (path === '/employees/11/contracts?page=1&pageSize=20') return json(page(contracts))
    return json({ errorCode: 'NOT_FOUND' }, 404)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('OA management entry', () => {
  let wrapper

  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('keeps auth in memory, sends the OA header only to authorized requests and resets on reload', async () => {
    const fetchMock = fixtureServer()
    wrapper = mountApp()
    await login(wrapper)

    expect(wrapper.text()).toContain('人事管理员')
    expect(wrapper.text()).toContain('¥6,000.09')
    const [loginCall, listCall] = fetchMock.mock.calls
    expect(loginCall[0]).toBe(API + '/auth/login')
    expect(JSON.parse(loginCall[1].body)).toEqual({ username: 'demo-account', password: 'fixture-password' })
    expect(loginCall[1].headers.has('x-token')).toBe(false)
    expect(listCall[0]).toBe(API + '/employees?page=1&pageSize=20')
    expect(listCall[1].headers.get('x-token')).toBe(token)
    expect(listCall[1]).toMatchObject({ credentials: 'omit', cache: 'no-store' })
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
    expect(document.cookie).toBe('')

    await button(wrapper, '退出登录').trigger('click')
    expect(wrapper.text()).toContain('登录人事管理系统')
    await login(wrapper)
    wrapper.unmount()
    wrapper = mountApp()
    expect(wrapper.text()).toContain('登录人事管理系统')
    expect(wrapper.text()).not.toContain(employee.name)
  })

  it('hides salary and all write controls for the viewer', async () => {
    const { salaryCents, ...viewerRow } = employee
    fixtureServer({ role: 'HR_VIEWER', employees: [viewerRow] })
    wrapper = mountApp()
    await login(wrapper)

    expect(wrapper.text()).toContain('人事查阅员')
    expect(wrapper.text()).not.toContain('约定工资')
    expect(wrapper.text()).not.toContain('新增员工')

    await button(wrapper, '查看档案').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('劳动合同')
    expect(wrapper.text()).toContain('SIM-C21')
    for (const text of ['约定工资', '新建合同', '续约', '添加附件']) {
      expect(wrapper.findAll('button').map((node) => node.text())).not.toContain(text)
    }
    expect(wrapper.text()).not.toContain('约定工资')
  })

  it('shows the approver role and allows returning to the employee list from the sidebar', async () => {
    fixtureServer({ role: 'APPROVER' })
    wrapper = mountApp()
    await login(wrapper)
    expect(wrapper.text()).toContain('审批人')
    expect(wrapper.text()).toContain('查看本人的档案和劳动合同')
    expect(wrapper.get('nav[aria-label="主导航"] a').attributes('href')).toBe('./h5.html')
    expect(wrapper.text()).not.toContain('新增员工')

    await button(wrapper, '查看档案').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('员工详情')
    await button(wrapper, '员工档案').trigger('click')
    await flushPromises()
    expect(wrapper.get('table').text()).toContain(employee.name)
  })

  it('creates an employee with exact integer cents and fetches the returned employee detail', async () => {
    const fetchMock = fixtureServer({
      employees: [],
      contracts: [],
      onRequest(path, options) {
        if (path === '/employees' && options.method === 'POST') return json(employee, 201)
        if (path === '/employees/11') return json(employee)
      }
    })
    wrapper = mountApp()
    await login(wrapper)
    expect(wrapper.text()).toContain('暂无员工档案。')

    await button(wrapper, '新增员工').trigger('click')
    await wrapper.get('#employee-no').setValue(employee.employeeNo)
    await wrapper.get('#employee-name').setValue(employee.name)
    await wrapper.get('#employee-department').setValue(employee.department)
    await wrapper.get('#employee-birth').setValue(employee.birthDate)
    await wrapper.get('#employee-hire').setValue(employee.hireDate)
    await wrapper.get('#employee-salary').setValue('6000.009')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('最多保留两位小数')
    expect(fetchMock.mock.calls.filter(([, options]) => options.method === 'POST')).toHaveLength(1)
    await wrapper.get('#employee-salary').setValue('6000.09')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    const writes = fetchMock.mock.calls.filter(([url]) => url === API + '/employees')
    expect(writes).toHaveLength(1)
    expect(JSON.parse(writes[0][1].body)).toMatchObject({ salaryCents: 600009, employeeNo: 'SIM-011' })
    expect(wrapper.text()).toContain('员工详情')
    expect(wrapper.text()).toContain('¥6,000.09')
    expect(wrapper.text()).toContain('暂无劳动合同。')
  })

  it('creates a contract and renews it through the selected prior contract', async () => {
    const contracts = [contract]
    const fetchMock = fixtureServer({
      contracts,
      onRequest(path, options) {
        if (path === '/employees/11/contracts' && options.method === 'POST') {
          const created = {
            ...contract, id: 21 + contracts.length, previousContractId: null, ...JSON.parse(options.body)
          }
          contracts.push(created)
          return json(created, 201)
        }
      }
    })
    wrapper = mountApp()
    await login(wrapper)
    await button(wrapper, '查看档案').trigger('click')
    await flushPromises()

    await button(wrapper, '新建合同').trigger('click')
    await wrapper.get('#contract-number').setValue('SIM-C22')
    await wrapper.get('#contract-start').setValue('2027-01-01')
    await wrapper.get('#contract-end').setValue('2027-12-31')
    await wrapper.get('#contract-status').setValue('SIGNED')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('SIM-C22')

    const renewed = wrapper.findAll('article').find((item) => item.text().includes('SIM-C22'))
    await button(renewed, '续约').trigger('click')
    expect(wrapper.text()).toContain('上一份合同：SIM-C22')
    await wrapper.get('#contract-number').setValue('SIM-C23')
    await wrapper.get('#contract-start').setValue('2028-01-01')
    await wrapper.get('#contract-end').setValue('2028-12-31')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    const writes = fetchMock.mock.calls.filter(([url]) => url === API + '/employees/11/contracts')
    expect(writes.map(([, options]) => JSON.parse(options.body))).toEqual([
      { contractNo: 'SIM-C22', startDate: '2027-01-01', endDate: '2027-12-31', status: 'SIGNED' },
      { contractNo: 'SIM-C23', startDate: '2028-01-01', endDate: '2028-12-31', status: 'DRAFT', previousContractId: 22 }
    ])
    expect(wrapper.text()).toContain('续约来源：SIM-C22')
  })

  it('blocks duplicate and uncertain upload attempts until the ledger is refreshed', async () => {
    const contracts = [{ ...contract, files: [] }]
    let failUpload
    const fetchMock = fixtureServer({
      contracts,
      onRequest(path, options) {
        if (path !== '/contracts/21/files') return
        if (!failUpload) return new Promise((resolve, reject) => { failUpload = reject })
        contracts[0].files.push(fixtureFile)
        return json(fixtureFile, 201)
      }
    })
    const uploads = () => fetchMock.mock.calls.filter(([url]) => url === API + '/contracts/21/files')
    wrapper = mountApp()
    await login(wrapper)
    await button(wrapper, '查看档案').trigger('click')
    await flushPromises()
    await button(wrapper, '添加附件').trigger('click')

    await chooseFile(wrapper, new File(['fixture'], 'bad.txt'))
    expect(wrapper.text()).toContain('请选择 PDF、JPEG 或 PNG 文件。')
    await chooseFile(wrapper, new File([new Uint8Array(10 * 1024 * 1024 + 1)], 'big.pdf'))
    expect(wrapper.text()).toContain('超过 10 MiB')
    expect(uploads()).toHaveLength(0)

    const file = new File(['%PDF-fixture'], 'fixture.pdf', { type: 'application/pdf' })
    await chooseFile(wrapper, file)
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(uploads()).toHaveLength(1)
    expect(uploads()[0][1].body.get('file').name).toBe('fixture.pdf')
    expect(uploads()[0][1].headers.has('Content-Type')).toBe(false)
    failUpload(new TypeError('network fixture'))
    await flushPromises()
    expect(wrapper.text()).toContain('上传结果未知')
    expect(button(wrapper, '取消').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(uploads()).toHaveLength(1)

    await button(wrapper, '刷新并核对附件').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('已刷新')
    expect(button(wrapper, '上传附件').attributes('disabled')).toBeDefined()
    await chooseFile(wrapper, file)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(uploads()).toHaveLength(2)
    expect(wrapper.text()).toContain('附件已上传。')
    expect(wrapper.text()).toContain('fixture.pdf')
  })

  it('downloads a private attachment through a temporary URL and clears the account on 401', async () => {
    let status = 403
    const fetchMock = fixtureServer({
      role: 'EMPLOYEE',
      contracts: [{ ...contract, files: [fixtureFile] }],
      onRequest(path) {
        if (path !== '/files/31/download') return
        if (status !== 200) return json({ errorCode: 'DENIED' }, status)
        return new Response('%PDF-fixture', {
          headers: {
            'Content-Type': 'application/pdf',
            'Content-Disposition': 'attachment; filename="fixture.pdf"'
          }
        })
      }
    })
    wrapper = mountApp()
    await login(wrapper)
    await button(wrapper, '查看档案').trigger('click')
    await flushPromises()

    expect(wrapper.find('a[href*="/files/"]').exists()).toBe(false)
    await button(wrapper, '下载').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('当前账号没有执行此操作的权限。')
    expect(wrapper.text()).not.toContain('数据库内部信息不得直接展示')

    const download = fetchMock.mock.calls.find(([url]) => url === API + '/files/31/download')
    expect(download[1].headers.get('x-token')).toBe(token)
    expect(download[1]).toMatchObject({ credentials: 'omit', cache: 'no-store', redirect: 'error' })

    const createUrl = vi.fn(() => 'blob:fixture')
    const revokeUrl = vi.fn()
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = createUrl
      static revokeObjectURL = revokeUrl
    })
    const clickedFiles = []
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function () {
      clickedFiles.push({ href: this.getAttribute('href'), name: this.download })
    })
    status = 200
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await button(wrapper, '下载').trigger('click')
      await flushPromises()
      expect(createUrl.mock.calls[0][0]).toMatchObject({ type: 'application/pdf', size: 12 })
      expect(clickedFiles).toEqual([{ href: 'blob:fixture', name: 'fixture.pdf' }])
      expect(document.querySelector('a[href="blob:fixture"]')).toBeNull()
      vi.advanceTimersByTime(1000)
      expect(revokeUrl).toHaveBeenCalledWith('blob:fixture')
    } finally {
      vi.useRealTimers()
    }

    status = 401
    await button(wrapper, '下载').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('登录已失效，请重新登录。')
    expect(wrapper.text()).not.toContain('fixture.pdf')
  })

  it('shows loading and retry states and moves through server pages', async () => {
    let firstPage
    let calls = 0
    const secondEmployee = { ...employee, id: 12, name: '下一页员工' }
    fixtureServer({
      onRequest(path) {
        if (path === '/employees?page=1&pageSize=20') {
          if (++calls === 1) return new Promise((resolve) => { firstPage = resolve })
          return json({ ...page([employee]), total: 21 })
        }
        if (path === '/employees?page=2&pageSize=20') {
          return json({ ...page([secondEmployee]), total: 21, page: 2 })
        }
      }
    })
    wrapper = mountApp()
    await login(wrapper)
    expect(wrapper.text()).toContain('正在加载…')
    firstPage(json({}, 500))
    await flushPromises()
    expect(wrapper.text()).toContain('服务暂时不可用')
    await button(wrapper, '重新加载').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('第 1 / 2 页')
    await button(wrapper, '下一页').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('下一页员工')
    expect(wrapper.text()).not.toContain(employee.name)
    expect(button(wrapper, '下一页').attributes('disabled')).toBeDefined()
  })
})
