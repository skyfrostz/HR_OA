# M2 sequential approval and H5 contract

2026-11-06. Controller-owned extension of OA_API_CONTRACT.md. Preserve all M1 behavior. Base /api/oa/v1, x-token, {code,data,msg}, existing pagination, safe errors and no-store. M2 implements a development workflow, not client-approved final forms or enterprise integration.

## Accounts and permissions

Existing accounts unchanged. Add authority 9004 APPROVER. Like EMPLOYEE it sees only its bound employee/salary/contracts, never all HR records, and cannot create HR records/files. Seed manager, finance, delegate with this authority and their own synthetic employee bindings. Existing seed password mechanism, no password resets or overwrites.

Add booleans canSubmitRequests and canApproveRequests to login/me permissions. A current enabled EMPLOYEE or APPROVER with a bound employee can submit. Only current enabled APPROVER users may act on assigned tasks. HR_ADMIN/HR_VIEWER have no implicit access to everybody's applications or decisions. All participants still use sys_users/sys_authorities. Do not trust role/assignee from JWT or caller body.

## Development templates and immutable snapshots

GET /request-templates returns data {list: Template[], integrationMode:"mock"}, available to submitters and approvers.
Template: {type,name,version:1,description,route:RouteStep[]}. The three fixed demo types and form shapes:
- LEAVE / 请假申请: form {startDate,endDate,reason}. Dates YYYY-MM-DD, start<=end, reason 1..1000 characters.
- EXPENSE / 报销申请: form {expenseDate,amountCents,reason}. Date YYYY-MM-DD, integer amountCents 1..9007199254740991, reason 1..1000. No payment execution or receipt upload in M2.
- GENERAL / 通用申请: form {content}, 1..2000 characters.
All requests additionally have title 1..100 characters. Trim text, reject unknown fields and invalid type-specific fields. No employee/salary fields are copied into approval payloads. Use the M1 business-date conventions.
RouteStep: {step:1-based integer,name,assigneeId,assigneeName}. Every demo template resolves server-side to manager (负责人审批) then finance (复核审批). The caller cannot choose route/assignees. Missing/disabled/non-APPROVER approvers or self-approval in the path reject submission with 409 ROUTE_UNAVAILABLE. These seeded users are replaceable development routing, not inferred customer policy.
Snapshot template version, validated form, applicant identity label and both route steps at submission. No editing of a submitted record. Transfer changes the current task, not the original route snapshot.

## Endpoints and response types

POST /requests requires Idempotency-Key; body {type,title,form,previousRequestId?}; immediately submits and returns RequestDetail with HTTP 201. previousRequestId only for the same applicant's REJECTED request; new record and new ID, old record immutable. No draft persistence endpoint.
GET /requests?page=&pageSize= returns the current user's own RequestSummary list (never a global HR feed).
GET /requests/:id returns RequestDetail. Allowed only to applicant or a user who has actually been assigned a task on this request (current or historical, including transfer-away). Future route membership alone is not access. Inaccessible requests return 404.
GET /tasks?page=&pageSize= returns current actor's PENDING tasks only, using TaskView items.
GET /tasks/history?page=&pageSize= returns current actor's completed decision/transfer ActionView items. This history records the actor's actual past action, not the task's later status.
GET /tasks/:id/transfer-targets returns data {list:[{id,displayName}]}. Only the current pending task assignee with APPROVER role can use it. Candidates must be enabled APPROVER, bound to an employee, excluding current actor and request applicant. Return no salaries/contact info. Transfer to a later route assignee is allowed; it never skips a node.
POST /tasks/:id/actions requires Idempotency-Key; body {action:"APPROVE"|"REJECT"|"TRANSFER",version,comment,targetUserId?}; returns RequestDetail HTTP 200. version is required current task integer version. comment max 1000 characters, required nonblank for REJECT/TRANSFER, optional (empty string allowed) for APPROVE. targetUserId is required only for TRANSFER, otherwise rejected.
GET /notifications?page=&pageSize= returns only current recipient's delivered MockNotification list. No real message API exists.

RequestSummary: {id,type,title,status:"IN_REVIEW"|"APPROVED"|"REJECTED",applicantId,applicantName,createdAt,updatedAt,previousRequestId:null|number,currentStep:null|number}.
RequestDetail extends RequestSummary with {templateVersion,form,route:RouteStep[],tasks:TaskView[],history:ActionView[]}.
TaskView: {id,requestId,requestTitle,requestType,applicantName,step,stepName,assigneeId,assigneeName,status:"PENDING"|"APPROVED"|"REJECTED",version,createdAt,completedAt:null|ISO-UTC}.
ActionView: {id,requestId,requestTitle,requestType,taskId:null|number,actorId,actorName,action:"SUBMIT"|"APPROVE"|"REJECT"|"TRANSFER",comment,targetUserId:null|number,targetName:null|string,createdAt}.
MockNotification: {id,eventId,requestId,kind:"TASK_ASSIGNED"|"REQUEST_APPROVED"|"REQUEST_REJECTED",title,createdAt,deliveryMode:"mock"}. title is a fixed generic action label (e.g. 有新的待办申请), not form contents. Clicking retrieves fresh authorized detail. A transfer-away recipient may retain an old notification, but cannot act as current assignee.
Numeric IDs, UTC ISO timestamps and empty arrays (not null) follow M1 conventions.

## State transitions and authorization

Submit creates IN_REVIEW request, immutable snapshots, first PENDING task (version 1), SUBMIT audit and TASK_ASSIGNED outbox atomically.
APPROVE completes current task once. If another snapshotted route step exists, validate its currently enabled approver role, create exactly one next task and TASK_ASSIGNED outbox; otherwise finish request APPROVED and notify applicant. If the next approver is unavailable, return 409 ROUTE_UNAVAILABLE without partially approving the current step.
REJECT completes current task, terminates request REJECTED, notifies applicant; no later task is created.
TRANSFER leaves current task PENDING, changes only current assignee, increments version and appends audit/assignment history plus new TASK_ASSIGNED outbox. Old assignee immediately loses decision authority. Bound transfer count to 20 per request (409 TRANSFER_LIMIT); do not allow self-transfer or transfer to applicant. No automatic bypass/skip for identical actors in adjacent nodes.
Recheck live authorization and task state under the transaction lock. Wrong/noncurrent actor returns 403; stale version or resolved task returns 409 STALE_TASK. Task/list/detail never leak other applicants to nonparticipants. No administrator override.

## Idempotency and transaction contract

Both M2 POST endpoints use a client intent key: 16..128 ASCII letters/digits/_/- (UUID accepted). Scope UNIQUE(actorId,method+concrete route,key). Persist canonical validated request hash, HTTP result status and response. Same intent reused with different payload => 422 IDEMPOTENCY_KEY_REUSED; missing/invalid key => 400. Same key/body after success replays exact original response/status, sets Idempotency-Replayed:true, and performs no second action/audit/outbox.

Authenticate before replay; replay must not bypass current account disabling or role authorization. Prior successful task action may replay for its actor even after task moved, because it is not a new decision. Keep keys indefinitely in M2 (no TTL cleanup). Failed rolled-back operations may be retried with the same key.

Enforce unique intent claim in MySQL, lock request then task consistently, check expected version/status, and commit business state + audit + outbox + stored result in one transaction. Concurrent duplicate intent may briefly wait for the first transaction, then replay; bound lock/transaction wait and return 409 IDEMPOTENCY_IN_PROGRESS when unresolved. Different keys racing on one task must allow only one transition; losers return 409 (or 403 if transferred away). Do not use in-memory locks as the sole guarantee, SQLite, or blind automatic HTTP retries. Unique DB constraints additionally prevent duplicate tasks per request/step and duplicate outbox/event delivery.

## Mock notifications and runtime lifecycle

A background outbox worker uses the same MySQL. Delivery adapter is explicitly mock, with no network calls. Atomic state changes do not depend on delivery success. Worker persists attempts/backoff, retries pending events across restart, and inserts one mock inbox receipt per event (unique eventId). Delivery and status update use transactions; multiple workers/iterations must not duplicate receipts. No approval content/salary in messages/logs.

bootstrap.Config adds MockNotifyFail bool; controller main reads OA_MOCK_NOTIFY_FAIL == "true". This development fault mode makes mock delivery fail while preserving pending events and increasing attempts. Switching it off and restarting drains the same outbox without duplicating decisions/audits/notifications. No public fault injection endpoint. Default false. Worker is cancellable; returned http.Handler also implements Close() error to cancel/wait worker and close its DB pool. Existing New(Config)(http.Handler,error) remains compatible. Controller defers Close after HTTP shutdown.

## H5 behavior and scope

web/h5.html, independent Vue 3/Vant entry built alongside oa.html. Same API, memory-only login, clear local-development login and mock-notification labels. Screens: templates/new request; my applications; pending; action history; detail with form/route/action timeline; notification inbox. Refresh requires login, no PII/token/form persistence in browser storage.

Generate key once per user intent, disable duplicate clicks, preserve exact key/body for explicit retry after uncertain network results. Do not generate a new key for a retry. On 409 stale task refresh detail/queue. On 422 show key conflict. Rejected resubmission starts a new intent with previousRequestId and new editable form values.

Do not claim conditions, parallel approvals, workflow designer, real enterprise SSO/messages, payments, approval attachments, or customer-final form definitions. M1 PC/private attachments and all existing rows must remain functional.
