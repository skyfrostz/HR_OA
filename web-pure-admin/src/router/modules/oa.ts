import type { RouteRecordRaw } from "vue-router";
import User from "~icons/ri/team-line";
import Department from "~icons/ri/organization-chart";
import Excel from "~icons/ri/file-excel-2-line";
import Reminder from "~icons/ri/calendar-event-line";
import Approval from "~icons/ri/task-line";
import Dashboard from "~icons/ri/dashboard-line";
import Settings from "~icons/ri/settings-3-line";
import Leave from "~icons/ri/calendar-check-line";
import Alert from "~icons/ri/error-warning-line";
import Mobile from "~icons/ri/smartphone-line";

export default {
  path: "/",
  name: "OaHome",
  component: () => import("@/layout/index.vue"),
  redirect: "/employees",
  meta: { title: "人事管理", rank: 0 },
  children: [
    { path: "/dashboard", name: "OaDashboard", component: () => import("@/oa/dashboard.vue"), meta: { title: "工作台", icon: Dashboard, rank: 0 } },
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
      path: "/approvals",
      name: "OaApprovals",
      component: () => import("@/oa/approvals/workspace.vue"),
      meta: { title: "审批中心", icon: Approval, roles: ["EMPLOYEE", "APPROVER"], rank: 5 }
    },
    {
      path: "/mobile-approvals",
      name: "OaMobileApprovals",
      component: () => import("@/oa/mobile-approvals.vue"),
      meta: { title: "移动审批入口", icon: Mobile, rank: 6 }
    },
    { path: "/my-leave", name: "OaMyLeave", component: () => import("@/oa/personal-leave.vue"), meta: { title: "我的假期", icon: Leave, rank: 7 } },
    { path: "/workflow-settings", name: "OaWorkflowSettings", component: () => import("@/oa/workflow-settings.vue"), meta: { title: "请假与审批配置", icon: Settings, roles: ["HR_ADMIN"], rank: 8 } },
    { path: "/approved-leaves", name: "OaApprovedLeaves", component: () => import("@/oa/approved-leaves.vue"), meta: { title: "已批请假", icon: Leave, roles: ["HR_ADMIN"], rank: 9 } },
    { path: "/workflow-exceptions", name: "OaWorkflowExceptions", component: () => import("@/oa/workflow-exceptions.vue"), meta: { title: "审批异常", icon: Alert, roles: ["HR_ADMIN"], rank: 10 } },
  ]
} satisfies RouteRecordRaw;
