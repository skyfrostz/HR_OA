import { defineStore } from "pinia";
import { computed, shallowRef } from "vue";
import { store } from "@/store";
import { router, resetRouter } from "@/router";
import { OaError, request, type RequestOptions } from "@/oa/api";

export type OaAccount = {
  user: {
    id: number;
    username: string;
    displayName: string;
    role: "HR_ADMIN" | "HR_VIEWER" | "EMPLOYEE" | "APPROVER";
    employeeId: number | null;
  };
  permissions: {
    canWriteEmployees: boolean;
    canReadSalary: boolean;
    canSubmitRequests?: boolean;
    canApproveRequests?: boolean;
    canManageDepartments?: boolean;
    canManageAnnualLeave?: boolean;
    canImportEmployees?: boolean;
    canExportEmployees?: boolean;
  };
};

type LoginResult = OaAccount & { token: string; expiresAt: string };

export const useUserStore = defineStore("pure-user", () => {
  const account = shallowRef<OaAccount | null>(null);
  const notice = shallowRef("");
  const username = computed(() => account.value?.user.username || "");
  const nickname = computed(() => account.value?.user.displayName || "");
  const avatar = computed(() => "");
  const roles = computed(() => account.value ? [account.value.user.role] : []);
  const canWrite = computed(() => account.value?.permissions.canWriteEmployees === true);
  const canReadSalary = computed(() => account.value?.permissions.canReadSalary === true);
  let activeSession: { token: string; controller: AbortController } | undefined;
  let expiryTimer: ReturnType<typeof setTimeout> | undefined;

  function clearSession(message = "") {
    activeSession?.controller.abort();
    activeSession = undefined;
    clearTimeout(expiryTimer);
    account.value = null;
    notice.value = message;
  }

  function logOut(message = "") {
    clearSession(message);
    resetRouter();
    void router.replace({ name: "Login" });
  }

  async function login(name: string, password: string, signal?: AbortSignal) {
    notice.value = "";
    let result: LoginResult;
    try {
      result = await request<LoginResult>("/auth/login", {
        method: "POST", data: { username: name, password }, signal
      });
    } catch (error) {
      if (error instanceof OaError && error.status === 401) {
        throw new OaError(401, "账号或密码错误。");
      }
      throw error;
    }
    signal?.throwIfAborted();
    const remaining = Date.parse(result.expiresAt) - Date.now();
    if (!result.token || !result.user || !result.permissions || !(remaining > 0)) {
      throw new OaError(0, "登录响应异常，请重新登录。");
    }
    clearSession();
    activeSession = { token: result.token, controller: new AbortController() };
    account.value = { user: result.user, permissions: result.permissions };
    expiryTimer = setTimeout(() => logOut("登录已失效，请重新登录。"), Math.min(remaining, 2_147_483_647));
  }

  async function authorized<T = any>(path: string, options: RequestOptions = {}) {
    const current = activeSession;
    if (!current) throw new OaError(401);
    const sessionSignal = current.controller.signal;
    try {
      const data = await request<T>(path, { ...options, token: current.token, sessionSignal });
      sessionSignal.throwIfAborted();
      options.signal?.throwIfAborted();
      return data;
    } catch (error) {
      if (error instanceof OaError && error.status === 401 && current === activeSession) {
        logOut(error.message);
      }
      throw error;
    }
  }

  return { account, notice, username, nickname, avatar, roles, canWrite, canReadSalary, login, logOut, authorized };
});

export function useUserStoreHook() {
  return useUserStore(store);
}
