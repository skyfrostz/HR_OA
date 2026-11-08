import { onScopeDispose } from "vue";

const lockClass = "oa-contract-scroll-locked";
const leases = new WeakMap<
  HTMLElement,
  { count: number; preexisting: boolean }
>();

// Own a separate lock so Element Plus nested dialogs retain ownership of theirs.
// Release on the actual drawer exit, without the upstream delayed-unlock race.
export function useContractScrollLock() {
  let target: HTMLElement | null = null;
  function acquire() {
    if (target || typeof document === "undefined") return;
    target = document.body;
    let lease = leases.get(target);
    if (!lease) {
      lease = { count: 0, preexisting: target.classList.contains(lockClass) };
      leases.set(target, lease);
    }
    lease.count++;
    target.classList.add(lockClass);
  }
  function release() {
    if (!target) return;
    const lease = leases.get(target);
    if (lease && --lease.count === 0) {
      if (!lease.preexisting) target.classList.remove(lockClass);
      leases.delete(target);
    }
    target = null;
  }
  onScopeDispose(release);
  return { acquire, release };
}
