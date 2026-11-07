# Pure Admin 与 M3 本地验证

日期：2026-11-07。环境为现有本地 Compose、MySQL 8.4.8 和 Chromium；仅使用模拟资料。本报告不代表企业微信、阿里云 OSS 或生产部署验收。

## 交付范围

PC 使用官方 Pure Admin v7.0.0 提交 `b8177a202dceb1412d3bf56f3d15483dc1606c9c` 的真实布局、菜单、标签、主题和路由体系。完整原始源码导入 `web-pure-admin/`，原 636 个文件全部保留，MIT 文本不变。原始哈希与集成改动清单分别见 `PURE_ADMIN_UPSTREAM_SHA256SUMS`、`PURE_ADMIN_CHANGES.md`。

新增部门和员工版本编辑、手工年度年假、Excel 模板/预览/确认/导出、合同到期队列及成对备份/隔离恢复。PC `/oa.html` 与 Vant H5 `/h5.html` 共用原账号、数据库和私有附件卷，使用独立静态资源前缀。M1/M2 历史证据保留。

## 验证结果

| 检查 | 结果与证据范围 |
| --- | --- |
| Go 单元与竞态检查、入口构建 | PASS。`go test -mod=readonly -count=1 ./oa/...`、`go test -mod=readonly -race -count=1 ./oa/...` 和 `go build -mod=readonly ./cmd/oa`。单元竞态检测不替代下列真实 MySQL 并发测试。 |
| Pure 前端组件测试、生产入口类型检查及构建 | PASS。固定 pnpm 10.32.1，锁文件安装，实际 Pure 布局配合模拟 API 测试。保留但未注册的上游演示源码全量类型检查有 14 条旧接口诊断；生产入口可达的类型检查通过，旧 JavaScript 沿用 `checkJs: false`。 |
| 原前端回归 | PASS，16 项。保留 Vant H5 的独立依赖及入口。 |
| 组合镜像和健康检查 | PASS。Docker 构建分别生成 Pure PC 和原 H5，静态根只发布 Pure 的 PC 入口；Web 健康检查覆盖两入口及 API。 |
| MySQL 升级与版本写入 | PASS。原员工字段、身份绑定和合同附件保持；部门回填、更名、员工及年假并发编辑仅一个版本胜出。 |
| Excel API | PASS。逐行错误、文件边界、公式/外链拒绝；同预览并发确认及原结果重放；创建者和活动管理员校验；过期与实时冲突拒绝且无部分员工写入；导出工资列及员工行权限。 |
| 到期提醒 API | PASS。上海日历边界、今日/逾期/将到期、有效续约排除、排序及角色范围；读取不修改合同状态。 |
| M3 真实浏览器业务链 | PASS。新增部门、下载模板、错误预览、丢失确认响应后重试同一预览、员工编辑、半天年假、提醒跳转和查阅员导出。 |
| Pure 四角色及写入保护 | PASS，最终 Web 镜像复验通过，包含被拒绝导航后的侧栏高亮断言。实际菜单和标签未丢失待确认附件状态；1440/390 布局、工资隔离、退出/刷新及敏感存储检查通过。 |
| H5 真实审批回归 | PASS。提交双击保护、丢失响应原键重试、转交、逐级通过、申请人读回和模拟通知。 |
| 旧记录持久化 | PASS。原 M1 记录和附件字节一致；新样本经历容器重建后读回。 |
| M3 成对备份及隔离恢复 | PASS。暂停 API 写入时保存 SQL/私有文件，恢复到独立临时 MySQL 与私有目录。真实 API 读回原员工、合同、附件字节、M2 审批/重提记录以及 M3 部门/年假的 ID、版本和值。 |
| 备份负向验证 | PASS。校验和不符在创建恢复容器前拒绝；故意读回失败后清理隔离项目；归档目录 0700、文件 0600，未删除活动卷。 |

## 修复与证据

最初备份脚本的 Python 版本兼容问题已修复；首轮 Pure 浏览器脚本的 Playwright 回调包装问题已修复，随后业务链通过。浏览器另发现写入锁拒绝导航时侧栏高亮仍变化，前端已修复并补回归，QA 在最终 Web 镜像上重新通过 Pure、H5 和 M3 浏览器场景。保留失败日志，不将首轮失败改记为通过。

核心日志位于忽略目录 `.local/`：`m3-qa-api-1.log`、`m3-qa-browser-final.log`、`pure-qa-run-1.log`、`pure-qa-sidebar-before-fix.log`、`pure-qa-browser-final.log`、`m3-qa-backup.log` 和 `m3-qa-restore.log`。截图位于 `tests/results/pure/browser/` 与 `tests/results/m3/browser/`。详细角色报告在相邻 `hr-oa-coordination/` 的 `M3_BACKEND_REPORT.md`、`PURE_FRONTEND_REPORT.md`、`PURE_QA_REPORT.md` 与 `M3_QA_REPORT.md`。

总控独立核对 QA 冻结清单 795 项，全部匹配；随后仅更新交付说明与验收状态文档。最终文件绑定见 `PURE_M3_SOURCE_SHA256SUMS`，该清单不包含密码、业务数据、构建输出或自身。官方原始导入清单继续保留，不会以修改后的源码哈希覆盖。

## 外部与生产边界

企业微信和阿里云 OSS 已由用户选定，尚无客户应用、桶、凭据和部署域名，真实登录、消息与云附件联调未执行。现有通知为模拟站内投递，合同提醒为页面查询；年假是人工台账。正式表单/审批策略仍待客户确认；未实施公网部署、异地备份或生产容灾验收。具体接入说明见 `INTEGRATION_STATUS.md`。
