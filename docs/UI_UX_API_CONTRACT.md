# UI/UX 改造 API 兼容增量

2026-11-08。沿用 `/api/oa/v1` 前缀、`{code,data,msg}` 信封、原分页结构及角色/对象级权限。没有数据库迁移、依赖升级或历史数据重算。

## 部门筛选与导出

`POST /employees/search` 和 `POST /employees/export.xlsx` 共用可选字段 `departmentScope`：

- 省略、空字符串或 `DIRECT`：仅选中部门，保持原行为。
- `SUBTREE`：必须同时传有效的 `departmentId`，包括该部门及后代；递归深度沿用组织树上限 20。
- 无效范围或缺少所需部门返回 400。

列表、total、导出使用相同筛选。员工/审批人本人范围、HR 范围及工资字段限制仍在服务端执行。搜索姓名和工号只放 JSON body，不进入 URL 或持久存储。部门树上的 employeeCount 仍是直属人数。

## 导入行错误

原有 `errors[].field,message` 保留，相关错误增加可选 `code`：

| code | 含义 |
| --- | --- |
| DATE_FORMAT | 不符合 YYYY-MM-DD |
| DATE_NOT_FOUND | 日期不存在，如非闰年 2 月 29 日 |
| DATE_OUT_OF_RANGE | 年份超出 1000–9999 |
| FUTURE_BIRTH_DATE | 出生日期晚于业务当天 |
| HIRE_BEFORE_BIRTH | 入职日期早于出生日期 |
| DUPLICATE_IN_WORKBOOK | 当前工作簿中工号重复 |
| EMPLOYEE_NO_EXISTS | 数据库已有相同工号 |

旧预览可没有 code，前端回退到 message。未来入职日期仍允许预录；本次没有新增禁止规则。保留行号、原值、字段、具体原因和错误工作簿。

## 替换文件使旧预览失效

新增 `POST /employee-imports/:id/discard`，JSON body 为 `{}`。仅预览创建者且当前仍为 HR_ADMIN 可调用；其他管理员访问返回 404，无写权限返回 403。

成功返回 200 和原 `ImportPreview` 结构：`status=EXPIRED`、`rows=[]`、`canConfirm=false`；尚未到期时 expiresAt 截为当前时间。源行及待提交快照在同一事务中清除。重复丢弃幂等返回 200，已提交预览返回 409 `INVALID_STATE`。确认及错误导出沿用 410 `PREVIEW_EXPIRED`。

丢弃与确认共用预览行锁，因此并发只能得到“丢弃成功、确认过期”或“确认成功、丢弃冲突”。不重置已提交记录，不失效其他独立预览。前端实际选定替换文件后，先完成旧预览失效，再进入新文件流程；失败保留当前状态和可重试信息，不显示伪成功。

## 发布、回退与证据

先上线支持新范围字段及 discard 路由的 API，再启用对应前端。旧 API 严格解析 JSON，会拒绝未知 departmentScope。回退须先撤前端新参数和新路由调用，再回退 API；已经失效的预览继续失效，不恢复敏感行。旧 API 可读取含额外可选 code 的预览 JSON。

现象—API—权限—兼容—验证逐项关系见 [问题矩阵](UI_UX_BACKEND_MATRIX.md)。验证证据与真实服务限制见最终验收报告；Swagger 来自同一实现，不把文档声明当作运行通过。
