import { onScopeDispose, shallowRef } from 'vue'
import { useSession } from './session'

export function useLoginForm() {
  const session = useSession()
  const username = shallowRef('')
  const password = shallowRef('')
  const busy = shallowRef(false)
  const error = shallowRef('')
  const controller = new AbortController()

  onScopeDispose(() => controller.abort())

  async function submit() {
    if (busy.value || !username.value.trim() || !password.value) return
    busy.value = true
    error.value = ''
    const enteredPassword = password.value
    password.value = ''
    try {
      await session.login(username.value.trim(), enteredPassword, controller.signal)
    } catch (cause) {
      if (!controller.signal.aborted) error.value = cause.message
    } finally {
      busy.value = false
    }
  }

  return { session, username, password, busy, error, submit }
}
