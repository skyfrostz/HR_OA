import type { RouteRecordRaw } from "vue-router";
import User from "~icons/ri/team-line";
import Department from "~icons/ri/organization-chart";
import Excel from "~icons/ri/file-excel-2-line";
import Reminder from "~icons/ri/calendar-event-line";
import Mobile from "~icons/ri/smartphone-line";

export default {
  path: "/",
  name: "OaHome",
  component: () => import("@/layout/index.vue"),
  redirect: "/employees",
  meta: { title: "人事管理", rank: 0 },
  children: [
    {
      path: "/employees",
      name: "OaEmployees",
      component: () => import("@/oa/employee-workspace.vue"),
      meta: { title: "员工档案", icon: User, fixedTag: true, rank: 1 }
    },
    {
      path: "/departments",
      name: "OaDepartments",
      component: () => import("@/oa/departments.vue"),
      meta: { title: "部门管理", icon: Department, roles: ["HR_ADMIN", "HR_VIEWER"], rank: 2 }
    },
    {
      path: "/employee-import",
      name: "OaEmployeeImport",
      component: () => import("@/oa/employee-import.vue"),
      meta: { title: "Excel 导入", icon: Excel, roles: ["HR_ADMIN"], oaPermission: "canImportEmployees", rank: 3 }
    },
    {
      path: "/contract-reminders",
      name: "OaContractReminders",
      component: () => import("@/oa/contract-reminders.vue"),
      meta: { title: "合同到期提醒", icon: Reminder, rank: 4 }
    },
    {
      path: "/mobile-approvals",
      name: "OaMobileApprovals",
      component: () => import("@/oa/mobile-approvals.vue"),
      meta: { title: "移动审批入口", icon: Mobile, rank: 5 }
    }
  ]
} satisfies RouteRecordRaw;
