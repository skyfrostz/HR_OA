import { useUserStoreHook } from "@/store/modules/user";

/** Visibility helper only. The OA API enforces all permissions. */
export function hasPerms(value: string | string[]): boolean {
  const permissions = useUserStoreHook().account?.permissions;
  if (!permissions || !value) return false;
  return (Array.isArray(value) ? value : [value]).every(
    key => Object.hasOwn(permissions, key) && permissions[key as keyof typeof permissions] === true
  );
}
