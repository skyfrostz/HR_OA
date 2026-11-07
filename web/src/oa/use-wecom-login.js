import { onMounted, onScopeDispose, shallowRef } from 'vue'
import { request } from './api'
import { useSession } from './session'

export function useWeComLogin(authenticated, setBusy) {
  const session = useSession()
  const mode = shallowRef('loading')
  const error = shallowRef('')
  const busy = shallowRef(false)
  const controller = new AbortController()
  onScopeDispose(() => controller.abort())
  onMounted(async () => {
    const url = new URL(window.location.href)
    const callback = url.searchParams.get('wecom')
    if (callback !== null) {
      url.searchParams.delete('wecom')
      history.replaceState(history.state, '', url.pathname + url.search + url.hash)
    }
    if (callback === 'ready') {
      busy.value = true
      setBusy(true)
      try {
        await session.loginWeCom(controller.signal)
        if (!controller.signal.aborted) authenticated()
      } catch {
        if (!controller.signal.aborted) error.value = '企业微信登录未完成，请重新授权；如仍失败，请联系人事管理员核对账号绑定。'
      } finally {
        busy.value = false
        if (!controller.signal.aborted) setBusy(false)
      }
    } else if (callback === 'failed') {
      error.value = '企业微信授权失败或账号未绑定，请重新授权或联系人事管理员。'
    }
    if (controller.signal.aborted || session.account) return
    try {
      const status = await request('/integration/status', { signal: controller.signal })
      if (!controller.signal.aborted) mode.value = ['mock', 'configured', 'disabled'].includes(status.wecomLogin) ? status.wecomLogin : 'unavailable'
    } catch {
      if (!controller.signal.aborted) mode.value = 'unavailable'
    }
  })
  return { mode, error, busy }
}
