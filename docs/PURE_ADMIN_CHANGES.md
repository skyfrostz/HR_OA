# Pure Admin integration changes

Compared with the pinned v7.0.0 import recorded in `PURE_ADMIN_UPSTREAM_SHA256SUMS`. Updated for M5 on 2026-11-07; dependency/build directories are excluded. This inventory does not replace final runtime acceptance hashes.

Original files: 636; modified: 27; missing: 0; new source/config/test files: 56. The MIT license is unchanged.

## Modified upstream files

- `web-pure-admin/package.json`
- `web-pure-admin/pnpm-lock.yaml`
- `web-pure-admin/public/platform-config.json`
- `web-pure-admin/src/App.vue`
- `web-pure-admin/src/components/ReIcon/src/hooks.ts`
- `web-pure-admin/src/components/RePureTableBar/src/bar.tsx`
- `web-pure-admin/src/config/index.ts`
- `web-pure-admin/src/layout/components/lay-navbar/index.vue`
- `web-pure-admin/src/layout/components/lay-setting/index.vue`
- `web-pure-admin/src/layout/components/lay-sidebar/NavVertical.vue`
- `web-pure-admin/src/layout/components/lay-sidebar/components/SidebarLinkItem.vue`
- `web-pure-admin/src/layout/components/lay-tag/index.scss`
- `web-pure-admin/src/layout/components/lay-tag/index.vue`
- `web-pure-admin/src/layout/hooks/useDataThemeChange.ts`
- `web-pure-admin/src/layout/hooks/useNav.ts`
- `web-pure-admin/src/layout/index.vue`
- `web-pure-admin/src/layout/types.ts`
- `web-pure-admin/src/main.ts`
- `web-pure-admin/src/router/index.ts`
- `web-pure-admin/src/router/utils.ts`
- `web-pure-admin/src/store/modules/multiTags.ts`
- `web-pure-admin/src/store/modules/user.ts`
- `web-pure-admin/src/style/tailwind.css`
- `web-pure-admin/src/utils/auth.ts`
- `web-pure-admin/src/utils/responsive.ts`
- `web-pure-admin/src/views/login/index.vue`
- `web-pure-admin/vite.config.ts`

## Added files

- `web-pure-admin/oa.html`
- `web-pure-admin/src/bootstrap.ts`
- `web-pure-admin/src/oa/annual-leave.vue`
- `web-pure-admin/src/oa/api.ts`
- `web-pure-admin/src/oa/approvals/approval.js`
- `web-pure-admin/src/oa/approvals/leave-fields.vue`
- `web-pure-admin/src/oa/approvals/record-list.vue`
- `web-pure-admin/src/oa/approvals/request-actions.vue`
- `web-pure-admin/src/oa/approvals/request-detail.vue`
- `web-pure-admin/src/oa/approvals/request-editor.vue`
- `web-pure-admin/src/oa/approvals/task-actions.vue`
- `web-pure-admin/src/oa/approvals/use-intent.js`
- `web-pure-admin/src/oa/approvals/workspace.css`
- `web-pure-admin/src/oa/approvals/workspace.vue`
- `web-pure-admin/src/oa/approved-leaves.vue`
- `web-pure-admin/src/oa/attachment-form.vue`
- `web-pure-admin/src/oa/audit-history.vue`
- `web-pure-admin/src/oa/business.test.ts`
- `web-pure-admin/src/oa/business.ts`
- `web-pure-admin/src/oa/contract-files.vue`
- `web-pure-admin/src/oa/contract-form.vue`
- `web-pure-admin/src/oa/contract-ledger.vue`
- `web-pure-admin/src/oa/contract-reminders.vue`
- `web-pure-admin/src/oa/dashboard.vue`
- `web-pure-admin/src/oa/departments.vue`
- `web-pure-admin/src/oa/employee-account.vue`
- `web-pure-admin/src/oa/employee-detail.vue`
- `web-pure-admin/src/oa/employee-form.vue`
- `web-pure-admin/src/oa/employee-import.vue`
- `web-pure-admin/src/oa/employee-workspace.vue`
- `web-pure-admin/src/oa/format.js`
- `web-pure-admin/src/oa/leave-ledger.vue`
- `web-pure-admin/src/oa/login-form.vue`
- `web-pure-admin/src/oa/m5-admin.test.js`
- `web-pure-admin/src/oa/m5-approvals.test.js`
- `web-pure-admin/src/oa/mobile-approvals.vue`
- `web-pure-admin/src/oa/navigation.ts`
- `web-pure-admin/src/oa/oa-mark.svg`
- `web-pure-admin/src/oa/page-nav.vue`
- `web-pure-admin/src/oa/personal-leave.vue`
- `web-pure-admin/src/oa/pure-app.test.js`
- `web-pure-admin/src/oa/remote-state.vue`
- `web-pure-admin/src/oa/session.js`
- `web-pure-admin/src/oa/settings/approval-rules.vue`
- `web-pure-admin/src/oa/settings/leave-types.vue`
- `web-pure-admin/src/oa/settings/work-calendar.vue`
- `web-pure-admin/src/oa/style.css`
- `web-pure-admin/src/oa/tokens.css`
- `web-pure-admin/src/oa/use-login-form.js`
- `web-pure-admin/src/oa/use-remote.js`
- `web-pure-admin/src/oa/use-route-write-lock.ts`
- `web-pure-admin/src/oa/use-versioned-write.js`
- `web-pure-admin/src/oa/workflow-exceptions.vue`
- `web-pure-admin/src/oa/workflow-settings.vue`
- `web-pure-admin/src/router/modules/oa.ts`
- `web-pure-admin/tsconfig.oa.json`

## Integration scope

The upstream layout, theme, router, navigation and components remain the PC foundation. OA supplies its own in-memory session and role-scoped API. M5 adds workbench statistics, leave calculation and ledger, versioned configuration, request actions and exception handling. No new frontend dependency was added for M5. See `M5_SOURCE_REUSE.md` for source adaptation and `VALIDATION_M5.md` for verification.
