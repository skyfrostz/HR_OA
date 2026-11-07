# M5 API 契约

2026-11-07，M4 增量。沿用 `/api/oa/v1`、x-token、现有权限、`{code,data,msg}`、`{list,total,page,pageSize}`。UTC 时间戳，业务日期为上海 YYYY-MM-DD。正文中的短路径均在此前缀下。写操作 JSON 严格校验。管理端点限 HR_ADMIN。

## 请假类型、日历与计算

- `GET /leave-types` → `{list:[{id,name,usesBalance,enabled,version}]}`。管理员含已停用类型，其余账号仅启用项。初始化年假（扣额度）与事假（不扣额度）。
- `POST /leave-types` → 201，正文 `{name,usesBalance,enabled}`。 `PUT /leave-types/:id` → 200，同上加 version。返回类型记录。名称 1–60 字符，唯一；布尔字段必填。编辑只影响以后提交。
- `GET /leave-calendar` → `{version,workingWeekdays,exceptions}`。 `PUT /leave-calendar` 提交同形状，按版本完整替换。workingWeekdays 为 1（周一）至 7（周日）的不重复数组，至少一天，默认 [1,2,3,4,5]。exceptions 为 `[{date,working}]`，日期唯一、最多 1000 个，优先于周规则。无内置法定节假日。
- 新 `POST /requests` 的 LEAVE form 为 `{leaveTypeId,leaveTypeVersion,calendarVersion,startDate,endDate,startHalf,endHalf,reason}`。startHalf/endHalf 为 AM/PM 且必填；同日 AM→AM/PM→PM 为半天、AM→PM 为一天。区间首尾均包含，最多 366 个自然日。非工作日跳过；全区间无工作半天拒绝。旧记录及旧成功意图重放兼容；旧记录重提时须补类型和半天。
- `POST /leave-preview`（当前有员工绑定的申请人）正文同 LEAVE form 但省略 reason、leaveTypeVersion、calendarVersion。返回 `{calculation,balances,canSubmit,issues}`。calculation 为 `{leaveTypeId,leaveTypeName,leaveTypeVersion,calendarVersion,usesBalance,startDate,endDate,startHalf,endHalf,totalHalfDays,years:[{year,halfDays}],days:[{date,halves:["AM","PM"]}]}`；years 按实际工作日期所属年分摊。balances 是各涉及年度的余额视图；无额度台账时额度为 0，非额度类型 balances=[]。issues 枚举 `LEAVE_OVERLAP|INSUFFICIENT_LEAVE`，有问题时预览仍 200；无工作半天为 400 `INVALID_INPUT`。
- RequestDetail 新增 `leaveCalculation:null|Calculation`。M1–M4 历史 LEAVE 为 null，仍可处理/查看，无追溯扣减。新申请提交须带预览 calculation 返回的 leaveTypeVersion 和 calendarVersion，过期返回 409 STALE_RECORD 并提示重新预览。服务端重新计算并冻结日历计算及类型语义；已批准及进行中的新版请假按实际工作半天判重叠，已拒/撤回/撤销不占时间。没有历史计算快照的旧申请不参加半天占用判断。

## 年度余额与流水

- 现有年度年假 `usedHalfDays` 仍为人工基线，新增 `approvalUsedHalfDays,reservedHalfDays`；`remainingHalfDays=entitledHalfDays-usedHalfDays-approvalUsedHalfDays-reservedHalfDays`。GET/PUT 年假原路径和版本语义不变。PUT 不允许人工基线降低可用额度到负值（409 INSUFFICIENT_LEAVE）。不自动结转。
- `GET /employees/:id/leave-ledger?page=&pageSize=&year=` 与该员工年假台账同权限。分页按 ID 倒序，year 可省略。每项 `{id,requestId,year,kind,halfDays,createdAt}`；kind 为 `RESERVE|CONSUME|RELEASE|REVERSE`，halfDays 正数。人工基线继续从台账与 M4 审计核对；不为旧申请补造流水。
- 新额度请假提交预占；最终批准从预占转为审批使用；驳回/撤回释放预占；管理员撤销批准冲回审批使用。非额度类型仍检查时间重叠并冻结快照，不产生额度流水。所有年份、申请、任务、审计及 outbox 同事务。相同员工的申请与人工台账写入持同一员工锁，任何一年失败都不部分预占。旧版历史批准不会写流水。

## 版本化顺序审批规则

- `GET /approval-rules?page=&pageSize=`，`POST /approval-rules`，`PUT /approval-rules/:id`。字段 `{name,type,departmentId,minAmountCents,maxAmountCents,priority,enabled,steps:[{name,assigneeId}]}`，更新加 version。创建 201，更新 200，返回字段加 id/version。name 1–100、step name 1–100、1–10 步，priority 整数 1–10000。仅 EXPENSE 可设非空金额上下界，单位分，含边界。部门 null 表示全员，非空匹配申请人直属部门。
- 仅启用规则参与匹配，按 priority 从高到低，启用的同类型同优先级条件若存在交集则 409 `RULE_CONFLICT`。无匹配使用 M4 演示默认顺序。启用规则的审批人须为可用、在职且有员工绑定的 APPROVER，提交时禁止自审批。可重复同一审批人，各步骤独立。规则编辑/停用只影响新申请。RequestDetail 增 `rule:null|{id,version,name}`，route 仍是提交时完整快照；历史、默认规则的 rule 为 null。
- `GET /approval-rules/assignees?page=&pageSize=` → 分页 `{id,displayName}`。 `GET /request-templates` 对有员工绑定者按当前部门返回路由提示，金额相关规则提交时才能确定；服务端提交结果为准。模板读取不赋予申请访问权。

## 撤回、撤销、催办、异常交接

- `POST /requests/:id/actions`，Idempotency-Key 同 M2，`{action,reason}`，返回 RequestSummary。reason 1–500 字符，REMIND 可省略。WITHDRAW 仅本人进行中，最终申请状态 WITHDRAWN，关闭当前任务（CANCELLED），释放新版请假预占；CANCEL 仅 HR_ADMIN 对 APPROVED 新版请假，最终状态 CANCELLED，按快照冲回；响应 title 为空，避免向 HR 暴露私人标题。既有申请/任务读接口新增状态枚举。非本人一般操作返回 404。失败不存成功幂等结果。
- REMIND 仅本人进行中，面向当前可用处理人，使用既有 outbox 和 `TASK_REMINDER` 模拟通知。每申请相邻两次成功催办至少间隔 5 分钟；超限 429 `REMINDER_RATE_LIMIT`；相同幂等键重放不会多发。不可用处理人返回 409 ROUTE_UNAVAILABLE。
- `GET /approved-leaves?page=&pageSize=`（HR_ADMIN）用于管理员选取可撤销的新版已批请假。每项 `{id,applicantId,applicantName,leaveTypeName,startDate,endDate,totalHalfDays,updatedAt}`，不返回私人理由/表单。不赋予全局审批详情权。
- `GET /workflow-exceptions?page=&pageSize=`（HR_ADMIN）。每项 `{taskId,requestId,requestType,step,stepName,assigneeId,reason,version,createdAt}`，每申请仅返回最早异常步骤。taskId 始终是当前待办 ID（未来节点还没有任务）；step/stepName 始终是异常节点。reason 为 CURRENT_UNAVAILABLE 或 FUTURE_UNAVAILABLE。发现进行中申请当前/未来处理人停用、权限或员工绑定不可用，以及后续路径无法解析；不返回表单、私人标题或申请理由。
- `POST /workflow-exceptions/:taskId/handoff`（HR_ADMIN），`{version,targetUserId,reason,step}`，对当前最早异常步骤生效；step 必填且须与列表一致，防止账号可用性恢复后误处理另一异常步骤，返回 `{taskId,requestId,version,assigneeId}`。候选复用规则 assignees 接口；不能选申请人。当前任务改分配及通知；未来步骤追加单申请覆盖与时间线，原 route 快照保留；异常交接的 HANDOFF 动作额外返回 routeStep 与 previousAssigneeId，接手人实际获得任务后才有通知和详情权。当前任务版本递增（包括修复未来节点），响应 assigneeId 为接手人 ID。已恢复正常或版本变动返回 409。保留原 M4 离职交接并让它覆盖启用规则及未来节点覆盖中的账号。

## 工作台

`GET /dashboard?days=30`，days 同合同提醒 1–90。返回 `{pendingTasks,myRequests,contractReminders,workflowExceptions,approvedLeaves}`；计数与同权限列表一致，后两项非 HR_ADMIN 为 null。待办、本人申请、合同、异常、可撤销请假分别链接既有列表或本轮新列表。无全局业务详情外泄。

## 增量错误

审批列表 status 新增 WITHDRAWN、CANCELLED；待办仍只含 IN_REVIEW。

继承 UNAUTHENTICATED、FORBIDDEN、NOT_FOUND、INVALID_INPUT、STALE_RECORD、INVALID_STATE、STALE_TASK、ROUTE_UNAVAILABLE、HANDOFF_CONFLICT、IDEMPOTENCY_*。新增 409 LEAVE_OVERLAP、INSUFFICIENT_LEAVE、RULE_CONFLICT；429 REMINDER_RATE_LIMIT。已有 API 沿用角色、分页和模拟集成边界。

启用规则的审批人不可用时，创建、编辑及按该规则提交返回 409 ROUTE_UNAVAILABLE。配置写入将生成 HR_ADMIN 可读的业务审计事件 LEAVE_TYPE、LEAVE_CALENDAR、APPROVAL_RULE。新建版本为 1，编辑成功版本 +1，记录对象和版本前后值；不存在自动覆盖。
