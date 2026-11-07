# HR OA 本地开发与验收

从仓库根目录执行。需要运行中的 Docker Engine、Docker Compose 和 Python 3。首次运行时，`init` 在缺少 `.env` 时生成本机开发凭据。已有文件会保留。

```bash
./scripts/oa-dev.sh init
./scripts/oa-dev.sh up
```

访问 http://127.0.0.1:8080/oa.html。开发账号为 `hr_admin`、`hr_viewer` 和 `employee`，使用本机 `OA_SEED_PASSWORD`。API 通过 Web 的同源代理访问，健康检查为 `/api/oa/v1/health`。MySQL 的宿主机端口为 `127.0.0.1:13306`。API 容器只在 Compose 网络内提供服务。

数据库和附件分别存放在 `mysql-data` 与 `private-files` 命名卷。运行脚本统一使用完整 Compose 栈，避免同一个数据库在不同启动方式之间指向不同的附件目录。执行 `./scripts/oa-dev.sh stop` 会停止容器并保留数据。此栈只用于本机开发和模拟资料验收。

## 验收

```bash
./scripts/oa-dev.sh verify
```

脚本写入随机编号的模拟资料，短暂变更并恢复种子 `employee` 账号的授权状态，随后重建容器并复核同一员工、合同和附件字节。执行期间会短暂中断本地服务，应在专用开发栈中运行。测试数据留在命名卷供检查，验收状态文件写入被忽略的 `tests/results/`。

浏览器验收使用 API 验收生成的状态文件和本地 Chromium。可在被忽略的目录安装独立测试环境。

```bash
python3 -m venv .local/qa-venv
.local/qa-venv/bin/python -m pip install -r tests/requirements-browser.txt
PLAYWRIGHT_BROWSERS_PATH="$PWD/.local/playwright-browsers" .local/qa-venv/bin/python -m playwright install chromium
(
  set +x
  set -a
  source ./.env
  set +a
  PLAYWRIGHT_BROWSERS_PATH="$PWD/.local/playwright-browsers" .local/qa-venv/bin/python -B tests/oa_browser.py
)
```

## 本地备份和恢复演练

```bash
archive="$(./scripts/oa-backup.sh backup)"
./scripts/oa-backup.sh restore "$archive" tests/results/oa-state.json tests/results/m2/oa-m2-state.json tests/results/m3/state.json
```

备份会短暂停止 API 和它的通知 worker。演练期间不要经 MySQL 调试端口直接写库。脚本使用 MySQL 一致性快照导出数据库，归档同一暂停期间的私有目录，再恢复原 API。默认输出到被忽略的 `.local/backups/`，每个快照目录权限为 0700，SQL、附件、对应 API 镜像 ID 和校验清单权限为 0600。备份含数据库和私有文件，应按敏感资料保管。

恢复校验会使用备份记录的本机 API 镜像、新的 MySQL 临时数据库、独立的 API 文件系统和临时回环端口。需要有已知的模拟员工、合同、附件及审批状态文件。最后一项 M3 状态文件可省略；提供时会额外比较部门与年假记录。校验检查记录 ID 和附件字节摘要，完成后移除隔离容器。演练不会把数据写回现有业务库或附件卷。该流程覆盖本机可读回验证，生产备份保留策略和异地恢复需要另行验收。

## 隔离 MySQL 持续验收

从全新检出运行 `bash scripts/oa-ci-integration.sh`。脚本创建随机 Compose 项目、一次性凭据与临时 MySQL，API 只使用随机回环端口，结束后清理。无需本地 `.env`、历史测试 JSON 或其他开发项目。模拟 WeCom 与私有 OSS 的额外 HTTP 联调使用 `bash scripts/oa-ci-integration.sh --mock-providers`，提供商服务只接受合成请求，固定在隔离网络。

## 生产配置模板

`production.compose.yaml` 与 `production.env.example` 是一个 API 实例和一个同源 Web 代理的部署模板。准备带 TLS 验证的独立 MySQL、可信 HTTPS 入口、DNS 和两个不可变镜像引用。将模板复制到被忽略的 `.local/oa-production.env`，设为 0600 并配置客户独立的强密钥。生产要求 OA 公共 origin 为 HTTPS，数据库 DSN 使用 `tls=true`。按首次启动约定提供管理员引导用户名和密码，创建后删除引导凭据并重新创建 API 容器。开发种子和模拟提供商在生产模式被拒绝。

先做静态检查，不输出解析后的秘密。

```bash
export OA_PRODUCTION_ENV_FILE="$PWD/.local/oa-production.env"
docker compose --env-file "$OA_PRODUCTION_ENV_FILE" -f deploy/oa/production.compose.yaml config --quiet
docker compose --env-file "$OA_PRODUCTION_ENV_FILE" -f deploy/oa/production.compose.yaml run --rm --no-deps api --check-config
```

模板只把 Web 发布到宿主机回环地址，API 和 MySQL 连接留在受控网络。应用只信任固定 Web 代理 IP。Nginx 覆盖来访的转发地址头，丢弃可伪造的协议和主机转发头。若前面还有宿主机 HTTPS 入口，默认限流会共享入口地址。需要按客户端区分时，应在确认真实连接拓扑后配置 Nginx realip，只信任已知入口的精确来源并由入口覆盖客户端地址头。避免让边缘访问日志记录 OAuth code、state 或完整查询串。生产环境不使用开发栈的凭据或卷。

## 可移植本地快照与恢复

```bash
snapshot="$(./scripts/oa-backup.sh backup)"
portable="$(./scripts/oa-backup.sh pack "$snapshot")"
./scripts/oa-backup.sh verify "$portable"
./scripts/oa-backup.sh restore-check "$portable"
```

portable 目录包含同一时间点的 MySQL 导出、私有附件、精确 API 和隔离恢复用 MySQL 镜像、SHA-256 清单及无凭据的恢复验收清单。恢复清单在导入归档后记录关键表行数和每个私有文件的字节数与摘要。目标机器用同版本脚本、兼容镜像平台及 Docker Compose 执行 `restore-check` 即可校验 API 健康、行数和附件摘要。命令自动加载已验证的镜像，不需要历史 fixture JSON 或原账号密码。Web 镜像由部署镜像库单独保留。此恢复验证在本机独立项目执行，异机灾备演练单独记录。

备份包含私有数据库和原文件。传输和保留按客户备份制度加密与控制权限，接收目录保持 0700、文件 0600。仅加载可信来源归档，摘要用于检测变化。若数据库包含远端 OSS 附件，本地备份命令会拒绝产生不完整快照。实际使用 OSS 时应在维护窗口协调数据库与对象存储备份，保存私有对象清单和字节校验结果，按现有对象/保留策略在隔离环境逐项恢复后再切流量。

## 升级及附件迁移

生产迁移先停止所有写入实例、保留旧镜像及成对快照，再用目标镜像启动一个实例执行数据库版本检查和升级。MySQL DDL 的事务边界与业务事务不同，失败时应检查版本状态和诊断，按已验证快照恢复，不能假定切回旧镜像能自动回退数据库结构。升级前后用旧数据、附件及审计做实际读回。请确认新旧读写模式、版本表和升级回退流程均符合发布记录。

本地附件迁移默认只演练，运行结果使用记录 ID、字节摘要和续跑锚点。先核对 dry run，然后使用相同镜像和配置执行明确的 `--apply`。本地原件在迁移后保留。启用 OSS 写入前完成私有权限预检和备份策略检查。
