<script setup lang="ts">
import { useNav } from "@/layout/hooks/useNav";
import { useUserStoreHook } from "@/store/modules/user";
import { employeeNavigation } from "@/oa/navigation";
import { roleLabels } from "@/oa/format";
import LaySidebarBreadCrumb from "../lay-sidebar/components/SidebarBreadCrumb.vue";
import LaySidebarTopCollapse from "../lay-sidebar/components/SidebarTopCollapse.vue";
import Setting from "~icons/ri/settings-3-line";

const { device, logout, onPanel, pureApp, username, toggleSideBar } = useNav();
const user = useUserStoreHook();
const navigation = employeeNavigation();
</script>

<template>
  <header class="navbar bg-bg_color shadow-xs">
    <LaySidebarTopCollapse
      v-if="device === 'mobile'"
      class="hamburger-container"
      :is-active="pureApp.sidebar.opened"
      @toggleClick="toggleSideBar"
    />
    <LaySidebarBreadCrumb v-else class="breadcrumb-container" />
    <div class="vertical-header-right">
      <span class="oa-navbar-dev">本地开发</span>
      <a
        class="oa-navbar-link"
        :href="navigation.writeLocked ? undefined : './h5.html'"
        :aria-disabled="navigation.writeLocked"
      >移动审批</a>
      <span v-if="username" class="oa-navbar-user">{{ username }}</span>
      <el-tag v-if="user.account" size="small" effect="plain" type="info">
        {{ roleLabels[user.account.user.role] }}
      </el-tag>
      <el-button size="small" @click="logout">退出登录</el-button>
      <button class="oa-settings navbar-bg-hover" type="button" aria-label="界面设置" @click="onPanel">
        <Setting aria-hidden="true" />
      </button>
    </div>
  </header>
</template>

<style scoped>
.navbar { display: flex; align-items: center; justify-content: space-between; height: 48px; padding-inline: 12px; gap: 8px; }
.vertical-header-right { display: flex; flex-wrap: nowrap; align-items: center; gap: 10px; margin-inline-start: auto; min-width: 0; }
.oa-navbar-dev { color: var(--el-text-color-secondary); font-size: 12px; }
.oa-navbar-link { color: var(--el-color-primary); white-space: nowrap; }
.oa-navbar-link[aria-disabled="true"] { color: var(--el-text-color-secondary); }
.oa-navbar-user { max-width: 12rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.oa-settings { display: grid; place-items: center; min-width: 32px; min-height: 32px; border: 0; background: transparent; cursor: pointer; }
@media (max-width: 560px) {
  .vertical-header-right { gap: 6px; }
  .oa-navbar-dev, .oa-navbar-link[aria-disabled="true"] { color: var(--el-text-color-secondary); }
.oa-navbar-user { display: none; }
}
</style>
