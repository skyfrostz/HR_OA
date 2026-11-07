# M1 acceptance scope

2026-11-06. This is a test plan, not a test result. Results belong in `VALIDATION.md` with the commands and environment actually used.

## Required evidence

| Area | Required observable behavior |
| --- | --- |
| Reproducible build | OA Go entry builds from the fixed upstream module; OA frontend builds from its lockfile. |
| Real persistence | API uses MySQL, creates an employee and contract, saves a private attachment, and reads the same IDs and bytes after application restart. |
| Authentication | Missing/invalid/expired tokens fail; login returns the contracted identity; disabled accounts and changed roles are resolved from the database on subsequent requests. |
| Role permissions | Admin can create/read; viewer cannot write and receives no `salaryCents` key in either lists or details; employee sees only the linked employee and their salary. |
| Row isolation | Employee cannot read another employee's detail, contract list or file; inaccessible rows return 404. |
| Input validation | Bad dates, negative/fractional salary, duplicate employee/contract numbers and cross-employee renewal references are rejected without partial business writes. |
| Private files | PDF/JPEG/PNG checks use content and extension; unsupported and over-10-MiB files fail; stored paths are not exposed; downloads require authentication and include attachment/nosniff/no-store headers. |
| Restricted runtime | Only the documented OA routes are registered. Probes of upstream initialization, auto-code, plugin, LLM, example and public upload endpoints fail. |
| Browser integration | Real backend login, employee creation, contract creation, attachment upload/download and viewer/employee visibility work through the OA page. No auth/HR data is stored in browser persistent storage. |
| Retry behavior | Non-idempotent M1 POST requests are not retried automatically; controls prevent repeated clicks and uncertain upload outcomes prompt refresh before retry. |
| Development setup | Existing ignored secrets are preserved; public ports default to loopback; health and private data storage are documented. |

## Evidence boundaries

- A source review does not prove runtime behavior. A frontend mock test does not prove API integration. A successful build does not prove browser behavior.
- Record each command as PASS, FAIL or NOT RUN. Failed attempts remain in reports even when a later fix passes.
- Use only synthetic employees and attachments. Do not record passwords, bearer tokens or connection strings in reports/screenshots.
- Local password login and local private storage are development integrations. They do not prove enterprise SSO, cloud storage or production acceptance.

## Explicitly outside this milestone

M2/M3 retain employee editing, normalized organization management, annual leave records, Excel import/export, reminders, sequential approvals, dedicated Vant H5, enterprise identity/messages, cloud object storage, production security assessment and backup/restore verification. The agreed overall project scope remains unchanged.
