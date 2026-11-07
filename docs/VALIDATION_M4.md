# M4 第一批验证记录

日期：2026-11-07。升级基线为 `2d026e1bc36674bb741c901170ebe00f67dee858`。数据均为模拟资料，旧 M1–M3 数据和附件保留。本轮包含五项业务与前端显示优化，范围见 [M4_SCOPE](M4_SCOPE.md)。

当前状态：五项功能已通过源码、真实 MySQL、最终镜像 Chromium、迁移及成对备份恢复的本地验收。main 发布后的远端 CI 以对应提交的 [Actions 运行](https://github.com/skyfrostz/HR_OA/actions/workflows/ci.yaml) 为准。

## 分层验证

| 项目 | 结果 | 范围 |
| --- | --- | --- |
| Go 单元、竞态与构建 | PASS | `go test -mod=readonly -race -count=1 ./oa/...`；`go build -mod=readonly ./cmd/oa` |
| Swagger | PASS | 42 条路由解析，字段与状态契约核对 |
| Pure 测试、类型与生产构建 | PASS | 最终视觉修复后 7 项测试、生产入口类型检查及构建通过 |
| H5 测试与生产构建 | PASS | 17 项回归，保留原 H5 业务 |
| 升级前备份恢复 | PASS | 数据库/附件成对归档；原员工、合同、审批、部门、年假及附件字节恢复读回 |
| 隔离 MySQL 升级 | PASS | 原数据迁移保留；新增表/列与种子可重复启动 |
| 隔离账号与交接 | PASS | 创建、唯一绑定、角色限制、并发重置、启停/离职、旧 token 持续撤销；失败交接整笔回滚；当前/未来/新申请路由 |
| 隔离审计原子性 | PASS | 仅对合成合同设置触发器使审计 INSERT 失败，业务状态/版本一并回滚；移除触发器后同版本成功 |
| 组合 API / 浏览器业务 | PASS | 原数据与新接口、跨端竞争、列表同步、账号动作后连续编辑、合同附件与跨页提醒 |
| 最终视觉与构建绑定 | PASS | 最终组合镜像的 1440/390px 浅深色、键盘、焦点、减少动态效果及截图；源码哈希与镜像记录一同留存 |
| 升级后 M4 备份恢复 | PASS | 按备份绑定 API 镜像隔离恢复 M1–M4；账户状态、终止合同与审计、到期合同、原有和新增附件哈希一致 |
| GitHub CI | 发布触发 | 后端竞态/构建、Pure 测试/类型/构建、H5 测试/构建；远端结果见对应提交 Actions，不以本地结果替代 |

工资事件及其总数在 SQL 授权范围中排除；非管理员无自由原因文本。未来-only HANDOFF 时间线记录接手人且 taskId 为 null；分配前接手人不能查看申请，也没有提前通知。离职交接不改写提交时的原路径快照。

## 失败与复验记录

- 首轮隔离容器重启探测沿用临时旧端口而失败。修正为重启后重新读取 Docker 端口，实测从 63296 变为 63433；健康检查、旧令牌拒绝和新令牌可用复验通过。两轮日志分别保留在忽略的 `.local/m4-isolated-api-1.log`、`.local/m4-isolated-api-2.log`。
- 初次 Swagger 解析要求 `json.RawMessage` 明确 object 注解，修正后 42 路由通过。
- 总控源码复核发现 PC 分栏处理结果未同步左侧列表，以及账号动作后员工版本未同步；修复后 7 项 Pure 测试通过，最终浏览器验证左侧立即更新与账号后档案保存成功。
- 首轮浏览器实际发现桌面浅色表头 2.87:1、深色操作文字 3.61:1。未放宽阈值；已要求修复主题文字并增加抽屉文字检查。原生凸边分组/工具按钮也在总控截图复核后要求改为平整分段控件。最终组合镜像复验通过，浅色表头 12.13:1、深色操作文字 11.04:1；390px 深色抽屉标签 9.99:1、说明 12.29:1、返回旁文字 13.20:1。

## 复现与证据

- [API 验收](../tests/oa_m4_api.py)：真实 MySQL 下的权限、状态、并发和投影。
- [隔离运行脚本](../scripts/oa-m4-isolated.sh)：以升级前备份创建独立环境，允许限定于合成记录的回滚故障注入。
- [浏览器验收](../tests/oa_m4_browser.py)：真实 PC/H5 与组件交互。
- [成对备份恢复](../scripts/oa-backup.sh) / [恢复读回](../tests/oa_restore_verify.py)。
- [操作说明](M4_USER_GUIDE.md)、[API 契约](M4_API_CONTRACT.md)、[源码来源](M4_SOURCE_REUSE.md)、[前端设计](M4_UI_DESIGN.md)。

本机日志、备份、状态文件和凭据均不入库。最终原始截图见 `docs/assets/m4/`，摘要见 [截图哈希](M4_SCREENSHOTS_SHA256SUMS)。总控已查看最终名册、审批、账号和窄屏深色抽屉，视觉复核通过。

最后一轮独立 QA 日志：`.local/m4-browser-final.log`、`.local/m4-live-build-final.log`、`.local/m4-isolated-api-final.log`、`.local/m4-post-restore-final.log`。首次失败日志保留，未通过删除或放宽断言获得通过。

## 运行版本绑定

[1507 项构建输入与测试摘要](M4_SOURCE_SHA256SUMS) 覆盖后端、PC/H5、脚本、Docker/Compose 和部署配置，最终构建前后逐项一致。文档与截图独立归档，不充当运行镜像证据。

- API 镜像：`sha256:a717ad5817c2795203b4d6d90d368366d076d452b41bf3169ae6efa3502b5e73`
- Web 镜像：`sha256:c08c5961eeba606d2c951dc6b3e5d69ddf39f2f7c5b6a383a46f9a2b6c984df1`
- MySQL 镜像：`sha256:2952e3be7807f06fc18de50b3ea1a632d5c70d63482ff7d7376fe3aa8999babf`

最终 API 镜像与先前隔离测试镜像 ID 不同，容器内业务二进制 SHA-256 完全一致；最终成对恢复另用最终 API 镜像重新验证通过。临时隔离服务及网络已清理，主栈三个服务健康。

## 实际页面截图

以下均为本地真实页面，使用模拟资料；没有合成 UI 图或以旧截图冒充新版本。

| 页面 | 截图 |
| --- | --- |
| 组织树与员工表格 | [浅色 1440](assets/m4/m4-light-1440.png) · [深色 1440](assets/m4/m4-dark-1440.png) · [部门过滤](assets/m4/organization-tree-table-1440.png) |
| 窄屏名册 | [浅色 390](assets/m4/m4-light-390.png) · [深色 390](assets/m4/m4-dark-390.png) |
| 员工账号 | [桌面抽屉](assets/m4/employee-account-1440.png) · [深色 390](assets/m4/employee-drawer-dark-390.png) |
| 合同生命周期 | [桌面历史](assets/m4/contract-lifecycle-1440.png) · [提醒直达](assets/m4/contract-reminder-direct-1440.png) · [深色 390](assets/m4/contract-drawer-dark-390.png) |
| 业务变更历史 | [桌面](assets/m4/audit-history-1440.png) · [深色 390](assets/m4/audit-drawer-dark-390.png) |
| 审批中心 | [桌面分栏](assets/m4/pc-approvals-1440.png) · [浅色 390](assets/m4/pc-approval-light-390.png) · [深色 390](assets/m4/pc-approval-dark-390.png) · [H5 同一任务](assets/m4/h5-shared-task-390.png) |

## 证据边界

本轮是本地开发验收与源码发布；GitHub CI 不执行真实 MySQL、浏览器或部署。企业微信、阿里云 OSS、生产配置、正式审批规则、电子签章和返聘不属于本轮完成项。生产授权和交付条件仍见 [README](../README.md) 与 [外部集成状态](INTEGRATION_STATUS.md)。
