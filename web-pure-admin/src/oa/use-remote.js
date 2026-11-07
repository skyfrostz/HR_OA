import { onScopeDispose, shallowRef } from 'vue'

export function useRemote(fetchData) {
  const result = shallowRef(null)
  const loading = shallowRef(false)
  const error = shallowRef('')
  let pending

  async function load() {
    pending?.abort()
    const current = new AbortController()
    pending = current
    loading.value = true
    error.value = ''
    try {
      const data = await fetchData(current.signal)
      if (current.signal.aborted) return false
      result.value = data
      return true
    } catch (cause) {
      if (!current.signal.aborted) {
        result.value = null
        error.value = cause.message
      }
      return false
    } finally {
      if (pending === current) {
        loading.value = false
        pending = undefined
      }
    }
  }

  onScopeDispose(() => pending?.abort())
  return { result, loading, error, load }
}
