# HR OA M1 API contract (2026-11-06)

Base `/api/oa/v1`. This is the single controller-owned contract. Uses GVA `{code,data,msg}` envelope: code 0 success, 7 error. Errors also include `data.errorCode` (stable machine string). HTTP statuses 400 invalid, 401 unauthenticated, 403 forbidden, 404 absent/inaccessible row, 409 duplicate conflict, 413 oversized, 429 rate limited (Retry-After), 500 internal. Do not expose SQL or storage paths. Lists: data `{list,total,page,pageSize}` (defaults 1/20, max 100). Auth `x-token` header only; short-lived GVA JWT kept in frontend memory, no cookie auth. Same sys_users/sys_authorities as GVA; no second account database.

## Roles and scope

`HR_ADMIN` authority 9001 reads/writes all employees/contracts and reads salary; `HR_VIEWER` authority 9002 reads employees/contracts without salary, no writes; `EMPLOYEE` authority 9003 reads own employee/contracts and own salary only. Server resolves identity and role from DB per request, checks enabled status, derives own employee from userId, never trusts caller row scope. Salary key is OMITTED when unauthorized. Lists/details/files enforce same scope. Unknown roles denied. No delete/update in M1.

## Routes

- GET `/health`: public data `{status:"ok",database:"ok",integrationMode:"local-development"}`. 503 if DB unhealthy.
- POST `/auth/login`: `{username,password}` -> data `{token,expiresAt,user:{id,username,displayName,role,employeeId},permissions:{canWriteEmployees,canReadSalary}}`. expiresAt ISO UTC. Local development password authentication; NOT enterprise SSO. Login rate limited. No tokens in URL/logs.
- GET `/auth/me`: data `{user,permissions}` same objects. Logout local client clears memory (no server revocation claim).
- GET `/employees?page=1&pageSize=20`: employee list.
- POST `/employees`: `{employeeNo,name,birthDate,hireDate,department,salaryCents}` -> data employee (201). Dates YYYY-MM-DD; salary integer cents >=0, required HR input. employeeNo unique. M1 creation is NOT automatically retryable; unique employeeNo conflicts return 409. Server generates id.
- GET `/employees/:id`: data employee.
- GET `/employees/:id/contracts?page=1&pageSize=20`: contract list.
- POST `/employees/:id/contracts`: `{contractNo,startDate,endDate,status,previousContractId?}` -> data contract (201). status `DRAFT|SIGNED|EXPIRED|TERMINATED`; contractNo globally unique, start<=end; renewal reference must belong to same employee and differ. NOT automatically retryable; unique contractNo conflicts return 409.
- POST `/contracts/:id/files`: multipart `file`, PDF/JPEG/PNG only, content-sniffed plus extension validation, max 10 MiB. HR_ADMIN only, employee scope enforced. -> data file (201). NOT automatically retryable; UI prevents double submit, on uncertain network state refresh contract before retry. Store generated opaque name OUTSIDE any public/static directory.
- GET `/files/:id/download`: authenticated binary with safe Content-Disposition attachment, nosniff, no-store. Same employee row scope. No public or permanent URL.

Employee: `{id,employeeNo,name,birthDate,hireDate,department,age,tenureYears,salaryCents?}`. id numeric; age and tenureYears completed integer years derived on read in Asia/Shanghai; salary never float and is bounded to the JavaScript safe integer range `0..9007199254740991` cents. Date-only validation compares calendar dates, not mixed-timezone instants.
Contract: `{id,employeeId,contractNo,startDate,endDate,status,previousContractId:null|number,files:File[]}`.
File: `{id,originalName,size,contentType,createdAt}`. IDs numeric, timestamps ISO UTC.

## Future approvals (not M1 implementation)

Simple sequential approval snapshot. Mutation key scope `(actorId,route,intentKey)` with unique DB constraint, request hash, 409 in-flight, 422 key/payload conflict, stored response replay; all status/audit/outbox in same MySQL transaction, no blind retries. These endpoints are NOT yet registered or claimed implemented.

## Runtime integration

Backend provides `server/oa/bootstrap` package with `New(config Config) (http.Handler, error)` consumed by `server/cmd/oa/main.go` (controller owns main). `Config` has string fields `DSN`, `StorageDir`, `JWTSecret`, `SeedPassword` and boolean `DevSeed`. Existing GVA modules kept, OA runtime registers explicit allowlist only; upstream init/autocode/plugins/LLM/examples/static upload are NOT mounted. This is the chosen GVA module and dependency tree with Router -> API -> Service -> Model layering. No separate Go module or SQLite fallback.

Development seed is opt-in. Usernames are `hr_admin`, `hr_viewer`, and `employee`; the employee account is bound to exactly one synthetic employee. Seed initialization must be repeatable without overwriting existing business records or resetting existing passwords. JWTSecret requires at least 32 bytes and SeedPassword (when seeding) requires 12–72 bytes. Secrets come from the existing ignored local environment, never API responses or reports. Application restart must preserve database rows and private attachments.
