# M4 源码复用与来源

2026-11-07。本轮复用现有 Pure 组件和本仓库 H5 审批模块，没有增加第二套前端框架或后端服务。原始第三方许可保持不变。

- Pure 现有 `web-pure-admin/src/components/RePureTableBar/src/bar.tsx`，基线同上，首次入库 `84b9809cc226fca820714b47c373f1f54a3a0a56`，原 SHA-256 `af5d258c21d145bc1c94dfea060689690c69ddb1c446d70b81d639b800a12867`。保留现有 `web-pure-admin/LICENSE`。适配为键盘可操作的图标按钮、根据动态列渲染列菜单、减少动态效果时不播放拖动动画；OA 隐藏该表布局未承诺的固定列选项。
- OA 仓库 H5 源码，基线同上；复制到 `web-pure-admin/src/oa/approvals` 并换用 Element Plus 和 Pure session/remote/pager，增加 PC 列表筛选、联动布局、路由写锁、上海时区、交接动作。原始文件和 SHA-256：

  | 文件 | SHA-256 |
  | --- | --- |
  | `web/src/h5/approval.js` | `3a21032c7da19550036d1550155fbbd5ec9ad78de2322eec3f5a0bbcc385f9fb` |
  | `web/src/h5/use-intent.js` | `38ede205500b0828f458dd7873741c3dfa294a9055fe1d2bd795b54c8c47cfd2` |
  | `web/src/h5/record-list.vue` | `d72dbb5d2bdd42a4131515a5305164a95c68a469caf11b594d480c4aa19e6319` |
  | `web/src/h5/request-detail.vue` | `767ec1cbc31dada41499eb7f51900eb4b158d904c348cb533188c351c898b993` |
  | `web/src/h5/request-editor.vue` | `c24b69cf6e776b74a47ffc66c048f9fb367694c099ef4da763ea618f7f18ecb6` |
  | `web/src/h5/task-actions.vue` | `8dc96055332063eedfb9d5d47f6d639bc1a37df4ff538ee50d3e7e6d09eeeccc` |

- 查阅芋道 MIT 固定版本 `a33cdfd565edae4f866901bf6105c7087b0697c4` 的 `src/views/bpm/processInstance/detail/ProcessInstanceTimeline.vue`（SHA-256 `8fe01b6705ba6158a34ac15fea01362147ccf2a540846aaf77b3d5e0c8779b7e`），参考节点标题、时间与人员的呈现顺序。采用已有 H5/OA 源码和原生列表实现，未复制芋道代码、图片、样式或依赖；芋道 checkout 的 MIT LICENSE 和署名未改动。未纳入 Inspira 源码或 motion-v。无新增三方许可证需并入产物。


后端复用已有 GVA 身份和 OA 事务模型，无新增 Go 依赖；Horilla/Frappe 仅作业务边界参考，没有复制其业务源码。Inspira 的选型结论见 [前端设计](M4_UI_DESIGN.md)。参考仓库固定提交与评估路径见 [后续计划](NEXT_PHASE_PLAN.md)。
