# 仓库发布说明

日期：2026-11-07。用户明确授权将当前 HR OA 项目推送到 `skyfrostz/HR_OA` 的 `main`，并完善项目 README。本次授权为源码发布，不包含公网业务部署、真实企业消息或 OSS 上传。

## 许可说明纠正

发布前逐字检查发现，固定 GVA 提交 `a6882210a80bb27e3aa5dff0b4c21aa4afe8988a` 的 `LICENSE` 是 **Business Source License 1.1**。此前 `UPSTREAM.md` 和 Pure 集成说明中的 Apache-2.0 描述有误，现已纠正。BSL 文本中的 Apache-2.0 是未来 Change License，不能据此将当前版本标记为 Apache-2.0。

固定来源可在 [上游提交的 LICENSE](https://github.com/flipped-aurora/gin-vue-admin/blob/a6882210a80bb27e3aa5dff0b4c21aa4afe8988a/LICENSE) 核对。当前根目录保留该原始文本及署名，不修改条款；Pure Admin 的 MIT 文本也保持不变。原始 GVA 中文 README 保存为根目录 `README.gin-vue-admin.md`，保持原相对链接基础和原始内容；原英文 README 保留。

目标仓库 `main` 原提交 `656bd12960fc28a160d19a96bd56650e62da77f4` 含初始化 README 和 Apache-2.0 文本。初始化许可证完整保存为 `LICENSE-HR-OA`，不覆盖引入组件的条款；发布提交接续原 `main`，保留该提交历史。根 README 是 HR OA 的新项目入口。

## 发布内容

- 已验收的 OA API、Pure PC、Vant H5、Compose 构建、测试、备份恢复脚本和相关文档。
- README 的功能状态、架构、权限矩阵、配置、快速启动、开发验证、运维、后续集成和组件来源。
- 两张已检查的本地合成资料截图，放在 `docs/assets/`；不分发测试数据库或状态文件。
- 根工作流改为本项目 Go/Pure/H5 的检查，不沿用上游部署逻辑；上游工作流原文保存在 `docs/upstream/gin-vue-admin-ci.yaml`，该位置不会触发 GitHub Actions。
- 修改的 GVA 原文件补充修改说明。业务接口、数据库迁移和页面逻辑没有因本次 README 发布而更改。

## 证据边界

`M1_SOURCE_SHA256SUMS`、`M2_SOURCE_SHA256SUMS`、`PURE_M3_SOURCE_SHA256SUMS` 是各自验收时的历史快照。此次文档、修改声明与 CI 调整不回写这些历史清单；它们不应被当成发布后整个工作树的零差异清单。`PURE_ADMIN_UPSTREAM_SHA256SUMS` 始终描述原始 Pure 导入，不能被修改后的内容替换。

本机 `.env`、依赖、构建输出、数据库、私有附件、备份、日志和测试状态不参与发布。当前企业微信和阿里云 OSS 未配置，历史本地 PASS 不证明真实外部联调或生产可用性。CI 的实际结果以该发布提交对应的 GitHub Actions 状态为准。
