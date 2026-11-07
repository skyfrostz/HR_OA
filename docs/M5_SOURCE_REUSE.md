# M5 源码复用记录

本轮延续已固定的 Pure、OA PC/H5 与 Go/GORM 模块。没有将参考仓库整体加入运行时，也没有新增前端或 Go 依赖；实际复用为现有组件和业务基础模块的适配。

## 实际适配

| 来源 | 本轮接入 | 适配内容 |
| --- | --- | --- |
| 已导入 Pure / Element Plus | `web-pure-admin/src/oa/dashboard.vue` 与新增配置/异常/假期页面 | 沿用真实布局、菜单、主题、按钮与 Statistic 组件；统计接授权 API，无演示数字或动画计数 |
| OA PC/H5 审批 | 两端 `leave-fields.vue`、`request-editor.vue`、`request-actions.vue`、`request-detail.vue` | 扩展原表单、申请状态和时间线；复用 `useIntent`，新增预览失效、幂等撤回/催办/撤销与未来步骤交接显示 |
| OA 读取与版本编辑 | `useRemote`、`useVersionedWrite`、分页和路由写锁 | 新增假种、日历、规则、异常与流水页沿用迟到读取防护、未知写入核对和版本冲突反馈 |
| OA 后端 | `server/oa/service/leave*.go`、`workflow_rules.go`、`workflow_exceptions.go`、`request_actions.go` | 复用既有身份和会话世代、工作流守卫、MySQL 事务、幂等意图与模拟 outbox；半天占用、流水和规则匹配由本仓库实现 |
| M4 账号交接与审计 | `workflow_handoff.go`、`audit.go` | 使离职交接识别启用规则及未来覆盖；配置审计限定 HR_ADMIN，原规则快照保留 |

上述本仓库模块的复用不改变 GVA、Pure 或其他依赖的原许可证。继续保留上游版权、MIT 与 BSL 文本。新增功能不是引入 Frappe、Flowable 或其他完整 HR 系统。

## 已核对的参考点

- Pure `src/views/components/statistic.vue`：已有 Element Plus Statistic、ReCol 使用示例。工作台只接真实授权计数，不复制演示数字、倒计时或持续数字动画。保留原 MIT。
- Frappe HRMS 固定 `8816102577c92979df49c20a1807b8c7ab948eb3` 的 `hrms/hr/doctype/leave_application/leave_application.py`：已核对日期/余额/重叠验证、提交与取消调用台账的边界。其数据模型依赖 Frappe，GPL-3.0，本轮仅作业务对照，不复制代码。OA 使用自己的半天槽、事务、预占和保留的冲回流水，不删除历史流水。
- 芋道固定 `a33cdfd565edae4f866901bf6105c7087b0697c4`：顺序流程配置与人员选择可参考呈现；本轮采用受限规则表单，不引入完整 Flowable 设计器或第二套用户系统。

更多固定来源与许可记录见 [前期评估](NEXT_PHASE_PLAN.md) 与 [M4 来源](M4_SOURCE_REUSE.md)。

参考文件摘要：

- `statistic.vue`：`d085ee8f26773938d5a51b30203bfa5b10be91eebef40adff38b7c2c656c37b3`
- `leave_application.py`：`7c1a4cce344ee7bcd3bf2c893aea4eab675abfba9d7949259932e2c83a84bb37`
