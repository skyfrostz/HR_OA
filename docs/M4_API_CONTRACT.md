# M4 API 契约

2026-11-07。增量契约。沿用 `/api/oa/v1`、`x-token`、`{code,data,msg}`、`{list,total,page,pageSize}`、UTC ISO 时间、数字 ID 和 JSON 严格字段校验。旧 H5 和既有请求/响应保持兼容。除明确新增字段外，以 M1–M3 契约为准。

## 业务变更审计

- `GET /audit-events?page=&pageSize=&employeeId=&departmentId=&contractId=&type=`。筛选均可省略，组合为 AND，按 ID 倒序。type 为 `SALARY|DEPARTMENT|ANNUAL_LEAVE|CONTRACT|ACCOUNT|EMPLOYMENT`。管理员、查阅员可查授权范围，员工、审批人仅有本人事件。其他员工 ID 不产生可见事件。部门自身事件只对 HR 可见。
- 每项 `{id,type,action,actorId,employeeId,departmentId,contractId,reason,before,after,createdAt}`。对象 ID 无关联时为 null，before/after 是对象或 null。字段为事件白名单：工资 `salaryCents`；部门 `name,parentId` 或员工部门 `departmentId,department`；年假 `year,entitledHalfDays,usedHalfDays`；合同 `contractNo,startDate,endDate,status,version,previousContractId,signedDate,terminatedDate`；账号 `userId,role,enabled`（交接时 after 另含 `handoffUserId`）；在职 `employmentStatus`。不保留原始请求体、凭据、令牌、附件正文和年假备注。
- 工资无权用户完全看不到 SALARY 事件及其数量。旧档案不回填虚构事件。新增员工/导入的工资与部门、调薪/调部门、部门创建/编辑、年假设置、合同创建/编辑/签署/终止/续约、账号与离职均与对应事件同一事务。旧写入端点可传 `reason`（1–500 字符，省略使用操作标签）；新写入必须提供。导入使用固定原因。
- action 取 `CREATE|UPDATE|IMPORT|RENEW|SIGN|TERMINATE|BIND|SET_ROLE|RESET_PASSWORD|DISABLE|ENABLE|OFFBOARD`。员工/审批人可读本人事件。原因文本由操作者填写，仅 HR_ADMIN 读取原文，其余角色返回空字符串，避免其他事件的说明间接泄漏工资。界面应提示不要填写敏感凭据。

## 员工账号与在职状态

- EmployeeView 新增 `employmentStatus:"ACTIVE"|"OFFBOARDED"`。普通档案 PUT 不改变账号与在职状态。历史记录默认 ACTIVE。
- `GET /employees/:id/account`（HR_ADMIN），返回 `{employeeId,employmentStatus,version,account:null|{id,username,role,enabled}}`。version 使用员工版本，所有成功账号操作都递增。
- `GET /employees/:id/account-candidates?page=&pageSize=`（HR_ADMIN），只列未绑定的 OA EMPLOYEE/APPROVER，条目 `{id,username,role,enabled}`。绑定不会更改原角色、密码或启用状态。
- `GET /employees/:id/handoff-targets?page=&pageSize=`（HR_ADMIN），在职、已绑定、启用且角色为 APPROVER 的候选，排除目标员工本人及已保留对外交接映射的账号。条目 `{id,displayName}`。实际提交会再次检查候选与申请人的冲突。
- `POST /employees/:id/account/actions`（HR_ADMIN），返回与 GET 相同。公共字段 `{action,version,reason}`，额外字段按动作校验：
  - CREATE 加 `{username,password,role}`；用户名 3–64 ASCII 字母、数字、点、下划线、连字符；密码 12–72 UTF-8 字节；角色只能 EMPLOYEE/APPROVER。创建并绑定启用账号。
  - BIND 加 `{userId}`；绑定唯一的未绑定 OA 员工或审批人账号。
  - SET_ROLE 加 `{role,handoffUserId?}`；只能 EMPLOYEE/APPROVER。移除审批角色时可交接。
  - RESET_PASSWORD 加 `{password}`。密码只传请求体，不回传、不审计、不持久化明文。
  - DISABLE 加 `{handoffUserId?}`。关闭登录和旧会话。
  - ENABLE 无额外字段。仅在职账号可用。新登录取得新会话。
  - OFFBOARD 加 `{handoffUserId?}`。员工可无账号；有账号时停用并撤销会话，保持绑定、档案、历史。离职后不可启用或开通。
- 角色变更、密码重置、停用、启用、绑定与离职会使操作前签发的 token 失效，重启后仍失效。现有密码和历史记录不改写。禁止操作 HR 管理类账号及自己。禁止覆盖已有绑定。用户名查重包含软删账号。
- 对被移除审批能力的账号，存在待办、进行中申请的未来节点或演示默认角色时，必须提供可用 `handoffUserId`。事务内迁移当前待办、递增任务版本、记录转交及模拟通知；既有未来节点和新申请解析到交接人，保留原始申请快照。受影响的未来节点在申请历史追加 HANDOFF（taskId 为 null）；未来接手人仅在实际分配时获得详情权限和通知。交接链不能形成循环或让申请人审批自己。重新启用不会倒转已经完成的交接。拒绝有冲突的交接，业务与审计均回滚。未涉及任何交接时提交非空 handoffUserId 返回 400。

## 合同

- ContractView 增加 `version,signedDate:null|YYYY-MM-DD,terminatedDate:null|YYYY-MM-DD`。旧合同版本初始为 1，未知历史签署/终止日期保持 null。
- `GET /contracts/:id`：同员工合同列表行权限，返回 ContractView 含文件列表，支持提醒直接定位，越权 404。
- `PUT /contracts/:id`（HR_ADMIN）：`{version,reason,contractNo,startDate,endDate}`，只允许编辑 DRAFT。续约关联不可编辑。返回 ContractView。
- `POST /contracts/:id/actions`（HR_ADMIN）：`{action:"SIGN"|"TERMINATE",version,reason,effectiveDate}`。DRAFT→SIGNED 记录签署日期；SIGNED→TERMINATED 记录终止日期。生效日期为 YYYY-MM-DD，不得晚于业务今日，终止不得早于开始日期或已记录签署日期。版本递增。此处为人工签署登记。
- 原 `POST /employees/:id/contracts` 兼容旧状态及 `previousContractId`，可选 reason，新增业务审计。新界面创建/续约使用 DRAFT 后登记签署。续约创建关联新记录，不覆盖旧状态和附件。附件继续按原授权保存和下载。

## 审批查询

`GET /requests`、`GET /tasks`、`GET /tasks/history` 追加可选 query `type=LEAVE|EXPENSE|GENERAL&status=IN_REVIEW|APPROVED|REJECTED&fromDate=YYYY-MM-DD&toDate=YYYY-MM-DD`。status 表示申请状态；待办固定为进行中，其他合法状态返回空。日期含首尾，上海业务日期，分别按申请、任务、动作创建时间筛选。三个列表维持旧的行权限与返回形状。详情、动作、模板、幂等键和重提复用 M2。交接会新增 `HANDOFF` 时间线动作，格式与其他动作相同。

## 组织树

`GET /departments/tree`（HR_ADMIN、HR_VIEWER）返回 `{list:[{id,name,parentId,version,employeeCount}]}`，全量扁平节点，前端根据 parentId 组装。employeeCount 为既有员工可见范围内本部门的直接人数，包含保留档案的离职员工，不重复汇总下级，不用当前页推算。

## 新错误码

- 400 `INVALID_INPUT`：字段、日期、角色、动作参数校验。
- 401 `UNAUTHENTICATED`：旧会话被撤销或登录禁用。
- 403 `FORBIDDEN`：权限不足或受保护的管理账号。
- 404 `NOT_FOUND`：记录不存在或无行访问权。
- 409 `STALE_RECORD`：版本冲突；读取最新数据后确认再提交。
- 409 `INVALID_STATE`：合同、账号或在职状态不允许此动作。
- 409 `ACCOUNT_CONFLICT`：用户名/绑定冲突。
- 409 `HANDOFF_REQUIRED`：需指定交接人。
- 409 `HANDOFF_CONFLICT`：交接对象已不可用、会成环或与申请人冲突。
- 审批既有错误码与回放行为不变。新写操作使用版本检查，旧版本重复提交不产生第二次审计。
