import { shallowRef } from 'vue'
import { useSession } from '../session'

function intentKey() {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
    byte.toString(16).padStart(2, '0')
  ).join('')
}

export function useIntent(errorFeedback) {
  const session = useSession()
  const busy = shallowRef(false)
  const error = shallowRef('')
  const failure = shallowRef(null)
  const retryable = shallowRef(false)
  let intent

  async function retry() {
    if (!intent || busy.value) return null
    busy.value = true
    error.value = ''
    failure.value = null
    try {
      const result = await session.authorized(intent.path, intent.options)
      intent = undefined
      retryable.value = false
      return result
    } catch (cause) {
      const feedback = errorFeedback(cause)
      failure.value = { status: cause.status, code: cause.errorCode }
      retryable.value = feedback.retryable
      error.value = feedback.message
      if (!retryable.value) intent = undefined
      return null
    } finally {
      busy.value = false
    }
  }

  function submit(path, data, method = 'POST') {
    if (intent || busy.value) return null
    intent = {
      path,
      options: { method, data: JSON.parse(JSON.stringify(data)), idempotencyKey: intentKey() }
    }
    return retry()
  }

  return { busy, error, failure, retryable, submit, retry }
}
