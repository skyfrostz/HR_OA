# HR OA 本地运行

这是使用模拟员工的开发环境。M1 提供员工档案、角色权限、劳动合同台账/续约及私有附件；M2 增加两级审批、驳回重提、转交、模拟站内通知和独立 H5。当前 PC 已切换为仓库内的 Pure Admin v7.0.0，M3 增加部门管理、员工编辑、手工年假、Excel 导入导出和合同到期提醒。操作说明见 `M3_USER_GUIDE.md`。M1/M2 历史实测分别保留在 `VALIDATION.md`、`VALIDATION_M2.md`。

## 启动

需要 Docker Desktop/Compose 和 Python 3。项目内不要求全局安装 MySQL。

```sh
cd /Users/skyfrost/Documents/Business/hr-oa
bash scripts/oa-dev.sh init
bash scripts/oa-dev.sh up
```

`init` 仅在 `.env` 不存在时生成随机开发密码和 JWT 密钥，权限为 0600；已有文件会保留，不输出密钥。`up` 从锁定的 Go 模块和前端锁文件构建镜像，等待 MySQL、API、Web 健康检查。

桌面人事入口 <http://127.0.0.1:8080/oa.html>，移动审批入口 <http://127.0.0.1:8080/h5.html>。两个入口独立登录，令牌不通过 URL 或存储共享。开发账号如下，密码是本机 `.env` 的 `OA_SEED_PASSWORD` 值；不要将密码提交到源码或测试报告。

| 账号 | 角色 | 行为 |
| --- | --- | --- |
| `hr_admin` | 人事管理员 | 所有员工/合同，工资可读，可编辑部门/员工/年假、导入及上传 |
| `hr_viewer` | 人事查阅员 | 员工/合同/普通附件只读，敏感附件不可见，响应不含工资字段 |
| `employee` | 员工 | 仅绑定的演示员工甲及其工资/合同/附件 |
| `manager` | 审批人 | 负责人节点；员工档案仅本人范围 |
| `finance` | 审批人 | 复核节点；员工档案仅本人范围 |
| `delegate` | 审批人 | 演示转交对象；员工档案仅本人范围 |

种子重复启动不会重置已有密码或覆盖业务数据。修改 `.env` 不会自动改变数据库中已有账号或 MySQL 卷的密码。刷新页面会回到登录页，因为令牌仅保存在内存。退出登录会清除当前浏览器会话；M4 起重置凭据、停用和离职会在数据库中撤销旧会话，容器重启后仍然有效。

## 演示审批

1. 使用 `employee` 登录 PC 审批中心或 H5，选择请假、报销或通用申请；新版请假需选择类型和半天范围、预览计算后提交。
2. 使用 `manager` 登录 H5，在待办中通过、驳回或转交给 `delegate`。通过第一步后，由 `finance` 处理第二步。
3. 申请人可以查看状态和操作历史；被驳回的申请可修改后重新提交，新记录关联原申请。
4. 默认开发配置仅生成站内模拟通知。M6 可单独配置企业微信登录和通知发送；模拟联调不会联系真实企业成员。

M5 支持 HR 配置有版本的顺序规则；没有匹配规则时，回退到负责人、复核两级演示路由。路径中包含申请人本人时会拒绝提交，因此 `manager` / `finance` 主要用于演示审批操作。人事管理员和查阅员没有全局审批详情权限，HR 的异常与已批请假处理只提供必要摘要。提交结果暂时无法确认时，请使用页面上的“重试原操作”；浏览器刷新会清除内存中的未确认操作，应先登录查看既有记录。请假和额度操作见 [M5 使用说明](M5_USER_GUIDE.md)。

## 数据与进程

- 使用单一 Compose 栈。MySQL 保存在 `hr-oa_mysql-data` 命名卷，附件保存在 `hr-oa_private-files` 命名卷的私有子目录。
- Web 发布到回环地址 8080，MySQL 调试端口发布到回环地址 13306。API 由 Compose 内网访问，经 Web 同源代理提供业务接口。
- `bash scripts/oa-dev.sh stop` 停止容器，保留数据。再次 `up` 可恢复。
- 不要将相同数据库与另一个附件目录组合使用。停止/重建不等于备份。`bash scripts/oa-backup.sh backup` 短暂停止 API 写入后成对备份数据库和附件，再恢复 API；备份位于本机忽略目录。恢复脚本使用独立临时环境验证，不覆盖活动卷，不代表异地备份或生产容灾已完成。

## 验证

```sh
bash scripts/oa-dev.sh verify
```

该命令会创建额外的模拟员工/合同/附件，验证权限与文件边界，并重建容器后检查原记录和附件字节。它不删除数据卷。浏览器场景由 `tests/oa_browser.py` 执行，Python 浏览器依赖锁定在 `tests/requirements-browser.txt`，安装到本地隔离环境；具体实测命令见 `VALIDATION.md`。

开发者可在 `server` 目录运行 `go test -mod=readonly ./oa/...`，在 `web` 目录运行 `npm run test:oa` 和 `npm run build:oa`。这些检查与真实 MySQL/浏览器验收分别记录。日常使用优先完整 Compose 入口；Vite 调试入口需要另行配置可达的 OA API 代理目标，不能直接假定容器 API 已发布到宿主机 8888。

主 PC 在 `web-pure-admin` 目录执行 `npx --yes pnpm@10.32.1 install --frozen-lockfile --ignore-scripts`，然后分别运行 `npx --yes pnpm@10.32.1 run test`、`run typecheck`、`run build`。Dockerfile 会将 Pure 的 `oa.html` 和 `pure-assets/` 与原前端的 `h5.html`、`assets/` 组合。原前端构建同时生成的旧 `oa.html` 不进入组合镜像。

Pure 运行验收入口为 `bash scripts/oa-pure-verify.sh`；M3 为 `bash scripts/oa-m3-verify.sh`。两者依赖本机开发验收状态文件和浏览器环境，创建模拟记录；M3 脚本还执行成对备份及隔离恢复。不要用于真实业务数据库。

`scripts/oa-m2-verify.sh` 保留为 M2 历史里程碑的全套验收入口，不能原样对 M5 执行：其新建请假夹具使用旧表单，而 M5 新申请必须提供类型、半天和预览版本。该脚本会切换通知故障模式、注入测试触发器和重建容器，只适合对应版本的隔离开发环境。M5 的历史兼容、幂等、并发和事务回归在 `tests/oa_m5_api.py` 执行，不将“旧成功意图可重放”误认为“新旧字段请求仍可提交”。

M5 隔离入口为 `OA_QA_MILESTONE=5 bash scripts/oa-m4-isolated.sh python3 -B tests/oa_m5_api.py`，依赖本机 `.local/m5-baseline/` 中的 M4 基线归档及构建好的候选 API 镜像，具体参数见脚本。它恢复独立 MySQL 后验证升级与故障注入，不改写主栈业务数据。最终浏览器入口为 `tests/oa_m5_browser.py`；所需模拟状态文件与浏览器环境不随源码分发。实际通过项与运行边界见 [M5 验证](VALIDATION_M5.md)。

`OA_MOCK_NOTIFY_FAIL` 默认为 `false`。开发验收切换为 `true` 时仅使模拟通知失败，审批记录仍然提交，待发事件和尝试次数保存到数据库。恢复 `false` 后重新启动 API 会继续投递。脚本尝试在故障阶段退出时恢复正常配置；异常中断后可运行 `OA_MOCK_NOTIFY_FAIL=false docker compose up -d --wait` 恢复。

## 集成边界

默认开发栈使用密码登录、本地私有卷、可配置顺序规则和模拟通知。M6 已实现企业微信与阿里云 OSS 服务端适配器、敏感附件预览、Excel 历史和可移植恢复；配置见 [M6 运维说明](M6_OPERATIONS.md)，独立 MySQL 与模拟提供商验收入口为 `bash scripts/oa-ci-integration.sh --mock-providers`。使用模拟员工资料，源码按授权发布到 GitHub，运行栈未部署公网。真实企业应用和 Bucket 尚未准备，客户环境接入待配置和联调。正式表单、日历、额度和审批策略仍需客户确认，生产部署与安全验收尚未完成。具体接入状态见 [集成状态](INTEGRATION_STATUS.md)。
