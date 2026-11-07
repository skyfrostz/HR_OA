# 前后端边界说明

<!-- Modified for HR OA on 2026-11-06/07: add OA contracts, identity boundary and Pure PC entry. -->

## HR OA 本地扩展（2026-11-06）

OA 的业务契约为 `docs/OA_API_CONTRACT.md` 及 M2/M3 扩展，接口前缀 `/api/oa/v1`。后端由 `server/cmd/oa` 显式启动，仅注册 OA 允许的路由；当前 PC 来自 `web-pure-admin/oa.html`，H5 来自 `web/h5.html`。沿用 GVA 用户/角色模型与 `{ code, data, msg }` 响应，不引入第二套账号库。OA 通过 `x-token` 头传递内存中的短期令牌，不能复用会持久化令牌的原管理端登录流程。工资字段与员工行权限由服务端执行，前端可见性仅用于交互。

2026-11-07 商业交付审查增量见 `docs/COMMERCIAL_REVIEW_API.md`：新增 POST `/employees/search` 和 POST `/employees/export.xlsx`，搜索条件通过 JSON body 传递，禁止姓名/工号等进入 URL 或持久化存储；列表与导出共享筛选和授权语义。旧 GET 接口保留。文档描述的是本轮约定，实现与真实运行验收须另有证据。

## 归属边界

- 后端负责路由、参数校验、业务逻辑和响应结构
- 前端负责页面流程、交互体验、本地状态和展示层
- 共同行为通过明确的 API 契约协作，不通过隐式约定耦合

## 契约规则

- 保持统一响应结构：`{ code, data, msg }`
- 保持统一分页结构：`{ page, pageSize, total, list }`
- 字段名不要随意漂移
- 前后端字段类型必须保持一致
- 后端必须提供完整而准确的 Swagger 接口说明
- 前端接口调用应以实际 Swagger 与后端实现为准

## 变更规则

- 涉及破坏性接口调整时，要先写清楚变更范围
- Swagger 或其他接口说明必须与真实实现一致
- 前端接口封装应继续放在 `web/src/api/` 或 `web/src/plugin/<name>/api/`
- 可复用逻辑优先复用 `web/src/utils/` 现有能力

## 完成前检查

跨前后端改动结束前，至少确认以下几点：

1. 后端响应结构仍然满足前端预期
2. 前端仍在使用正确的字段名和数据类型
3. 若契约发生了长期变化，对应说明已经补到 `aiDoc/`

M4 增量以 `docs/M4_API_CONTRACT.md` 为准：业务审计、员工账号/在职状态、合同状态转换、审批筛选及部门树；契约由后端维护并经总控复核。M4 视觉与复用要求见 `docs/M4_UI_DESIGN.md`。
