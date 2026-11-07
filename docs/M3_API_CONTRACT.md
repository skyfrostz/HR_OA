# M3 local business completion contract

2026-11-07. Extends M1/M2, preserving their URLs, fields, identities, records and file bytes. Same `/api/oa/v1`, `x-token`, envelope, pagination, safe errors and no-store. This phase is local business functionality, not enterprise/cloud integration or production release.

## 1. Departments and employee editing

DepartmentView: `{id,name,parentId:null|number,version}`. Unique trimmed name (1..128 characters), optional parent. No delete/disable endpoint in this phase. Reject missing parent, self-parent, descendant cycles and a hierarchy deeper than 20. HR_ADMIN writes; HR_ADMIN and HR_VIEWER read management list. Other roles receive 403 for department management APIs; their employee detail still includes department name.

- GET `/departments?page=&pageSize=` returns Page<DepartmentView>, ordered by ID. This is a paginated flat list, parent IDs represent the hierarchy.
- POST `/departments` body `{name,parentId?}` returns DepartmentView 201.
- PUT `/departments/:id` body `{name,parentId,version}` returns DepartmentView 200. A stale version returns 409 STALE_RECORD. Rename updates the denormalized employee department labels consistently in the same transaction and increments affected employee versions.

EmployeeView adds `departmentId` and `version` (positive integer). Existing rows and seeded rows must be backfilled to department records without altering their displayed department text, user binding, IDs, salaries or dates. This migration must be restart-safe. Existing create payload with `department` string remains valid: resolve or create its named department transactionally, preserving old M1 behavior. New clients may send `departmentId` instead; if both name and ID are present they must agree. Department ID must exist.

- PUT `/employees/:id` body is EmployeeInput plus required `version`; full editable employee fields, same validation as create. HR_ADMIN only; no user/account binding changes. Return EmployeeView 200, increment version exactly once. Concurrent/stale edits return 409 STALE_RECORD; duplicate employee number 409 remains. Failure rolls back all fields.
- Add permission booleans `canManageDepartments`, `canManageAnnualLeave`, `canImportEmployees`, `canExportEmployees`; the first three only HR_ADMIN, export true for all four supported roles (same row/salary scope as list).

PUT/create UI must prevent duplicate clicks. Unknown write results require reading the current record before another intent. Do not automatically overwrite a stale version.

## 2. Annual leave records

This is a manually maintained yearly record, not statutory accrual, employment advice, approval auto-deduction or payroll calculation. Store half-day integer units so values have exact precision.

AnnualLeaveView: `{id,employeeId,year,entitledHalfDays,usedHalfDays,remainingHalfDays,note,version,updatedAt}`. `year` 2000..2100, half-day values integer 0..732, used <= entitled, note trimmed <=1000 characters. Remaining is calculated, never accepted from caller. Unique employee/year. No delete endpoint.

- GET `/employees/:id/annual-leave?page=&pageSize=` returns Page<AnnualLeaveView>, year descending. Same employee row isolation as employee detail; inaccessible employee 404.
- PUT `/employees/:id/annual-leave/:year` body `{entitledHalfDays,usedHalfDays,note,version}`; HR_ADMIN only. `version:0` creates an absent year, positive version updates an existing record. Create collision/stale version ->409 STALE_RECORD. Return AnnualLeaveView 200. Two concurrent updates must not silently overwrite each other.

PC employee detail shows these records; only admin can maintain them. Label as manually maintained, display values divided by two in days. Viewer/employee/approver read within their existing row scope.

## 3. Excel import / export

Use the already pinned Excelize v2.9.0 where sufficient. `.xlsx` only, max uploaded file 2 MiB, one worksheet named `员工导入`, exactly six headers in this order: `工号`, `姓名`, `部门`, `出生日期`, `入职日期`, `约定工资（元）`. Up to 1000 nonempty data rows; use worksheet row numbers in errors. Dates are literal YYYY-MM-DD strings, salary is a nonnegative decimal with <=2 fraction digits and existing safe integer cents limit. Template cells should be text formatted. Reject formulas (including cached formulas), external-link content and invalid/encrypted/corrupt files; bound zip expansion, worksheet dimensions and parsing to avoid oversized archives. Unsupported shape/size ->400/413. No legacy `.xls`, macros or multiple sheets.

All import APIs HR_ADMIN only. Import only creates employees, never overwrites or binds accounts. Department names must already exist at preview and confirmation. Validate every row, including duplicate employee numbers inside file and against DB, and return per-row errors. UI does not send normalized employee records back for confirmation.

- GET `/employees/import-template.xlsx` returns authenticated XLSX template with headers and blank text-formatted input cells, no fake employee data.
- POST `/employee-imports/preview`, multipart field `file`, returns 201 ImportPreview.
- GET `/employee-imports/:id` returns the caller's ImportPreview; inaccessible 404.
- POST `/employee-imports/:id/confirm`, body `{}`, returns 200 `{previewId,importedCount,employeeIds}`. The opaque preview ID is the durable idempotency identity: lock preview, recheck current enabled HR_ADMIN and owner, revalidate all rows against live database, then create all rows and save the result in one transaction. No partial import. Repeat confirmation after success returns the same result without duplicate rows. Different concurrent confirm calls also create at most once. Invalid preview ->409 IMPORT_INVALID; data changed/conflicts since preview ->409 IMPORT_CONFLICT; expired uncommitted preview ->410 PREVIEW_EXPIRED. A committed preview keeps its result available for replay. Successful confirmation must not be replayable by a disabled or no-longer-admin actor.

ImportPreview: `{id,createdAt,expiresAt,status:"PREVIEW"|"COMMITTED",totalRows,validRows,errorRows,canConfirm,rows:[ImportRow]}`. Opaque cryptographically random ID, 30-minute validity for uncommitted preview, DB-owned payload tied to creator. ImportRow: `{row,values:{employeeNo,name,department,birthDate,hireDate,salary},errors:[{field,message}]}`. Values are strings for review; the server separately owns normalized validated data. No file/HR content in logs, URLs or browser persistent storage. Raw uploaded file need not be retained after parsing. No preview deletion/TTL job claim in this phase.

- GET `/employees/export.xlsx` returns authenticated XLSX of all currently authorized rows, maximum 5000 (409 EXPORT_LIMIT if exceeded; do not silently truncate). Admin gets salary, viewer does not receive any salary column or values, employee/approver gets only bound employee with salary. Column names: 工号、姓名、部门、出生日期、年龄、入职日期、工龄 and 约定工资（元） only when allowed. All user text written as literal string cells, never executable formulas. Response has correct XLSX MIME, attachment disposition, no-store and nosniff. No public export file URL.

Frontend preview displays row errors and disables confirmation unless valid. Unknown confirmation result retries the same preview ID. Download through authenticated blob requests, revoke object URLs. Workbook generation/parse tests include formula-looking employee strings, actual formulas, cross-row scope and salary omission.

## 4. Contract expiry reminders

GET `/contract-reminders?days=30&page=&pageSize=` returns Page<ContractReminder>. `days` integer 1..90, default30; business calendar Asia/Shanghai. Include SIGNED or EXPIRED contracts whose end date <= today+days, including overdue rows. Exclude DRAFT/TERMINATED and contracts superseded by a SIGNED successor (previousContractId points to it) whose end date is later than the old contract and >= today. Apply existing employee row scope; no salary fields. Ordered endDate ASC then ID.

ContractReminder: `{contractId,contractNo,employeeId,employeeNo,employeeName,endDate,daysRemaining,status}`. Days remaining is signed calendar-day difference: zero today, negative overdue. Keep stored contract status unchanged by reading reminders. Pure PC provides a real reminder list/entry with due/overdue status, refresh and pagination; opening a row goes to authorized employee contracts without PII in URL. This is an on-screen reminder queue refreshed on access, not an externally delivered notification or an automatic contract-status mutation. Enterprise scheduled messages remain pending platform selection.

## 5. Local backup / restore acceptance

QA owns scripts and isolated restore runtime. Backup a consistent MySQL snapshot plus the matching private attachment tree; briefly quiesce API writes while capturing both and restore service on all exit paths. Preserve existing `.env` and data volumes. Local backup directory 0700, archive/dump files 0600, no secret values in logs. Include integrity manifest and clear restore instructions; archive DB contains sensitive records/hashes and must remain local/ignored.

Restore only to an explicitly separate disposable MySQL/attachment environment. Default restore must refuse the active database/volumes and must never erase current data. Verify employee/contract/approval IDs, original attachment bytes, and authenticated API readback on the isolated restored stack. Do not call a dump alone a tested restore. Record interruption/failure cleanup and leave normal API healthy. Production offsite rotation/encryption/retention and actual cloud object storage remain external work.

## 6. Completion evidence

Backend same-package validation/version tests and build; actual MySQL migration and concurrent edits/import confirmations; XLSX preview/errors/atomic conflict/export field isolation; expired/renewed contract reminder dates; existing M1/M2 regressions; actual Pure PC pages and H5 browser flow; isolated restore evidence. Preserve prior M1/M2 reports, create M3 reports and new hashes. No real employee data, real enterprise messaging, public deployment or remote push.
