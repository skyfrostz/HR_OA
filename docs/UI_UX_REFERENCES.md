# UI/UX 参考资料落实清单

2026-11-08。对应用户截图评审附件，文件 SHA-256 为 `ec75178b9705d38142a6203726fa2dd1cda31eea65344162a3088dee9af2b747`。本页区分下载/阅读与实现/验收；前者不代表后者通过。页面规则统一进入 [设计契约](UI_UX_DESIGN_CONTRACT.md)，实际浏览器证据由验收报告记录。

## 仓库与学习范围

| 来源与固定提交 | 已核对源码 | 用于本项目 | 复用边界 |
| --- | --- | --- | --- |
| [Pure Admin](https://github.com/pure-admin/vue-pure-admin/tree/b8177a202dceb1412d3bf56f3d15483dc1606c9c) | 已有 `web-pure-admin/` 导入及来源清单 | 外壳、主题、Element Plus 组件一致性 | 继续使用已有 MIT 源码及归属记录 |
| [Vben Admin](https://github.com/vbenjs/vue-vben-admin/tree/53b47f8ce99ecea11aa04c9c876a2f18c479838f) | `docs/src/demos/vben-form/query/index.vue`、`apps/web-ele/src/views/demos/form/basic.vue` | 紧凑查询、折叠低频条件、字段宽度及关联分组 | MIT；只参考结构，不搬整套 schema 引擎或 Ant 组件 |
| [Soybean Admin](https://github.com/soybeanjs/soybean-admin/tree/d2dfefd757e37d8d6077ea1cea6181fb628d6097) | `src/components/advanced/table-header-operation.vue` | 表格工具栏动作归组、刷新反馈、列设置归属 | MIT；保留 Element Plus，不引入 Naive UI |
| [Horilla](https://github.com/horilla/horilla-hr/tree/bba4b37bb4538ec1b9f149d10a3def64fc541c7a) | `employee/templates/employee/update_form/form_view.html`；参考册 R367/389/435 | 名册密度、合同待办、独立员工编辑任务 | LGPL-2.1；仅学任务组织，无源码复制 |
| [Frappe HR](https://github.com/frappe/hrms/tree/8816102577c92979df49c20a1807b8c7ab948eb3) | `frontend/src/components/RequestPanel.vue`、`LeaveBalance.vue`；参考册 R460/462/464/465 | 个人/团队任务分工、余额与历史入口、记录正文和辅助区 | GPL-3.0；仅学结构，不复制实现，也不照搬装饰图表 |
| [Yudao](https://github.com/yudaocode/yudao-ui-admin-vue3/tree/a33cdfd565edae4f866901bf6105c7087b0697c4) | `src/views/bpm/processInstance/detail/ProcessInstanceTimeline.vue`；参考册 R289/331 | 申请正文、审批步骤、历史分层 | MIT；保留当前顺序审批，不引入 BPMN 引擎 |

Vben/Soybean 新增稀疏克隆位于本机 `~/.agent-reach/hr-oa-design-20261108/`；Horilla/Frappe/Yudao 复用并补齐 `~/.agent-reach/references/hr-oa/` 的固定克隆。文件及许可哈希记录于 `UI_UX_SOURCE_MANIFEST.json`。第三方应用没有在项目运行，也没有安装其依赖。Inspira 已有参考克隆，本次不新增其装饰组件依赖。

## 已安装、读取并转交前端的技能

| 技能 | 固定来源 | 实际用途 |
| --- | --- | --- |
| Impeccable v4.8.1 | `pbakaus/impeccable@924bbd194d02e5e15a227e75b5901fb0d7a4ecad`；Apache-2.0 | Operate 模式，按 critique、layout/typeset、clarify、adapt/harden、polish/audit 检查；已运行 context，最终按真实修改运行 detect |
| frontend-ui-engineering | `addyosmani/agent-skills@301b02b12bff98e4688e6984af6556802322bbde`；MIT | 将参考转成页面任务、状态、响应式和组件契约；React 示例不改变现有 Vue 架构 |
| anthropic-frontend-design | `anthropics/skills@2c5958ecac7cee2b583967a2c4981d0f4a75c579`；Apache-2.0 | 明确任务、约束、实现及截图复查；别名安装以保留原有 frontend-design 技能 |

安装在本机 `~/.codex/skills/` 对应目录。当前会话显式读取即可应用；自动技能发现通常从下一轮载入。没有启用 Impeccable Live/hooks，也没有改变执行权限。用户已给出明确产品要求，`PRODUCT.md` 直接记录这些已确认事实，`DESIGN.md` 仅指向唯一设计契约。

原有 `frontend-design`、`apple-design`、`vue-best-practices` 继续按适用范围使用。Apple 相关规则仅用于及时反馈、空间一致性、焦点、触控与减少动态效果；不会增加与业务无关的弹簧、玻璃或大动效。

[UI UX Pro Max](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) 的现有应用主题流程已获取并核对：检查版本/组件 → 复用语义 tokens → 按组件替换硬编码 → 验证主题与状态。本次采用这些检查规则，未再安装重复的风格生成器；符合附件“不需要全部安装”的范围。

## 文章转成的执行约束

- [NN/g 数据表格任务](https://www.nngroup.com/articles/data-tables/)：员工表支持查找、比较、查看/编辑及操作，不能全部替换为大头像卡片。
- [Ant Design 表单规范](https://ant.design/docs/spec/research-form/)：同类字段同组件、有关字段分组、文案短而明确；短表单直接填写，真正分阶段的导入才显示阶段。
- [Refactoring UI](https://medium.com/refactoring-ui/7-practical-tips-for-cheating-at-design-40c736799886)：通过位置、字重和分组建立层级，减少无用途边框，主次按钮有区别。采用前检查实际对比度，不照搬过浅辅助文字。
- [Vercel Web Interface Guidelines](https://vercel.com/design/guidelines)：键盘/焦点、标签、失败保留输入、加载/空态、长文本和响应式作为交互检查项。搜索隐私、服务端账目与审批真实结果优先于通用 URL 状态或乐观更新建议；保留缩放。

## 本项目视觉起点

保留品牌蓝和用户主题，中文系统字体，正文约 14px、手机输入约 16px；标题约 24–28px、区块标题约 16–18px；间距 4/8/12/16/24/32；桌面内容边距约 24px、手机约 16px；控件约 36px、手机主要命中区域以 44px 为目标；表格约 48/56px；圆角约 8–10px。具体值按真实布局调整，不作为机械 KPI。

浅色明确页面与内容表面，深色明确页面、内容、浮层；状态同时用文字表达。正文与控件要用实际计算样式检查对比度。44px 是本项目触控目标，不将其冒称为 WCAG AA 的通用数值。

## 实施追踪

前端负责所有前端修改、参考应用映射及组件状态；总控负责来源、设计契约审阅、后端兼容与最终集成；QA 负责冻结基线、真实运行环境及同条件前后截图。下载和技能安装已完成；页面及最终验收仍以对应提交和实际证据为准。
