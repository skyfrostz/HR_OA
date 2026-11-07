# HR OA 商业交付 QA

2026-11-07。本轮独立记录，历史 M1/M2/M3 证据保持原状。当前栈仅使用模拟资料。

| 项目 | 状态 | 证据 |
| --- | --- | --- |
| 现有运行基线 | PASS | 原 API、Web、MySQL 都健康，原 Compose 仅 MySQL 配置自动重启。 |
| Compose 和验收脚本静态检查 | PASS | API/Web 已添加 `restart: unless-stopped`。配置、脚本及 CLI 静态检查通过。 |
| 新搜索/筛选导出真实 MySQL | PASS | 字面特殊字符、Unicode、组合条件、分页、导出与四角色隔离通过。 |
| XLSX 5000/5001 上限 | PASS | 独立 tmpfs MySQL 中管理员和查阅员均可导出完整 5000 条，5001 条均拒绝，隔离环境已清理。 |
| Pure/H5 Chromium | PASS（最终镜像） | 筛选、迟到响应不覆盖、按已应用条件导出、部门草稿、角色切换清理、浅/深色和 H5 刷新通过。H5 通过刷新读到由另一会话新提交的申请。 |
| 深色员工列表文字对比度 | 首轮 FAIL，最终 PASS | 首轮表头 3.01:1、工号 3.70:1、「查看档案」3.61:1。修复后列表最小对比度 1440px 深色 6.15:1、浅色 4.64:1；390px 深色 7.56:1、浅色 4.97:1。深色入口 7.56:1、导航 9.66:1，列表分隔线随主题变化。 |
| API/Web 自动恢复和原文件读回 | PASS | 同容器 PID 1 退出后，两个服务重启次数均从 0 增到 1，健康恢复后原附件逐字节一致。 |

执行后日志及截图写入本机忽略目录。现有命名卷和 `.env` 保留。


## 实际命令与证据

- `./scripts/oa-dev.sh up` 构建并启动组合栈。输出 `hr-oa/.local/commercial-build-1.log`。
- 读取本机测试环境后执行 `python3 -B tests/oa_commercial_api.py`，日志 `commercial-api-1.log`。旧 GET 默认列表及全量导出兼容；真实 MySQL 验证字面百分号、下划线、单引号、Unicode 边界、精确部门不含子部门、分页与筛选导出一致、空工作簿、字面 Excel 字符及四角色工资和行权限。
- `./scripts/oa-commercial-export-limit.sh` 的隔离 MySQL 使用独立临时文件系统。管理员与查阅员在恰好 5000 匹配行时完整导出，5001 行返回 409。日志 `commercial-limit-1.log`，隔离容器与网络已清理。
- `./scripts/oa-commercial-restart-check.sh` 在原服务进程内发送退出信号，API 与首版 Web 真实自动恢复且原附件字节一致。日志 `commercial-restart-1.log`。首版 Web 镜像 ID `sha256:349b4eb84d1d05b722c649a99cdc73419d2da9a76b3625c7af3030bd84c32f05`。最终 Web 仅因 CSS 修复重建，容器仍使用已核对的 `unless-stopped`。
- `docker compose up -d --build --no-deps --wait --wait-timeout 120 web` 重建深色修复后的 Web，日志 `commercial-web-final.log`。读取本机测试环境后，`.local/qa-venv/bin/python -B tests/oa_commercial_browser.py` 最终通过，日志 `commercial-browser-final-2.log`。保留首轮 `commercial-browser-1.log`、原深色缺陷 `commercial-dark-table-contrast.log`，以及首次复验脚本误把 CSS `color(srgb ...)` 小数当作 8 位 RGB 的失败日志 `commercial-browser-final.log`。改用 Chromium Canvas 转换颜色后通过，无需再改运行代码。
- 浏览器检查敏感筛选没有进入 URL、浏览器存储，切换账号后输入清空。另检查了样本关键词、工号和姓名没有出现在 API/Web 容器日志。
- 最终截图在 `hr-oa/tests/results/commercial/browser/`。列表为 `pc-light-1440.png`、`pc-dark-1440.png`、`pc-light-390.png`、`pc-dark-390.png`；深色入口为 `pc-dark-mobile-entry-1440.png` 和 `pc-dark-mobile-entry-390.png`；H5 为 `h5-list-refresh-390.png`。Chromium 已关闭。

## 冻结与边界

Git 基线 `84b9809cc226fca820714b47c373f1f54a3a0a56`。同目录 `qa-source.sha256` 校验本轮 32 项文件，包括根 CI、接口契约、业务源码、部署和测试。SHA-256 校验及 `git diff --check` 通过。综合审查正文由总控单独绑定。

最终 API 镜像 `sha256:a6d1bc1333cb771e4283bbb32037d056f9bf0df097a7a25d5edd8bb75ac72b49`，最终 Web 镜像 `sha256:b9819d09d345ba264b2a764ad40e5251954c2ad93225826dc4d56dedb5929642`。三个容器健康、当前项目及命名卷保留，未覆盖 `.env`。本轮 QA 未执行远端 CI、真实企业微信或阿里云 OSS 联调。
