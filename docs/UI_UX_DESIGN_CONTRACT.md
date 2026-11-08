# HR OA UI/UX 设计契约

2026-11-08 · 项目级实施契约，已由总控审核；验收状态见 UI_UX_VALIDATION.md。历史截图基线为 3e11a2a，当前代码不回退。

## 技术与职责

PC 使用现有 Pure Admin、Vue 3、Element Plus；H5 使用现有 Vue 3、Vant。保持 Composition API、原路由、内存会话、权限和服务端业务语义；不增加依赖或第二套组件库。共享样式由前端集成人统一管理，模块仅添加局部布局规则。API 仍为 `{code,data,msg}`；分页仍为 `{page,pageSize,total,list}`。

## 页面类型与状态所有权

| 类型 | 页面 | 规则 |
| --- | --- | --- |
| 工作台 | /dashboard | 小型真实计数与受权限限制的待处理对象；没有数据显示空态 |
| 列表浏览 | /employees、/departments、/contract-reminders、/notifications | 紧凑标题、搜索、按需筛选、数据、所属分页；详情返回保留筛选与页码 |
| 查看/任务 | 员工抽屉、合同、账户、/approvals、H5 | 快速查看保留摘要；编辑/创建以独立任务模式替换浏览内容，不把导航往下推 |
| 阶段任务 | /employee-import | 选择文件、校验预览、确认、结果；预览属于指定文件，过期或替换即失效 |
| 规则/账目 | /workflow-settings、/my-leave、/approved-leaves、/workflow-exceptions | 可用余额优先；规则与步骤分层；列表分页不嵌入编辑步骤 |
| 集成管理 | /integrations、企业微信绑定、移动入口 | 实际配置/模式优先；技术指南和诊断次级展开 |

路由/工作区拥有查询、选中记录和任务模式；表单拥有未提交输入与校验；`useRemote` 管请求取消与加载；`useVersionedWrite`/审批 intent 管并发、未知结果与重试。关闭任务不得隐式提交；失败保留输入。

## 语义 tokens 与主题

扩展现有 `--oa-*`，映射 Element Plus 的 `--el-*`：background→bg-color-page，surface→bg-color，border→border-color-light，text→text-color-primary，muted→text-color-regular，primary→color-primary，focus→color-primary，status→现有语义颜色。用户主题选择是唯一来源，不建立平行主题。H5 继续映射现有 Vant/oa tokens。

间距使用 4/8/12/16/24/32；桌面内容边距 24px、窄屏 16px。中文系统字体，正文 14px、手机输入 16px。普通控件桌面约 36px，主要手机命中区域 44px。表格单行约 48px、双层约 56px。内容圆角 8px，分组以标题/分隔线为主，不层层加阴影。浅色、深色、弹层、日期、下拉、错误状态都必须覆盖。

## 组件规则

- 页面头仅标题、必要一句说明和动作；避免重复 hero/大统计区。过滤条保留搜索，复杂条件按需展开。
- 保留真实表格比较能力，手机可使用同数据的紧凑条目。长中文、文件名、原因允许换行；按钮不被压成逐字换行。
- 表单单列或相关字段两列；长原因/附件跨行；保存/取消位置稳定。原生日期需中文说明，组件日期显示统一年月日。
- 上传隐藏浏览器英文默认文案，以可键盘触发的中文选择按钮和文件名/状态呈现。信息权限完全沿用 API。
- 分页只属于对应列表；单页显示数量、不显示无用翻页。年份选择不等于分页。
- 加载有可感知状态，错误带重试且不伪装空态，筛选无结果与真正无数据可区分。
- 状态用文字+颜色；提示标题/描述使用主题可读正文色，保留状态图标与背景；主色文字按钮/链接使用可读强调前景，与填充主按钮、禁用状态分开。实际渲染的文字/背景抽样达到 4.5:1，不能拿标签或邻近控件的结果代替。审计默认真实变化字段，完整记录可展开，不改原始记录。
- 普通通知以业务事件、对象、时间和动作排序；技术投递信息在详情/管理员诊断中。

## 状态词典与兼容边界

合同草稿/生效/终止等业务状态与按日期计算的到期分类分开。续约创建关联草稿，旧合同和附件不覆盖。审批待处理/通过/驳回/撤回依服务端；配置变更不重算历史快照。额度区分可用、已用、预占及流水。通知 MOCK=模拟记录，ACCEPTED=API 已接受，UNCERTAIN=结果未知，FAILED=处理失败；接受不代表送达/已读。不可新增所有写操作统一必填原因。

## 桌面/H5 与可访问性

PC 审批浏览左侧列表约 340px，详情使用剩余宽度；创建隐藏无关列表。窄屏 PC 不显示多标签栏。H5 列表/详情分屏，任务期间隐藏全局 tab/快捷区；动作与安全区有对应留白，不用 body overflow:hidden 遮盖溢出。主输入首屏可达，最后字段/提交需真实视口滚动检查。

键盘可达、有可见 focus，所有输入关联标签，错误与字段/表单关联；弹层支持关闭与焦点返回。减少动效偏好下取消非必要过渡；不禁止缩放。真实软键盘与物理设备未实测时必须 NOT RUN。

## 页面与验证清单

13 个业务路由：dashboard、employees、departments、employee-import、contract-reminders、approvals、mobile-approvals、my-leave、workflow-settings、approved-leaves、workflow-exceptions、integrations、notifications；另含登录、员工详情 tabs/合同与附件任务、H5 全部工作流。

代表场景先验证员工列表/编辑、审批创建/处理、H5 长表单，随后推广全部路由。主要视口 1440×900、1366×768、390×844，补查 320/360；浅深主题、0/1/多页、长文本、1/10步骤、加载/失败/无权/筛选空结果。截图需相同角色、数据、状态和视口；QA 从冻结运行镜像取 before 后才重建。P56/P113 先复现；H5 单页分页已存在隐藏逻辑，不能标作新修复。

主栈由 QA 管理；前端 build/typecheck/test 与真实 API/浏览器验收分别报告。真实企业微信/OSS、真实设备键盘不能用 mock/桌面模拟替代。

## 固定参考的实际采用

采用已安装 Impeccable v4.8.1（924bbd1）的 Operate 模式、craft-floor/layout/typeset/adapt/harden；Addy frontend-ui-engineering（301b02b）的参考证据→契约→既有组件实现→状态验收流程；Anthropic frontend-design（2c5958e）的信息层级与克制规则。原中文系统字体、品牌蓝、业务真实性优先于通用营销建议。root 管 PRODUCT.md/DESIGN.md 指向本契约，不建立第二套主题；最终 detector 由 root 统一执行，未安装 hooks/live。

| 参考/经验 | 对应实现 | 验证证据 |
| --- | --- | --- |
| Pure Admin 原基础 | 沿用 layout/router/theme/Element Plus；手机隐藏多标签但不改主题偏好 | layout/index.vue、lay-content；QA 窄屏 |
| Horilla R367/R389/R435 | employee-workspace 合并名册前置结构；dashboard-queue 真实合同/待办对象替代单纯大数字 | 同条件表头/首条位置与角色 API 验证 |
| Frappe R460/R462/R464/R465 | employee-detail 紧凑身份+业务 tabs；员工编辑、合同任务分离；H5 单任务布局 | 编辑后 tabs 不再被长表单推动；末尾控件可达 |
| Vben/Soybean R170/176/182/232/276–277 | 两列关联字段、全宽原因/附件、统一中文日期、紧凑筛选与所属分页 | 日期公开 v-model 测试；长中文/窄屏 |
| 芋道 R289/R331 | 审批摘要/内容/有序进度/历史分组，候选人分页与步骤分离 | 1/10 步规则与审批详情 |
| Vercel / Addy / Impeccable | 44px手机命中、focus、保留输入、状态/空错层级、缩放、无嵌套装饰 | 键盘/焦点、实际 viewport 截图与对比度 |
| Refactoring UI / Ant 表单 / NN/g 表格 | 用间距和字重组织信息、保留可比较的表格、动作分主次 | 共享 style.css 与页面实际渲染 |
| Inspira | 仅保留必要反馈，后台不采用粒子/3D/装饰大动效 | 无新增动画或依赖 |

所有参考只借鉴信息组织，不复制 GPL/LGPL 源码、品牌或素材。Vercel URL 状态/乐观更新不覆盖本项目隐私和真实提交契约：姓名、工资、令牌不写 URL；写入成功仅依 API 结果。
