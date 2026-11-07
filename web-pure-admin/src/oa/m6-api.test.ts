import { afterEach, expect, it, vi } from 'vitest'
import { request } from './api'
afterEach(() => vi.unstubAllGlobals())
it('sends same-origin cookies only for the one-time exchange and carries file classification as multipart', async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ code: 0, data: { ok: true } }), { headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', fetchMock)
  await request('/auth/wecom/exchange', { method: 'POST', data: {} })
  await request('/integration/status')
  await request('/contracts/11/files', { method: 'POST', file: new File(['bytes'], 'file.pdf'), fileFields: { classification: 'RESTRICTED' }, token: 'fixture' })
  expect(fetchMock.mock.calls[0][1].credentials).toBe('same-origin'); expect(fetchMock.mock.calls[1][1].credentials).toBe('omit'); expect(fetchMock.mock.calls[2][1].credentials).toBe('omit')
  expect(fetchMock.mock.calls[2][1].body.get('classification')).toBe('RESTRICTED'); expect(fetchMock.mock.calls[2][1].headers.has('Content-Type')).toBe(false)
})
