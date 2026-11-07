# HR OA 商业审查：总控/CI/交付文档

日期：2026-11-07。基线 `main=84b9809cc226fca820714b47c373f1f54a3a0a56`。本轮范围为根 CI、README、交付契约、需求记录与协调报告；未修改 `server/oa`、前端源码、测试交付脚本或运行栈，未提交/推送。

## 结论

本地 M1–M3 工程验收仍是历史有效证据，但首次 GitHub CI 并未全绿，更不能视为生产交付验收。当前至少有前端测试稳定性、BSL 客户生产授权、企业微信/OSS 真实接入、正式审批策略和生产运维待办。本轮搜索/筛选导出属于新增工作，契约落盘不代表完成。

## 首次 CI 的实际结果

[Run 42322714206](https://github.com/skyfrostz/HR_OA/actions/runs/42322714206)，提交 `84b9809`：

| Job | 结果 | 实际证据 |
| --- | --- | --- |
| backend / 128250786696 | PASS | Go 安装、`go test -mod=readonly -race ./oa/...`、OA 入口构建均成功 |
| h5 / 128250786853 | PASS | npm ci、原前端测试和 build:oa 均成功 |
| pure-admin / 128250786858 | FAIL | Node 24.15.0 / pnpm 10.32.1 安装成功；测试 3/4 通过，首次布局/角色测试失败，类型和构建 skipped |

直接失败位置为 `web-pure-admin/src/oa/pure-app.test.js:48`：登录辅助函数等待 `.sidebar-container`，断言报 `expected null not to be null`。首个测试耗时 1494 ms，第二个 M3 集成测试 7135 ms 通过。源码没有为该 `vi.waitFor` 指定超时；**冷加载/异步等待是需要前端复现确认的原因候选，日志本身不能证明只是机器慢**。已将此证据交给前端和本轮后端协调方，没有用根 CI 延长全局超时或跳测来掩盖失败。

原始日志完整保存在忽略目录 `hr-oa/.local/commercial-review/ci/pure-admin-job-128250786858.log`，24784 字节，SHA-256 `60d63684e99d142546c3cfb58a0871da5028b187cd87a49765f8591075360c6c`。保留 GitHub 的失败和 skipped 状态，不将本地修复倒写为原运行成功。

## 根 CI 调整

`.github/workflows/ci.yaml` 为 Pure 安装、测试、类型和构建添加可辨识步骤与 ID；在依赖安装成功且未取消的条件下，类型和构建独立执行，最后记录各步骤 outcome。没有 `continue-on-error`、没有删测试、没有改变 Node/pnpm/锁版本。测试失败仍使整个 job 失败。这项变更改善诊断，不是对前端失败根因的替代修复。

验证：Ruby YAML 解析、步骤 ID 唯一性、无失败屏蔽、条件表达式静态核对、summary 的 `bash -n`、`git diff --check` 均 PASS。首次本地检查命令用了宿主旧 Ruby 不支持的 `filter_map`，改用 `map.compact` 后通过；工作流本身未受该工具兼容错误影响。GitHub 远端重跑 **NOT RUN**，本轮明确禁止提交/推送。没有把本地静态检查写成实际 Actions 通过。

## 许可事实与交付风险

已再次核对本地 `LICENSE` 和 [固定上游提交的官方 LICENSE](https://github.com/flipped-aurora/gin-vue-admin/blob/a6882210a80bb27e3aa5dff0b4c21aa4afe8988a/LICENSE)：文本为 BSL 1.1。其 Production Use 定义包括向外部客户部署/交付、组织基础设施运行和商业集成；Section 1 要求另行取得商业许可。文中的 Apache-2.0 是达到相应版本 Change Date 后的 Change License，不是当前组合已经取得的 Apache 授权。MIT 的 Pure Admin 不能解除 GVA 的条款。

本会话没有客户商业授权证明，也未核实可以主张的 Change Date。因此当前不能宣称客户生产交付许可已满足。许可证事实核对不替代具体合同或法律适用判断；商业发布前应取得覆盖用途的上游许可，或另行评估并授权替换底座。仓库源码发布与本地合成资料测试不能自动证明客户生产用途被许可。

## 新接口和需求记录

- `docs/COMMERCIAL_REVIEW_API.md`：记录 POST `/employees/search` 和 POST `/employees/export.xlsx`，query 首尾修剪/100 Unicode 字符、字面子串、精确本部门、同授权范围/工资权限/5000 上限，旧 GET 保留。补充列表已应用筛选与导出一致、异步结果顺序、零匹配表头、PII 不入 URL/持久存储及 QA 要求。已发后端核对。
- `aiDoc/frontend-backend/boundary.md`：指向新接口增量，不替代 M1–M3 历史契约。
- `aiDoc/memory/business/active/hr-oa-commercial-delivery-review.md` 与需求索引：按当前转述来源记录商业审查和首批功能，不伪造用户逐字原话。
- README：补充首次 CI 实际失败及新契约入口，不宣称新增功能已验收。
- 历史验收文档和哈希清单未改。另发现上游原需求索引链接 `active/license-migrate-to-bsl.md` 指向的文件不存在，保留原条目并在此记录；未编造该历史需求正文。

## 交接和剩余验证

1. 前端复现并修复 CI 登录等待问题，处理其负责的编辑状态和暗主题；后端完成新增搜索/导出接口。
2. QA 验证真实 MySQL 四角色范围、字面搜索、导出权限与浏览器的筛选一致性，并检查没有 PII 入 URL/持久存储。总控不启动测试栈。
3. 获准提交后运行新 GitHub CI，记录具体提交和三项 job 的结果；原失败日志继续保留。
4. 当前报告只覆盖总控职责。其他项目 UI/底层代码的深度比较和本轮功能是否达标，以各角色后续证据综合评审。

## 最终收口补记

首批后端/前端实现及真实 MySQL、Chromium、5000/5001 边界和自动重启验收均已完成；以上待办是角色交接时的历史状态。总控核对 QA 的 32 项文件全部匹配，独立检查最终浅/深色和窄屏截图，再收口综合报告与 README。根 CI 远端重跑仍 NOT RUN；本轮未提交或推送。当前商业缺口与分阶段建议以 docs/COMMERCIAL_REVIEW.md 为准。
