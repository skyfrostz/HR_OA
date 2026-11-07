# M6 验证记录

日期：2026-11-07。基线 `d2f2fa1026eae443940f0b51a864292c23eab4a8`。用户确认企业微信应用和 OSS 私有 Bucket 尚未准备，要求先完成实现与模拟联调。本轮只使用合成人员资料、本地 MySQL 和模拟提供商。既定能力见 [功能验收索引](FEATURE_ACCEPTANCE_MATRIX.md)，接口见 [M6 API](M6_API_CONTRACT.md)。

## 分层结果

| 验证层 | 状态 | 范围及证据 |
| --- | --- | --- |
| 后端测试、race、构建 | PASS | `go test -mod=readonly -race -count=1 ./oa/... ./cmd/oa`；OA 二进制构建；Swagger 70 条注解路由解析 |
| Pure 交互、类型与构建 | PASS | 首轮完整 36 项通过，合同通知增量后定向 15 项通过，共覆盖 37 个不同案例；生产入口类型与构建通过。该计数不声称本地重新跑过完整 37 项 |
| H5 交互与构建 | PASS | 完整 28 项交互测试及生产构建 |
| 配置与 CLI | PASS | `--check-config` 无数据库/提供商连接，help/version 无需凭据；错误不回显秘密；禁网容器可完成纯配置检查；两处公开主机 TLS 证书链探针通过，不代表客户网络验收 |
| 全新 MySQL 与模拟提供商 | PASS | 最终候选在独立源码副本构建，9 组验收通过；随机配置、独立数据库、loopback 端口，无旧 `.env` 或历史测试状态依赖；结束清理隔离资源 |
| WeCom 身份与通知 | PASS（模拟） | PC/H5 授权、一次性 state/ticket、防重放、显式绑定、解绑/离职失效、未绑定与外部身份拒绝、token 更新、限流/明确失败/未知结果；广播收件人拒绝且零外发请求 |
| 合同提醒调度 | PASS | 当日/30天入队，31天和逾期排除；续约、终止、离职抑制，排队后发送前再次核验；重启和多次扫描不重复 |
| OSS 与迁移 | PASS（模拟） | 私有桶预检、SDK 签名请求头形态、PUT 权限/加密/防覆盖头、损坏及上传失败；dry-run 不写，失败返回非零不切换，同字节重试成功，异字节冲突拒绝，本地原件保留 |
| 附件与 Excel | PASS | 真实 MySQL/API 鉴权、敏感级元数据隔离、版本冲突、预览字节；错误工作簿为文字单元格并保留行号，预览过期/清理、历史结果及跨账号隔离 |
| 旧数据升级 | PASS | 最终主栈升级后 M1–M5 员工、合同、审批、审计、年假/额度及附件字节读回 |
| 真实浏览器 | PASS | PC/H5 模拟授权、真实附件上传、中文 PDF、危险内容无外发请求、故障恢复、图片资源释放、Excel 键盘筛选/下载/结果；M4 生命周期和 M5 跨端请假回归通过；最终通知标签定向复验通过 |
| 成对备份恢复 | PASS | 最终 API 镜像隔离恢复 M1–M6，包含敏感附件字节/权限和持久化导入结果 |
| 可移植恢复 | PASS | 不依赖历史测试 JSON 和原登录凭据，API 健康、25 张表记录数、49 份附件字节摘要匹配；API/MySQL 镜像与数据清单随包保存 |
| GitHub CI | 按发布提交执行 | [main Actions](https://github.com/skyfrostz/HR_OA/actions/workflows/ci.yaml?query=branch%3Amain) 运行 backend、pure-admin、h5、mysql-integration 四项任务；结果以同一发布提交为准，本地 PASS 不替代远端结果 |
| 客户 WeCom/OSS、生产部署、异机灾备 | NOT RUN | 用户明确尚未准备外部资源；不把模拟通过写成云端已上线 |

OSS 模拟器检查请求语义与签名头形态，未实现真实 OSS HMAC 鉴权；正式接入仍需云端验收。企业 API 接受不代表送达或已读。生产 Compose 的语法检查不代表生产安全或业务验收。

## 发现与修复

- fresh MySQL 首次迁移因 GORM 使用持锁连接发生 panic；锁改由独立 SQL 会话持有，迁移走正常连接池。随后修复版本账本的零时间值，显式写入 UTC。保留首次失败及复验日志。
- 单人企业通知曾允许 `@all` 选择器；绑定及发送两处均拒绝广播、多收件人和大小写/空白变形，独立回归确认零 HTTP 请求。
- 离职员工合同仍可能产生 HR 站内提醒；入队和发送前均补在职检查。已入队事件也不能绕过。
- 授权三个步骤原共用一个 IP 限流桶；拆成各入口独立桶，保留各自限流阈值。
- 迁移 CLI 标准输出混入 Gin 调试路由，影响 JSON 解析；启动时切换 release 模式。全路径迁移故障和重试后严格 JSON 解析通过。
- 浏览器实图发现集成状态标签与导入成功提示文字过淡；只调整对应主题文字。状态标签浅/深对比度为 6.11:1 / 12.29:1，成功提示为 12.06:1 / 13.11:1。
- 最终合同通知实图暴露独立通知卡标签尚未复用上述样式；PC 复用同一状态样式，H5 使用当前主题文字和边框。修复后 PC 浅/深为 6.11:1 / 12.29:1，H5 为 5.11:1 / 8.74:1，合同及普通通知均检查。只改样式，双端构建通过。
- Pure 旧测试夹具缺少预览有效期，且仍预期已提交状态重发 POST；按真实有效期和结果读取契约修正，另保留未知结果重试测试。根 CLI Python 3.9 探针的超时异常类型修正后重跑通过。
- 最终主栈人员较多，浏览器首轮只查当前列表页，未命中种子员工；验收脚本改用真实工号搜索再定位，保留权限断言，不改变业务数据或访问政策。
- 重跑浏览器时同名图片命中两个合同，脚本改为限定本次合同。M4 历史脚本的附件精确文本选择器受新增分级标签影响，改为检查同一附件的授权预览控制后，生命周期定向回归通过；审批与组织部分原样通过。M6 另验证普通/敏感级权限，不声称未修改的旧整套脚本通过。

## 复现与证据

全新 checkout 可运行 `bash scripts/oa-ci-integration.sh --mock-providers`。基础模式不要求浏览器；本机额外设置 `OA_CI_BROWSER_PYTHON` 为已安装锁定浏览器依赖的 Python，可执行真实 API 的 Chromium 场景。不要对真实业务库运行故障注入测试。

- [独立集成入口](../scripts/oa-ci-integration.sh)、[基础 API](../tests/oa_ci_integration.py)、[模拟服务](../tests/oa_provider_mock.py)、[提供商与迁移验收](../tests/oa_m6_providers.py)、[浏览器验收](../tests/oa_m6_browser.py)。
- [可移植备份脚本](../scripts/oa-backup.sh)、[独立恢复清单](../scripts/oa-restore-inventory.py)、[配置与迁移说明](M6_OPERATIONS.md)。
- 本地最终 fresh 日志 `.local/m6-ci-fresh-mock-final.log`；历史失败 `.local/m6-ci-integration-{4,5,6}.log`，初轮浏览器 `.local/m6-ci-mock-3.log`；最终主栈升级 `.local/m6-live-migration-1.log`，浏览器 `.local/m6-live-browser-final-2.log` / `.local/m6-mock-browser-final.log`，M4/M5 回归 `.local/m6-m4-browser-lifecycle-final.log` / `.local/m6-m5-browser-regression.log`，成对恢复 `.local/m6-post-restore-final.log`。凭据、数据库、私有附件、日志和状态文件不分发。

前端合成 API 预检图只证明视觉布局，不作为应用后端验收。最终交付截图须来自真实 OA HTTP API，所有人员与合同均为测试资料。

可移植恢复日志为 `.local/m6-post-portable-restore.log`。当前本地备份脚本检测到远端 OSS 附件时明确拒绝生成不完整备份；OSS 对象须另设配套备份。可移植包不包含 Web 镜像、真实客户密钥或云端对象，不能称为云灾备或异机切换验收。

## 实际页面截图

PNG 原图直接归档，未经编辑或合成。主栈与独立模拟栈均使用真实 OA API；模拟栈的企业微信和 OSS 端点由本地测试服务提供。总控已查看导入结果、中文 PDF、成员绑定、授权后 H5、窄屏深色集成与合同通知。

| 页面 | 原图 |
| --- | --- |
| Excel | [错误行筛选](assets/m6/excel-errors-1440.png) · [提交与历史结果](assets/m6/excel-result-history-1440.png) |
| 附件 | [默认敏感级](assets/m6/restricted-file-1440.png) · [中文 PDF](assets/m6/pdf-preview-1440.png) · [查阅员范围](assets/m6/viewer-file-scope-1440.png) |
| 企业身份 | [显式成员绑定](assets/m6/wecom-binding-1440.png) · [PC 登录](assets/m6/wecom-login-1440.png) · [H5 登录](assets/m6/wecom-login-390.png) · [PC 授权后](assets/m6/wecom-signed-in-1440.png) · [H5 授权后](assets/m6/wecom-signed-in-390.png) |
| 模拟集成状态 | [浅色1440](assets/m6/mock-integration-light-1440.png) · [深色1440](assets/m6/mock-integration-dark-1440.png) · [深色390](assets/m6/mock-integration-dark-390.png) |
| 默认未接入状态 | [浅色1440](assets/m6/integration-light-1440.png) · [深色1440](assets/m6/integration-dark-1440.png) · [深色390](assets/m6/integration-dark-390.png) |
| 合同定时通知 | [PC 站内通知](assets/m6/contract-notice-pc-1440.png) · [H5 深色通知](assets/m6/contract-notice-h5-390.png) |

## 运行版本绑定

[1,586 项运行、构建、部署与测试源码摘要](M6_SOURCE_SHA256SUMS)经 QA 和总控独立逐项校验一致；[18 张归档截图摘要](M6_SCREENSHOTS_SHA256SUMS)对应原始 PNG。发布说明与截图不混入运行源码清单。

- API：`sha256:6eebb86444ab221c7ed0740c42ed46ff3ae61a4d1f0ca418b187fdb283f6b1b6`
- Web：`sha256:68063d1801449eb8c930a98f90d935e3db0daf288c568cdc870c0df74c881e17`
- MySQL：`sha256:2952e3be7807f06fc18de50b3ea1a632d5c70d63482ff7d7376fe3aa8999babf`

主栈 API 与 fresh mock 验收副本的业务二进制逐字节一致，与最终恢复包 API 镜像一致。末次仅修改通知标签样式，重建 Web 并定向重拍 PC/H5 通知；其他截图中的组件未受影响，保留已通过的真实业务图。最终隔离 Web 与主栈 Web 文件系统层摘要一致。定向日志为 `.local/m6-mock-notice-final.log`，PC 浅色 6.11:1、H5 深色 8.74:1。可移植清单篡改副本被拒绝，原归档未修改。主栈三个服务健康，验收隔离项目均已清理。
