# M5 验证记录

日期：2026-11-07。升级基线 `7e8a338e4eed592e28a4031ba0bf5b582ed5aa83`。范围见 [M5_SCOPE](M5_SCOPE.md)，资料均为模拟数据。M5 已完成本地源码、真实 MySQL、最终组合镜像浏览器和成对备份恢复验收。远端 CI 按发布提交检查，见 [Actions](https://github.com/skyfrostz/HR_OA/actions/workflows/ci.yaml)。

## 分层结果

| 项目 | 状态 | 已验证范围 |
| --- | --- | --- |
| 升级前 M4 成对备份恢复 | PASS | 保存原源码和 API/Web/MySQL 镜像，原镜像隔离恢复 M1–M4 数据及两代附件 |
| Go 包、race、构建 | PASS | OA 全包测试和竞态检查，cmd/oa 构建；新增半天、规则交集、旧哈希和未来交接定向回归 |
| Swagger | PASS | 58 条路由解析 |
| Pure 测试、类型和构建 | PASS | 20 项交互回归、实际生产入口类型检查和构建 |
| H5 测试和构建 | PASS | 24 项交互回归及生产构建 |
| 真实 MySQL 增量升级 | PASS | 旧申请、账号、审计、部门、台账与附件读回；旧 LEAVE 成功意图重放，新旧字段请求不能绕过新版校验 |
| 余额与事务 | PASS | 工作半天、跨年、日历例外、预览版本、占用重叠、额度与人工维护竞争、释放与单次冲回；流水失败整笔回滚 |
| 规则、权限与交接 | PASS | 优先级、金额闭区间、冲突、规则快照、工作台计数与授权列表、当前/未来异常、步骤竞争、延迟通知/授权、配置审计范围 |
| 最终组合镜像浏览器 | PASS | PC/H5 业务流程、浅深色、390/1440px、焦点、对比度与 16 张真实截图；M4 关键浏览器回归 |
| 升级后 M5 成对备份恢复 | PASS | 新快照、撤销状态、额度流水和全部既有资料，使用最终 API 镜像隔离读回 |
| GitHub CI | 发布后自动执行 | 对应提交的后端、Pure、H5 工作流；以远端 Actions 结果为准，本地验收不替代 CI |

## 回归与修复

- 总控复核发现请假输入 A→B→A 可复活旧预览，已改为输入变动立即失效；迟到请求不能重新激活。原提交重试遇配置过期、重叠或余额不足也会清除预览并要求重新计算。PC/H5 均有组件测试。
- 年份筛选使用输入草稿与已应用条件，避免逐键请求和加载时打断输入。
- 异常交接以当前任务 ID 为锚点，必须同时校验任务版本和列表所示异常步骤。原步骤恢复后，即使任务版本未变，旧操作也不能误修后一个步骤。历史事件记录具体步骤和原/新审批人，未来步骤尚未产生任务时不提前授予详情访问或通知。
- 首轮隔离测试辅助 SQL 直接解析 BLOB JSON 失败，已显式转换 UTF-8；第二轮无效审批人测试原期望 400，按契约的 409 ROUTE_UNAVAILABLE 校正。保留原日志，未删减业务断言。
- 隔离环境故意使扣减流水 INSERT 失败，接口返回 500，申请、任务、额度及 outbox 均保持原状态。移除故障后沿用原幂等键重试成功。
- 额外实测闰日、非额度类型和假种版本过期；正常 OFFBOARD/DISABLE 接口分别覆盖启用规则与异常覆盖中的未来审批人，原路径保留，实际分配前没有提前授权或通知。
- 真实浏览器前两次失败分别来自日期标签匹配了左侧筛选及右侧表单、隐式选择框标签包含选项文本。按真实表单/步骤范围修正定位后完整业务流程通过；未改变业务结果断言。
- 总控截图复核发现年度余额浅色 Pure 表头过淡，QA 实测 3.08:1。定向复用主题正文色和浅填充背景，不更改 390px 卡片布局；修复后重新构建并测量，最终结果见分层结果及截图记录。

## 可复现入口

- [M5 API 验收](../tests/oa_m5_api.py) 与 [隔离运行脚本](../scripts/oa-m4-isolated.sh)：设置 `OA_QA_MILESTONE=5` 使用 M4 基线归档；仅在独立恢复库运行故障注入。
- [M5 浏览器验收](../tests/oa_m5_browser.py)：真实 PC/H5 与合成业务记录。
- [成对备份脚本](../scripts/oa-backup.sh) 与 [恢复读回](../tests/oa_restore_verify.py)。
- [操作说明](M5_USER_GUIDE.md)、[API 契约](M5_API_CONTRACT.md)、[界面设计](M5_UI_DESIGN.md)、[源码复用](M5_SOURCE_REUSE.md)。

本机初期日志为 `.local/m5-isolated-build-{1,2}.log`、`.local/m5-isolated-api-{1,2,3,4,5,6,7}.log`；最终后端冻结候选通过，后续两轮补齐闰日和正常离职交接场景。原失败与复验记录均保留。数据库、凭据、状态文件及备份不提交。

最终组合证据为 `.local/m5-live-build-2.log`、`.local/m5-live-browser-final.log`、`.local/m5-balance-contrast-final.log`、`.local/m5-post-restore-final.log`。表头浅色 12.13:1、深色 12.41:1；PC 390px 深色余额标签 12.29:1，H5 深色余额标签 7.56:1。隔离容器和网络已清理，活动栈三个服务健康。

## 运行版本绑定

[1538 项源码、构建与测试摘要](M5_SOURCE_SHA256SUMS) 与最终组合构建输入逐项一致；[截图摘要](M5_SCREENSHOTS_SHA256SUMS) 对应原始 PNG，未编辑或合成。

- API 镜像：`sha256:c465196dce5f54165b0ff522049fb0b53c580d0fdedd284c26212729f983efc3`
- Web 镜像：`sha256:dc29b3350f3819b0965b6c6198a3ceb5364fa10487c2e2a37ae28d211ce399a5`
- MySQL 镜像：`sha256:2952e3be7807f06fc18de50b3ea1a632d5c70d63482ff7d7376fe3aa8999babf`

主栈业务二进制与隔离 MySQL 验收候选逐字节一致，最终备份恢复另使用最终 API 镜像验证。文档和截图独立归档，不作为运行二进制的替代证据。未把历史 M2 新建旧表单的整套脚本标为当前回归；本轮明确验证旧记录、附件及旧成功幂等兼容。

## 实际页面截图

以下均来自最终组合镜像的真实页面和模拟资料；合成 API 的前端预检图及修复前截图未用于交付。总控已查看工作台、请假预览、余额流水和 390px 深色余额，视觉复核通过。

| 页面 | 截图 |
| --- | --- |
| 工作台 | [员工 1440](assets/m5/employee-dashboard-1440.png) · [HR 1440](assets/m5/admin-dashboard-1440.png) · [HR 深色 390](assets/m5/admin-dashboard-dark-390.png) |
| 请假 | [服务端计算预览](assets/m5/pc-leave-preview-1440.png) · [提交快照](assets/m5/pc-leave-snapshot-1440.png) |
| 跨端处理 | [H5 催办](assets/m5/h5-leave-reminder-390.png) · [H5 当前任务](assets/m5/h5-leave-task-390.png) |
| 余额流水 | [PC 1440](assets/m5/pc-leave-ledger-1440.png) · [PC 深色 390](assets/m5/pc-balance-dark-390.png) · [H5 390](assets/m5/h5-leave-ledger-390.png) |
| HR 操作 | [已批请假](assets/m5/approved-leave-1440.png) · [异常检查](assets/m5/workflow-exceptions-1440.png) |
| 配置 | [类型](assets/m5/leave-types-1440.png) · [工作日历](assets/m5/work-calendar-1440.png) · [审批规则](assets/m5/approval-rules-1440.png) · [深色日历 390](assets/m5/work-calendar-dark-390.png) |

## 证据边界

本轮验收为本地工程验证与源码发布。GitHub CI 检查源码测试与构建，不执行真实 MySQL、浏览器或部署。工作日历默认周一至周五，不含法定假期；额度和顺序规则可配置，正式业务制度尚未确认。企业微信、阿里云 OSS、生产部署、电子签章和复杂分支/会签不在本轮完成范围内。
