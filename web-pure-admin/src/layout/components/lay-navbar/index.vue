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
      <span class="oa-navbar-dev">人事工作台</span>
      <a
        class="oa-navbar-link"
        :href="navigation.writeLocked ? undefined : './h5.html'"
        :aria-disabled="navigation.writeLocked"
        >移动审批</a
      >
      <span v-if="username" class="oa-navbar-user">{{ username }}</span>
      <el-tag
        v-if="user.account"
        class="oa-navbar-role"
        :title="roleLabels[user.account.user.role]"
        size="small"
        effect="plain"
        type="info"
      >
        {{ roleLabels[user.account.user.role] }}
      </el-tag>
      <el-button class="oa-navbar-logout" size="small" @click="logout"
        >退出登录</el-button
      >
      <button
        class="oa-settings navbar-bg-hover"
        type="button"
        aria-label="界面设置"
        @click="onPanel"
      >
        <Setting aria-hidden="true" />
      </button>
    </div>
  </header>
</template>

<style scoped>
.navbar {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  height: 48px;
  padding-inline: 12px;
}

.vertical-header-right {
  display: flex;
  flex-wrap: nowrap;
  gap: 10px;
  align-items: center;
  min-width: 0;
  margin-inline-start: auto;
}

.oa-navbar-dev {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.oa-navbar-link {
  color: var(--oa-accent-text, var(--el-color-primary));
  white-space: nowrap;
}

.oa-navbar-link[aria-disabled="true"] {
  color: var(--el-text-color-secondary);
}

.oa-navbar-user {
  max-width: 12rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.oa-navbar-role.el-tag {
  --el-tag-text-color: var(--el-text-color-regular);
}

.oa-settings {
  display: grid;
  place-items: center;
  min-width: 32px;
  min-height: 32px;
  cursor: pointer;
  background: transparent;
  border: 0;
}

@media (width <= 560px) {
  .navbar {
    gap: 4px;
    padding-inline: 8px;
  }

  .hamburger-container {
    display: grid;
    flex: 0 0 44px;
    place-items: center;
    width: 44px;
    height: 44px;
    padding: 0;
    margin: 0;
  }

  .vertical-header-right {
    flex: 1;
    gap: 4px;
    justify-content: flex-end;
  }

  .oa-navbar-dev,
  .oa-navbar-user {
    display: none;
  }

  .oa-navbar-role {
    flex: 0 1 auto;
    min-width: 0;
    padding-inline: 0;
    border: 0;
  }

  .oa-navbar-role :deep(.el-tag__content) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .oa-navbar-link {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    min-height: 44px;
  }

  .oa-navbar-logout {
    flex-shrink: 0;
    min-width: 44px;
    min-height: 44px;
    margin: 0;
  }

  .oa-settings {
    flex: 0 0 44px;
    width: 44px;
    min-width: 44px;
    min-height: 44px;
    padding: 0;
  }
}
</style>
