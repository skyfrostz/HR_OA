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
