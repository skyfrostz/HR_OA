import { createRouter, createWebHashHistory, type RouteRecordRaw } from "vue-router";
import NProgress from "@/utils/progress";
import { useMultiTagsStoreHook } from "@/store/modules/multiTags";
import { usePermissionStoreHook } from "@/store/modules/permission";
import { useUserStoreHook } from "@/store/modules/user";
import { employeeNavigation } from "@/oa/navigation";
import oa from "./modules/oa";

const layout = oa.component;
const remaining: RouteRecordRaw[] = [
  {
    path: "/login",
    name: "Login",
    component: () => import("@/views/login/index.vue"),
    meta: { title: "登录", showLink: false, hiddenTag: true }
  },
  {
    path: "/redirect",
    component: layout,
    meta: { title: "加载", showLink: false },
    children: [{
      path: "/redirect/:path(.*)",
      name: "Redirect",
      component: () => import("@/layout/redirect.vue"),
      meta: { title: "加载", showLink: false, hiddenTag: true }
    }]
  },
  {
    path: "/error/403",
    name: "Forbidden",
    component: () => import("@/views/error/403.vue"),
    meta: { title: "访问受限", showLink: false }
  },
  {
    path: "/error/500",
    name: "ServerError",
    component: () => import("@/views/error/500.vue"),
    meta: { title: "服务异常", showLink: false }
  },
  {
    path: "/:pathMatch(.*)*",
    name: "PageNotFound",
    component: () => import("@/views/error/404.vue"),
    meta: { title: "页面不存在", showLink: false }
  }
];

export const constantMenus = [oa];
export const remainingPaths = remaining.map(route => route.path);
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [...constantMenus, ...remaining]
});

export function resetRouter() {
  const navigation = employeeNavigation();
  navigation.employeeId = null;
  navigation.writeLocked = false;
  useMultiTagsStoreHook().handleTags("equal", []);
  usePermissionStoreHook().clearAllCachePage();
}

router.beforeEach(to => {
  NProgress.start();
  const user = useUserStoreHook();
  if (!user.account) return to.name === "Login" ? true : { name: "Login", replace: true };
  if (to.name === "Login") return { name: "OaEmployees", replace: true };
  if (to.meta.oaPermission && !user.account.permissions[to.meta.oaPermission as keyof typeof user.account.permissions]) {
    return { name: "Forbidden", replace: true };
  }
  if (to.meta.roles?.length && !to.meta.roles.includes(user.account.user.role)) {
    return { name: "Forbidden", replace: true };
  }
  if (!usePermissionStoreHook().wholeMenus.length) {
    usePermissionStoreHook().handleWholeMenus([]);
  }
});

router.afterEach((to, _from, failure) => {
  NProgress.done();
  if (failure) return;
  document.title = to.meta.title ? String(to.meta.title) + " | HR OA" : "HR OA";
  if (to.meta.title && to.meta.showLink !== false && useUserStoreHook().account) {
    useMultiTagsStoreHook().handleTags("push", {
      path: to.path, name: to.name as string, meta: to.meta
    });
  }
});

export default router;
