import { onScopeDispose, watch, type Ref } from "vue";
import { onBeforeRouteLeave, onBeforeRouteUpdate } from "vue-router";
import { employeeNavigation } from "./navigation";
import { useSession } from "./session";

export function useRouteWriteLock(locked: Readonly<Ref<boolean>>) {
  const session = useSession();
  const navigation = employeeNavigation();
  watch(locked, value => { navigation.writeLocked = value; }, { flush: "sync", immediate: true });
  const mayNavigate = () => !session.account || !locked.value;
  onBeforeRouteLeave(mayNavigate);
  onBeforeRouteUpdate(mayNavigate);
  onScopeDispose(() => { navigation.writeLocked = false; });
}
