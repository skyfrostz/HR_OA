# HR OA 系统 UI/UX 改造

- 提出日期：2026-11-08
- 状态：implemented；本地验收通过，远程发布由总控执行
- 范围与验收：[docs/UI_UX_SCOPE.md](../../../../../docs/UI_UX_SCOPE.md)

用户要求基于当前仓库完成全部业务界面的布局与任务流程整理，并修复影响体验的真实前后端问题，不停在审查、配色或首页。优先员工列表/编辑、审批创建/处理、手机长表单，再推广全路由。用户进一步指定所有前端修改由原前端会话统一处理，转交要求必须完整精确；总控负责接口、设计约束和最终验收。

保留 Pure/Vant、蓝色品牌和用户主题；权限、历史、续约关联、账目和模拟投递语义不变。采用同条件前后截图，真实 API/浏览器验收及明确真实设备、外部服务限制。源码发布授权沿用既有 main 约定，不包含生产部署。

补充要求：用户指出附件中的参考仓库、技能及视觉规范必须实际执行，再用于前端。已补齐 Vben/Soybean 固定源码，复核 Horilla/Frappe/Yudao，安装 Impeccable、frontend-ui-engineering、anthropic-frontend-design；版本、许可与应用范围记录于 `docs/UI_UX_REFERENCES.md` 和 `docs/UI_UX_SOURCE_MANIFEST.json`。唯一规范为 `docs/UI_UX_DESIGN_CONTRACT.md`。

实际后端增量及回退顺序见 `docs/UI_UX_API_CONTRACT.md`。历史 P56/P113 空白在当前基线未复现，不能报告为已修复；合同提醒连续点击导航竞态已交前端修复及 QA 复测。全部 13 业务路由已实施并完成真实运行对照、关键业务和主题/窄屏验收。证据见 docs/UI_UX_VALIDATION.md、docs/UI_UX_SCREENSHOTS.md；59 前端文件冻结，未改依赖/锁文件。客户真实服务、物理手机/键盘、生产部署仍 NOT RUN。
