# OA runtime security baseline

Reviewed fixed GVA a6882210a80bb27e3aa5dff0b4c21aa4afe8988a on 2026-11-06.

## Sources and limitations

Official advisory https://github.com/flipped-aurora/gin-vue-admin/security/advisories/GHSA-mqv7-9mvf-h9fh describes public code-generation routes, plugin removal, upload/XSS and other public surfaces. Its patched-version metadata says 2.9.2 while prose names 2.9.3 / 3.0.1; this contradiction is retained, not resolved by assuming stable/latest is secure.

Official advisory https://github.com/flipped-aurora/gin-vue-admin/security/advisories/GHSA-3558-j79f-vvm6 describes traversal in resume-upload finishing (affected <=2.8.7, patched version unspecified).

## Actual startup source review

Upstream `server/initialize/router.go` unconditionally mounts Local.StorePath via StaticFS, swagger, public InitInitRouter, broad admin/system routes, example upload/download and InstallPlugin. `server/initialize/plugin_biz_v2.go` blank-imports plugin registration. This is too broad for the OA contract and is not used by OA entry.

`server/cmd/oa/main.go` imports OA bootstrap explicitly and never calls initialize.Routers/core.RunWindowsServer or the plugin registries. OA router must be an allowlist limited to `/api/oa/v1` documented endpoints. No initialization, code generation, LLM, plugin install/removal, debug, public upload or filesystem static handler is permitted. Upstream source retention does not mean those routes are available.

## Business boundary

- GVA sys_users/sys_authorities identity and roles, x-token JWT only; verify role/enabled state from database on each protected call, plus Casbin method/path authorization.
- Employee row scope is enforced at list, detail, contract and file service access; salary is omitted from unauthorized output rather than hidden only in the UI.
- Auth token stays in browser memory. No cookie-auth fallback, no token URLs. Return no-store headers on sensitive data.
- Private attachments have random stored names, bounded PDF/JPEG/PNG content checks, and authenticated download with attachment/nosniff/no-store. No storage path in response.
- Main has explicit HTTP timeouts and graceful shutdown; default listens only on loopback. Development seed is opt-in and requires environment secrets.

## Evidence status

Source review completed. Runtime route enumeration/negative probes and business integration tests are recorded in VALIDATION.md when actually run. This document does not claim a full dependency vulnerability audit, penetration test or production readiness.
