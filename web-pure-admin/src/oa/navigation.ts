import { shallowRef } from "vue";
import { defineStore } from "pinia";
import { store } from "@/store";

export const useEmployeeNavigation = defineStore("oa-employee-navigation", () => {
  const employeeId = shallowRef<number | null>(null);
  const contractId = shallowRef<number | null>(null);
  const requestId = shallowRef<number | null>(null);
  const approvalView = shallowRef<"mine" | "pending" | "history" | null>(null);
  const writeLocked = shallowRef(false);
  return { employeeId, contractId, requestId, approvalView, writeLocked };
});

export function employeeNavigation() {
  return useEmployeeNavigation(store);
}
