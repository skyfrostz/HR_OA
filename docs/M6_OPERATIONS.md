# M6 配置与维护操作

本页配合 `deploy/oa/production.env.example`。模板不是已经上线的服务。企业微信与 OSS 资源尚未准备，本轮仅执行模拟服务联调。

## 配置方式

把环境模板复制到本机忽略目录 `.local/`，文件权限设置为 `0600`；不要把真实配置提交 Git，也不要使用前端 `VITE_*` 变量存放密钥。运行时读取服务端环境，页面不提供密钥输入框。

| 配置 | 用途 |
| --- | --- |
| `OA_ENV` | `development`（默认）或 `production`；拼写错误拒绝启动 |
| `OA_MYSQL_DSN` | MySQL 连接串；生产连接要求验证证书的 TLS |
| `OA_JWT_SECRET` | 开发至少32字节；生产至少48字节且应由密码学随机源生成 |
| `OA_PUBLIC_ORIGIN` | 固定站点 origin；生产必须 HTTPS，不能含账号、路径、查询或片段 |
| `OA_TRUSTED_PROXIES` | 逗号分隔显式 IP/CIDR；默认不信任转发头，不接受全网通配 |
| `OA_DEV_SEED` | 仅开发可用；生产禁用演示员工和开发种子 |
| `OA_BOOTSTRAP_ADMIN_USERNAME / PASSWORD` | 空库首次管理员初始化；不覆盖既有账号，不用于重置密码 |
| `OA_WECOM_MODE` | `disabled`、`mock`、`wecom`；配置就绪不等于客户联调通过 |
| `OA_WECOM_CORP_ID / AGENT_ID / SECRET` | 企业应用服务端配置；密钥不能下发浏览器 |
| `OA_WECOM_DELIVER_NOTIFICATIONS` | 独立打开企业通知；默认关闭，不因开启登录就自动发送消息 |
| `OA_WECOM_MOCK_API_BASE / MOCK_AUTHORIZE_URL` | 仅明确本地模拟服务，生产拒绝 |
| `OA_OSS_ENABLED / MOCK` | 是否启用对象适配器及本地模拟模式；生产拒绝 mock |
| `OA_OSS_REGION / BUCKET / ENDPOINT / PREFIX` | 指定私有对象范围；以最终适配器支持的端点规则为准 |
| `OA_OSS_USE_CNAME` | 显式使用已配置的 HTTPS 自定义域名；模拟模式禁用 |
| `OA_OSS_ACCESS_KEY_ID / ACCESS_KEY_SECRET / SECURITY_TOKEN` | 服务端最小权限凭据；临时凭据过期时必须更新，不静默切回本地 |
| `OA_ATTACHMENT_WRITE_BACKEND` | `local` 或 `oss`；读取始终按每个文件记录选择后端，修改此值不会自动迁移旧附件 |
| `OA_CONTRACT_REMINDER_SCHEDULE / DAYS` | 显式启用合同提醒调度；开启时0天配置使用默认30天，允许1–90天 |

环境文件必须由操作者信任。如果通过 shell 加载，应关闭命令追踪，避免终端回显；更适合使用 Compose 的 `env_file` 注入。生产模板采用单独项目名、只读 API 根目录、可写私有卷和宿主 loopback Web 端口。HTTPS 入口与生产 MySQL 需独立配置，不由该模板购买或创建。

## 配置检查

二进制支持 `hr-oa --check-config`，仅检查配置，不连接数据库、企业微信或 OSS。通过该命令不代表网络或凭据有效。`hr-oa --version` 输出打包版本；未注入版本的本地构建显示 development。错误消息不得包含 DSN、密码或密钥值。

运行 API 镜像也可显式传入以上参数；生产 Compose 需先填写私密环境文件和不可变镜像引用。不要把整个配置展开打印到日志。`docker compose config --quiet` 仅验证 Compose 语法和变量，不替代应用检查。

## 附件迁移

迁移命令为 `hr-oa --migrate-attachments --after-id=0 --limit=100`，默认演练。仅显式追加 `--apply` 才向已配置 OSS 上传，验证读回字节后条件切换数据库引用。没有 HTTP 批量迁移入口。

维护任务不启动通知 worker；上传失败、读回不一致或版本冲突不能切换记录。本地原件保留。命令先输出逐文件结果，存在失败或被中断时返回非0状态。先按报告重新处理失败 ID，再使用游标推进；不能跳过失败项直接认定迁移完成。重复运行应安全；不要手工修改 StorageKey 或把 Bucket 改成公开权限。演练和配置检查不能当成真实迁移已完成。

服务端 RAM 权限模板见 [OSS_MINIMUM_POLICY.json](OSS_MINIMUM_POLICY.json)，替换 Bucket 占位和对象前缀后再由资源管理员审核应用。它只授权桶安全状态读取和指定前缀的对象读写，不创建/删除桶、修改桶权限或删除对象。权限依据官方 [PutObject](https://www.alibabacloud.com/help/en/oss/developer-reference/putobject)、[GetObject](https://www.alibabacloud.com/help/en/oss/developer-reference/getobject) 与 [GetBucketPublicAccessBlock](https://www.alibabacloud.com/help/en/oss/developer-reference/getbucketpublicaccessblock) 文档核对，未在真实 RAM/Bucket 上应用。当前使用 AES256 服务端加密；不需要 KMS 全权限。

切换写后端前后均需成对备份数据库及本地原件。当前适配器要求私有桶、开启阻止公共访问且未启用/暂停版本控制，以保证禁止覆盖语义；不要直接改变这一前置条件。云端对象需另设兼容的独立备份措施，本地备份包不能宣称完整云灾备。

## 代理和登录

API 只信任明确的 Nginx 地址。Nginx 覆写客户端提供的转发头；额外 HTTPS 代理串联时，须明确每一层的可信地址，否则所有用户可能共享上一层代理的登录限流桶。不要为了消除这一现象而信任任意客户端 X-Forwarded-For。

企业微信回调使用固定 origin 与服务端一次性状态，OA JWT 仍只保存在浏览器内存中。企业绑定不会创建员工或提升角色；管理员需先建立受控 OA 账号再显式绑定成员。用户停用、离职和解绑必须失效已有登录。

## 验收边界

开发功能以 VALIDATION_M6.md 为准。模拟服务不会联系真实企业成员；真实通知、真实私有 Bucket、TLS 域名、客户主机部署和异机恢复需要资源到位后另行验证。
