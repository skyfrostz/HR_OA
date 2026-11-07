# M1 本地验收结果

2026-11-06（Asia/Shanghai）。**M0 启动基线与 M1 本地开发里程碑通过。** 仅使用模拟员工和附件，完整一期、企业接入与生产验收尚未完成。

## 验证对象

- Git 基线：GVA v2.9.2-stable，`a6882210a80bb27e3aa5dff0b4c21aa4afe8988a`，分支 `hr-oa/m1`。功能实现为其上的本地工作树增量，未提交或推送远端。
- 60 个关键源码、配置、锁文件、脚本的最终 SHA-256 见 `M1_SOURCE_SHA256SUMS`。总控在 QA 完成后独立复核，0 项不一致。说明文档不属于运行镜像源码快照。
- MySQL 8.4.8；API 构建 Go 1.26.4；Web 容器 Node 24.15.0；Playwright 1.60.0 / Chromium。
- 已验证 API 镜像：`sha256:426949ebef206eb7cd184e191b6adf4eb9a46e2bcf8e6ea042ee624cc75c22f4`。
- 已验证 Web 镜像：`sha256:c6307d83209786c56c44034c445d7c8109a7b123ab8a32b90b4e61730796a306`。

## 结果

| 检查 | 最终结果 | 证据范围 |
| --- | --- | --- |
| `go test -mod=readonly ./oa/...` | PASS | OA 包的工资字段、金额/日期边界、上传限制、路由、Casbin、限流行为 |
| `go test -mod=readonly -race ./oa/...` | PASS | 已执行 Go 测试路径的竞态检测 |
| `go build -mod=readonly … ./cmd/oa` | PASS | 宿主机与最终容器 API 均构建成功 |
| OA Swagger 解析检查 | PASS | 10 个路由及 x-token 鉴权注解可解析 |
| `npm run test:oa` | PASS，7/7 | Vue Test Utils/jsdom + 模拟 API，独立于下述真实浏览器验证 |
| `npm run build:oa`、容器内 `npm ci` | PASS | OA 生产构建和从锁文件安装成功 |
| `docker compose config --quiet`、Nginx/脚本语法 | PASS | 配置校验；与实际启动分开记录 |
| `bash scripts/oa-dev.sh init` 临时目录两次执行 | PASS | 生成随机配置、0600 权限、不输出凭据、不覆盖已有文件 |
| `bash scripts/oa-dev.sh up` | PASS | 最终源码构建，MySQL/API/Web 均 healthy |
| `bash scripts/oa-dev.sh verify` | PASS | 真实 MySQL API 与容器重建后的持久化验收 |
| `.local/qa-venv/bin/python -B tests/oa_browser.py` | PASS | 真实 Chromium 三角色操作、附件与窄屏场景 |
| 总控健康接口、容器状态、源码哈希复核 | PASS | code=0、database=ok；三个容器 healthy；60 项哈希一致 |
| 许可证、Go 依赖和原 GVA Web 入口不变检查 | PASS | LICENSE、go.mod/go.sum、原入口/构建配置无差异 |

浏览器命令在仓库根执行，已从本机忽略的 `.env` 加载测试变量；没有将密码作为命令行参数或写入报告。运行与重复验收步骤见 `LOCAL_DEVELOPMENT.md`。

## 真实行为证据

- 缺失、无效及过期 JWT 被拒绝。已签发令牌在数据库角色变化和账号禁用后，按当前数据库状态重新判权。
- 管理员可创建和读取工资；查阅员列表/详情完全省略 `salaryCents`，写操作被拒绝；员工仅能访问本人行、工资、合同和附件。跨员工详情、合同列表及附件访问均失败。
- 无效日期、负数/小数工资、重复工号/合同编号和跨员工续约被拒绝。并发重复工号得到一次成功和一次冲突。
- PDF/JPEG/PNG 合法上传、授权下载通过；扩展名与内容错配失败；恰好 10 MiB 通过，10 MiB + 1 字节被拒绝。截断 multipart 返回 400，临时文件列表没有增长。
- API、MySQL、Web 容器重建后，同一员工/合同/附件 ID 仍可读取，PDF SHA-256 与重建前一致。没有删除或重建数据卷。
- 危险路由既从 Web 入口探测，也从 API 容器内直探，覆盖根路径、`/api`、`/api/oa/v1` 下的初始化、自动生成、插件、LLM、示例上传和公开文件路径，均返回 404。源码路由白名单已另行审查。
- Chromium 跑通管理员创建员工/合同/上传/下载，查阅员工资及写入口隐藏，员工仅本人资料/附件。上传已成功但响应丢失时，页面要求刷新核对；同步双提交只发出一次写请求。
- 三角色场景检查 localStorage/sessionStorage/IndexedDB/Cache Storage 和 Cookie，未持久化会话或 HR 数据。刷新需重新登录。390px 员工详情无页面级横向溢出。

## 保留的失败与修复记录

1. Alpine 镜像查询曾 TLS 超时；后续实际拉取成功。
2. 首次 Go 编译因 GVA JWT 两个内嵌 ID 字段歧义失败；改为显式用户 ID 后编译、单测和竞态检测通过。
3. 首次完整栈构建在已知编译错误和全量依赖预下载期间主动中止；Dockerfile 改为只解析实际构建所需模块，最终完整栈通过。
4. 前端首轮测试发现登录标签未关联实际输入，改为原生输入控件后 7 项测试通过。
5. 总控审查修复了工资安全整数上限、日期比较和附件启动模式一致性。最后的 multipart 临时文件清理补丁已纳入最终 API 镜像，并重新验收。

开发机 Node 26 下锁文件 dry-run 曾出现上游传递依赖 `@achrinza/node-ipc` 的 EBADENGINE 警告；已验证的 Web 容器使用 Node 24，并成功干净安装。这不构成全依赖漏洞审计。

## 证据索引与未完成范围

- 原始角色报告：仓库同级 `hr-oa-coordination/BACKEND_REPORT.md`、`FRONTEND_REPORT.md`、`QA_REPORT.md`。
- 原始验收输出：`.local/qa-api-accepted.log`、`.local/qa-browser-accepted.log`、`.local/qa-compose-accepted.log`、`.local/qa-manifest-accepted.log`。
- 模拟资料截图：`tests/results/browser/admin-contract.png`、`tests/results/browser/employee-narrow.png`。日志和截图保持本地忽略。
- **NOT RUN / 未实现：** 生产安全与完整依赖漏洞评估、真实企业微信/钉钉、COS/OSS、备份恢复、真实移动客户端验证；完整审批、专用 Vant H5、员工编辑、组织管理、年假、Excel 导入导出和到期提醒仍属后续。未来审批幂等/outbox 方案不代表 M1 已实现。
- 文件与数据库跨介质提交遇到进程崩溃或结果不明确时，可能留下孤立附件；后台清理尚未实现。当前只宣称上述开发验收范围。

开发栈保留运行于 <http://127.0.0.1:8080/oa.html>，没有公网部署或真实消息外发。
