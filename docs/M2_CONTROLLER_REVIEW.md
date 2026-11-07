# M2 controller review

2026-11-06. Source and integration review complete. Runtime acceptance passed; see `VALIDATION_M2.md` and its evidence links.

## Scope

Sequential development approval templates, immutable submission snapshots, approve/reject/transfer/resubmit, durable request idempotency, transactionally recorded audit/outbox, mock inbox delivery, standalone H5 and an upgraded PC visual shell. Canonical behavior: `M2_API_CONTRACT.md`; UI expectations: `M2_DESIGN.md`.

## Source observations

- Submission records a validated form and the server-resolved route. Existing requests have no edit/delete API. Resubmission creates a linked record.
- Mutations persist the idempotency result with the business transaction. The MySQL unique intent index is the durable guarantee; the connection-scoped advisory lock bounds duplicate-request waiting. Raw saved responses use a blob to avoid JSON reordering during exact replay.
- Transitions lock the request before its task, compare current assignee/version/status, and use a conditional update. The actor's current role/account is locked for the transaction. New route assignees and transfer targets are revalidated.
- Request detail access depends on applicant/current task assignment or recorded transfer-away actions. Future route membership alone is not sufficient.
- Outbox polling uses row locks with SKIP LOCKED, a unique event receipt, and one transaction for mock receipt plus delivered status. Payload text is a generic label. The mock adapter performs no external messaging.
- The controlled HTTP entry waits for graceful shutdown before stopping the worker and database pool. Initialization failures close the newly opened pool.
- H5 stores the exact intent body/key in memory across an uncertain response and exposes explicit retry. Sensitive session/form data are not intentionally persisted to browser storage.

## Required runtime evidence

Real MySQL same-key and different-key races, authorization changes, failed outbox insertion rollback, notification failure/restart recovery, M1 record and attachment readback, and PC/H5 browser interaction must pass before M2 acceptance. Screenshots must demonstrate the delivered UI rather than only reference projects.

## Integration finding

The first Chromium run passed submission, transfer and both approval decisions, but could not return the applicant from detail to its list by clicking the already active “我的申请” tab. The workspace listened only to Tabbar model updates; Vant does not emit an update when reselecting the same item. Frontend fixed explicit item selection while preserving the pending-intent lock and avoiding duplicate requests. A regression reproduced the old failure and passed with the fix. The final rebuilt web image passed both same-tab return and explicit back-button scenarios. The first failure log is retained. PC 1440px and H5 390px screenshots have been independently reviewed; no visible horizontal overflow or obscured action controls were observed.

## Limits

These are fixed local demonstration routes and mock notifications. They are not final customer approval policy, enterprise SSO/messages, a workflow designer, payment execution, production backup/recovery, or production security acceptance.
