# M6 API 契约

2026-11-07。继承 M1–M5 的 /api/oa/v1、x-token、JSON 信封、分页、逐请求权限与 no-store。以下 URL 均在 API 前缀下。模拟和生产配置状态不代表真实企业验收。

## 配置和状态

Go bootstrap.Config 增 `Production bool`、`PublicOrigin string`、`TrustedProxies []string`、`WeCom bootstrap.WeComConfig`、`OSS bootstrap.OSSConfig`、`AttachmentWriteBackend string`。由 cmd 读取环境，bootstrap 校验。可信代理必须明确 IP/CIDR，默认只使用实际连接地址。生产需 HTTPS 公共 origin、MySQL TLS（tls=true）、强 JWT 密钥，禁用 DevSeed、MockNotifyFail 和 provider mock。首次管理员初始化只适用于没有身份记录的库。MySQL 版本账本记录完成的 M6 迁移，拒绝比当前二进制更新的 schema。
- WeComConfig: `Mode,CorpID,Secret,MockAPIBase,MockAuthorizeURL string`、`AgentID int64`、`DeliverNotifications bool`。Mode 为空/disabled、mock、wecom。公共回调固定为 PublicOrigin + /api/oa/v1/auth/wecom/callback。mock 的 API/授权端点只支持明确本地地址，生产禁用。
- OSSConfig: `Enabled,Mock,UseCName bool`、`Region,Bucket,Endpoint,Prefix,AccessKeyID,AccessKeySecret,SecurityToken string`。附件写后端为空/local 或 oss；读按每个文件的元数据。真实端点必须 HTTPS 官方 OSS 或明确启用的 CNAME。启动检查桶 ACL 为 private、阻止公共访问已开启且未设置版本控制（保证禁止覆盖生效）。mock 明确本地地址、开发模式，使用路径风格和同样的预检。SDK 不生成外部公开链接。
- `GET /integration/status`（公开）→ `{wecomLogin:"disabled"|"mock"|"configured",notifications:"mock"|"wecom-mock"|"wecom",attachmentStorage:"local"|"oss-mock"|"oss"}`。无密钥、租户、网络地址和 liveVerified 断言。

## 企业微信

- `GET /auth/wecom/start?entry=pc|h5`：顶层跳转授权页。PC 使用企业微信新版扫码页，H5 使用企业微信内网页授权。建立 5 分钟一次性 state 与 HttpOnly SameSite=Lax cookie。Callback 消耗 state、用服务端 API 换企业成员 ID，显式绑定且启用才建立登录票据。仅重定向固定 OA 页面 /oa.html 或 /h5.html，查询参数 `wecom=ready|failed`，无凭据。
- `POST /auth/wecom/exchange`，JSON 空对象，浏览器同源 cookie。消费 90 秒一次性 HttpOnly 票据，返回既有 LoginView。验 OA 员工仍在职、绑定仍有效、会话世代匹配。错误使用既有 401 UNAUTHENTICATED，提供商禁用返回 503 INTEGRATION_DISABLED。三个授权入口分别限流，state 和 ticket 按企业及运行模式隔离。
- `GET /employees/:id/wecom-binding`（HR_ADMIN）→ `{employeeId,version,memberId:null|string}`。 `PUT` 同路径正文 `{version,memberId:null|string,reason}`，null 解绑。仅管理已有受控 EMPLOYEE/APPROVER 账号。成员 ID 在同一运行模式和企业内唯一并统一小写，拒绝广播保留值及批量选择器。绑定、解绑撤销旧 OA 会话并写审计。改用真实模式时由管理员重新绑定成员。
- 事务 outbox 冻结投递模式，既有积压保持 mock。企业发送只带通用动作文本及事件编号。授权失效更新 token，明确可重试失败有限退避，超时/未知结果单列。 `GET /notification-deliveries?page=&pageSize=`（HR_ADMIN）→ `{id,kind,mode,status,attempts,createdAt}` 列表。status 为 PENDING/ACCEPTED/FAILED/UNCERTAIN/SKIPPED，历史本地投递另标 MOCK。普通审批通知保留站内收件箱记录，企业 API 接受后更新该条目的 deliveryMode。ACCEPTED 表示企业 API 已接受请求。
- 合同到期通知由 `ContractReminderSchedule bool` 显式开启，`ContractReminderDays int` 默认 30（1–90）。每分钟扫描在职员工今天至阈值内、未被有效续约替代的 SIGNED 合同。启用员工企业投递且已绑定时发通用企业通知，HR 管理员及查阅员收本地站内提醒。每合同版本、阈值、收件人只入队一次，投递前复核终止、续约、在职、账号与绑定。通知 kind CONTRACT_EXPIRING，新增可选 contractId，requestId=0，点击合同提醒入口。现有查询仍可看逾期。

## 附件分级、预览与迁移

- FileView 新增 `classification:"STANDARD"|"RESTRICTED",version`。旧附件与缺分类的旧上传调用沿用 STANDARD。新版 UI 新上传默认 RESTRICTED，用户可显式选普通。RESTRICTED 只允许 HR_ADMIN 和合同所属员工账号看见元数据、预览与下载；HR_VIEWER 看不到这类附件。旧附件如含工资须先按原文核对并分级。
- 现有 `POST /contracts/:id/files` 支持 multipart 可选 `classification` 字段。 `PUT /files/:id/classification`（HR_ADMIN）正文 `{classification,version,reason}`，返回 FileView。版本冲突 409 STALE_RECORD；写审计。
- `GET /files/:id/preview` 与下载完全同一权限，固定 PDF、JPEG、PNG，返回字节及 attachment disposition、no-store、nosniff、CSP sandbox。图片预览在浏览器使用 img；PDF 仅交隔离的 canvas/PDF 解析路径，禁内联文档、脚本、链接、外部取数和嵌入内容。不开放文件 URL 或浏览器持久缓存。
- 迁移为运维入口，非公开 HTTP。bootstrap.New 返回的 handler 实现 `MigrateAttachments(ctx context.Context, afterID uint, limit int, apply bool) (service.AttachmentMigrationResult,error)`。默认演练只读取本地、计算 SHA-256；apply 才上传已授权 OSS，读回逐字节/摘要校验后条件切换文件记录。冲突不覆盖，保留本地原件。报告只给文件 ID、摘要与状态，可按 nextAfterId 继续。

## Excel 导入

- `GET /employee-imports?page=&pageSize=`（创建者 HR_ADMIN）分页，条目 `{id,status,createdAt,expiresAt,confirmedAt,totalRows,validRows,errorRows,importedCount}`。status PREVIEW/EXPIRED/COMMITTED，旧记录没有确认时间为 null。无原表或工资明细。
- `GET /employee-imports/:id?errorsOnly=true` 可只看错误行；计数仍原全表。预览有效 30 分钟，过期状态 EXPIRED 且 rows=[]，确认/错误导出返回 410 PREVIEW_EXPIRED。确认原子去除源数据，历史确认重放仍返回原结果。周期性批量清理过期预览和已确认旧源数据。
- `GET /employee-imports/:id/errors.xlsx`，未过期 PREVIEW，仅错误行，包含物理行号、源值及错误说明，所有用户字段写文字单元格。没有错误时仅表头。 `GET /employee-imports/:id/result`，COMMITTED 返回既有 ImportResult。所有端点只允许预览创建者；其他账号统一 404。
