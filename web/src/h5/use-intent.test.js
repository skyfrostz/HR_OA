import { afterEach, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSession } from '../oa/session'
import { useIntent } from './use-intent'

afterEach(() => vi.unstubAllGlobals())

function json(data, status = 200) {
  return new Response(JSON.stringify({ code: status < 400 ? 0 : 7, data }), {
    status,
    headers: { 'Content-Type': 'application/json' }
  })
}

it('retries the same key and payload through transport errors, then assigns a new key to a new intent', async () => {
  let rejectFirst
  const fetchMock = vi.fn()
    .mockResolvedValueOnce(json({
      token: 'fixture-token', expiresAt: new Date(Date.now() + 60_000).toISOString(),
      user: { id: 1 }, permissions: {}
    }))
    .mockImplementationOnce(() => new Promise((_, reject) => { rejectFirst = reject }))
    .mockResolvedValueOnce(json({ errorCode: 'FIXTURE_IN_FLIGHT' }, 409))
    .mockResolvedValueOnce(json({ id: 17 }, 201))
    .mockResolvedValueOnce(json({ id: 18 }, 201))
  vi.stubGlobal('fetch', fetchMock)

  setActivePinia(createPinia())
  const session = useSession()
  await session.login('fixture', 'fixture-password')
  try {
    const mutation = useIntent((cause) => ({
      retryable: cause.status === 0 || cause.errorCode === 'FIXTURE_IN_FLIGHT',
      message: cause.message
    }))

    const payload = { content: { note: '首次内容' } }
    const first = mutation.submit('/fixture', payload)
    expect(mutation.busy.value).toBe(true)
    expect(mutation.submit('/fixture', payload)).toBeNull()
    rejectFirst(new TypeError('fixture network error'))
    expect(await first).toBeNull()
    expect(mutation.retryable.value).toBe(true)
    payload.content.note = '之后修改'
    expect(mutation.submit('/fixture', payload)).toBeNull()
    expect(await mutation.retry()).toBeNull()
    expect(mutation.retryable.value).toBe(true)
    expect(await mutation.retry()).toEqual({ id: 17 })
    expect(mutation.retryable.value).toBe(false)
    expect(await mutation.submit('/fixture', payload)).toEqual({ id: 18 })

    const writes = fetchMock.mock.calls.slice(1)
    const firstKey = writes[0][1].headers.get('Idempotency-Key')
    expect(firstKey).toMatch(/^[0-9a-f]{32}$/)
    expect(writes.slice(0, 3).map(([url, options]) => ({
      url, key: options.headers.get('Idempotency-Key'), body: JSON.parse(options.body)
    }))).toEqual(Array(3).fill({
      url: '/api/oa/v1/fixture', key: firstKey, body: { content: { note: '首次内容' } }
    }))
    expect(writes[3][1].headers.get('Idempotency-Key')).not.toBe(firstKey)
    expect(JSON.parse(writes[3][1].body)).toEqual(payload)
  } finally {
    session.logout()
  }
})
