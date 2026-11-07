import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AttachmentPreview from './attachment-preview.vue'
import AttachmentForm from './attachment-form.vue'
import AttachmentClassification from './attachment-classification.vue'
const { session } = vi.hoisted(() => ({ session: { canWrite: true, account: { user: { role: 'HR_ADMIN' } }, authorized: vi.fn() } }))
vi.mock('./session', () => ({ useSession: () => session }))
let wrapper
const file = { id: 21, originalName: '测试合同.png', classification: 'STANDARD', version: 2 }
const global = { stubs: { ElDialog: { template: '<section><slot/><slot name="footer"/></section>' }, VuePdfEmbed: true } }
beforeEach(() => { session.authorized.mockReset(); URL.createObjectURL = vi.fn(() => 'blob:fixture'); URL.revokeObjectURL = vi.fn() })
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks() })
it('previews authorized image bytes and revokes the object URL on close', async () => {
  session.authorized.mockResolvedValue(new Blob(['image'], { type: 'image/png' }))
  wrapper = mount(AttachmentPreview, { props: { file }, global }); await flushPromises()
  expect(session.authorized.mock.calls[0][0]).toBe('/files/21/preview'); expect(wrapper.find('img').attributes('src')).toBe('blob:fixture')
  expect(wrapper.find('iframe, object, embed').exists()).toBe(false)
  const signal = session.authorized.mock.calls[0][1].signal; wrapper.unmount(); expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:fixture'); expect(signal.aborted).toBe(true)
})
it.each([403, 404])('does not display a denied preview (%i)', async status => {
  session.authorized.mockRejectedValue({ status, message: '权限不足' })
  wrapper = mount(AttachmentPreview, { props: { file }, global }); await flushPromises()
  expect(wrapper.text()).toContain('权限不足'); expect(wrapper.find('img').exists()).toBe(false); expect(URL.createObjectURL).not.toHaveBeenCalled()
})
it('rejects HTML and SVG bytes instead of embedding a document', async () => {
  session.authorized.mockResolvedValue(new Blob(['<svg/>'], { type: 'image/svg+xml' }))
  wrapper = mount(AttachmentPreview, { props: { file }, global }); await flushPromises()
  expect(wrapper.text()).toContain('无法显示此附件'); expect(wrapper.find('img, iframe, object').exists()).toBe(false)
})
it('uploads new attachments as RESTRICTED by default', async () => {
  session.authorized.mockResolvedValue({ id: 21 })
  wrapper = mount(AttachmentForm, { props: { contractId: 11, refresh: vi.fn() } })
  const upload = wrapper.get('input[type=file]'); Object.defineProperty(upload.element, 'files', { value: [new File(['data'], 'contract.pdf')] }); await upload.trigger('change')
  await wrapper.get('form').trigger('submit'); await flushPromises()
  expect(session.authorized.mock.calls[0][1].fileFields).toEqual({ classification: 'RESTRICTED' })
})
it('locks a classification with unknown write outcome until a successful readback', async () => {
  session.authorized.mockRejectedValue(new TypeError('lost response')); const refresh = vi.fn().mockResolvedValue(true)
  wrapper = mount(AttachmentClassification, { props: { file, refresh } })
  await wrapper.get('select').setValue('RESTRICTED'); await wrapper.get('input').setValue('原件含工资'); await wrapper.get('form').trigger('submit'); await flushPromises()
  expect(session.authorized.mock.calls[0][1].data).toEqual({ classification: 'RESTRICTED', version: 2, reason: '原件含工资' })
  expect(wrapper.findAll('button').find(button => button.text() === '取消').attributes('disabled')).toBeDefined()
  await wrapper.findAll('button').find(button => button.text() === '重新读取并核对').trigger('click'); await flushPromises()
  expect(wrapper.emitted('reconciled')).toHaveLength(1); expect(session.authorized).toHaveBeenCalledTimes(1)
})
