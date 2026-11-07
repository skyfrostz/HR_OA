import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'
import { OaError, request } from './api'

export const useSession = defineStore('oa-session', () => {
  const account = shallowRef(null)
  const notice = shallowRef('')
  const canWrite = computed(() => account.value?.permissions.canWriteEmployees === true)
  const canReadSalary = computed(() => account.value?.permissions.canReadSalary === true)
  let activeSession
  let expiryTimer

  function logout(message = '') {
    activeSession?.controller.abort()
    activeSession = undefined
    clearTimeout(expiryTimer)
    account.value = null
    notice.value = message
  }

  async function login(username, password, signal) {
    notice.value = ''
    let result
    try {
      result = await request('/auth/login', { method: 'POST', data: { username, password }, signal })
    } catch (error) {
      if (error.status === 401) throw new OaError(401, '账号或密码错误。')
      throw error
    }
    signal?.throwIfAborted()
    const remaining = Date.parse(result.expiresAt) - Date.now()
    if (!result.token || !result.user || !result.permissions || !(remaining > 0)) {
      throw new OaError(0, '登录响应异常，请重新登录。')
    }
    logout()
    activeSession = { token: result.token, controller: new AbortController() }
    account.value = { user: result.user, permissions: result.permissions }
    expiryTimer = setTimeout(() => logout('登录已失效，请重新登录。'), Math.min(remaining, 2_147_483_647))
  }

  async function authorized(path, options = {}) {
    const current = activeSession
    if (!current) throw new OaError(401)
    const sessionSignal = current.controller.signal
    try {
      const data = await request(path, { ...options, token: current.token, sessionSignal })
      sessionSignal.throwIfAborted()
      options.signal?.throwIfAborted()
      return data
    } catch (error) {
      if (error.status === 401 && current === activeSession) logout(error.message)
      throw error
    }
  }

  return { account, notice, canWrite, canReadSalary, login, logout, authorized }
})
