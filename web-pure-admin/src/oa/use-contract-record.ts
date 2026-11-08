import { onScopeDispose, shallowRef, watch } from "vue";
import type { RequestOptions } from "./api";
import type { Page } from "./business";

export type ContractRecord = {
  id: number;
  employeeId: number;
  contractNo: string;
  status: "DRAFT" | "SIGNED" | "EXPIRED" | "TERMINATED";
  startDate: string;
  endDate: string;
  signedDate?: string;
  terminatedDate?: string;
  previousContractId?: number | null;
  version: number;
  files?: Record<string, unknown>[];
};
export type ContractTaskState = {
  editing: boolean;
  dirty: boolean;
  busy: boolean;
};
export type ContractChange = {
  contractId: number;
  employeeId: number;
  kind: string;
  contract?: ContractRecord;
};
type Reader = <T>(path: string, options?: RequestOptions) => Promise<T>;

export function useContractRecord(
  target: () => { contractId: number; employeeId: number; active: boolean },
  read: Reader
) {
  const record = shallowRef<ContractRecord | null>(null);
  const loading = shallowRef(false);
  const error = shallowRef("");
  let controller: AbortController | undefined;
  let sequence = 0;

  function cancel() {
    sequence++;
    controller?.abort();
    controller = undefined;
    loading.value = false;
  }

  async function load() {
    cancel();
    record.value = null;
    error.value = "";
    const { contractId, employeeId, active } = target();
    if (!active) return null;
    const current = sequence;
    const request = new AbortController();
    controller = request;
    loading.value = true;
    try {
      const value = await read<ContractRecord>("/contracts/" + contractId, {
        signal: request.signal
      });
      if (current !== sequence || request.signal.aborted) return null;
      if (value.id !== contractId || value.employeeId !== employeeId) {
        error.value = "这份合同与当前员工不匹配，未显示合同内容。";
        return null;
      }
      record.value = value;
      return value;
    } catch (cause) {
      if (current !== sequence || request.signal.aborted) return null;
      const status = (cause as { status?: number }).status;
      error.value =
        status === 403
          ? "当前账号无权查看这份合同。"
          : status === 404
            ? "这份合同不存在或已不可访问。"
            : status
              ? "合同读取失败，请稍后重新加载。"
              : "无法读取合同，请检查网络后重试。";
      return null;
    } finally {
      if (current === sequence) {
        loading.value = false;
        controller = undefined;
      }
    }
  }

  watch(
    () => {
      const value = target();
      return [value.contractId, value.employeeId, value.active] as const;
    },
    (value, previous) => {
      if (!value[2]) {
        cancel();
        if (
          previous &&
          (value[0] !== previous[0] || value[1] !== previous[1])
        ) {
          record.value = null;
          error.value = "";
        }
        return;
      }
      void load();
    },
    { immediate: true, flush: "sync" }
  );
  onScopeDispose(cancel);
  return { record, loading, error, load, cancel };
}

// Only called by an explicit unknown-write reconciliation, never initial open.
export async function findCreatedContract(
  read: Reader,
  input: {
    employeeId: number;
    contractNo: string;
    startDate: string;
    endDate: string;
    previousContractId?: number;
  },
  signal?: AbortSignal
) {
  for (let page = 1; ; page++) {
    if (signal?.aborted) return null;
    const result = await read<Page<ContractRecord>>(
      `/employees/${input.employeeId}/contracts?page=${page}&pageSize=100`,
      { signal }
    );
    if (signal?.aborted) return null;
    const match = result.list.find(row => row.contractNo === input.contractNo);
    if (match) {
      const current = await read<ContractRecord>("/contracts/" + match.id, {
        signal
      });
      if (signal?.aborted) return null;
      if (
        current.id !== match.id ||
        current.employeeId !== input.employeeId ||
        current.contractNo !== input.contractNo ||
        current.startDate !== input.startDate ||
        current.endDate !== input.endDate ||
        (current.previousContractId ?? null) !==
          (input.previousContractId ?? null)
      )
        throw new Error(
          "找到同编号合同，但员工、续约来源或合同期限不一致，尚未确认本次结果。"
        );
      return current;
    }
    if (!result.list.length || result.page * result.pageSize >= result.total)
      return null;
  }
}
