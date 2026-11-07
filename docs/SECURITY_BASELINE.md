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
- Auth token stays in browser memory. No general cookie-auth fallback or OA-token URLs. M6 uses short-lived HttpOnly cookies only for OAuth state and one-time exchange; application APIs still require the x-token header. Return no-store headers on sensitive data.
- Private attachments have random stored names, bounded PDF/JPEG/PNG content checks, and authenticated download with attachment/nosniff/no-store. No storage path in response.
- Main has explicit HTTP timeouts and graceful shutdown; default listens only on loopback. Development seed is opt-in and requires environment secrets.

## M6 additions

- WeCom member mapping is explicit and mode/corporation scoped; disabled or offboarded employees and unmapped external identities cannot establish sessions. Binding changes revoke prior sessions. Broadcast and multi-member selectors are rejected both at binding and immediately before sending.
- Enterprise notifications contain fixed generic text and an event number. Unknown external results remain UNCERTAIN; API acceptance is not proof of delivery or read status. No real customer messages were used in development acceptance.
- Attachments have STANDARD/RESTRICTED classification. Restricted metadata, downloads and previews share authorization. Old attachments remain STANDARD for compatibility and require manual classification if their contents are sensitive.
- PDF preview renders canvas pages from authenticated bytes with script/XFA/eval and annotation links disabled; bundled CMaps and fonts avoid external CDNs. Image dimensions and PDF page geometry are bounded. This is not a general document malware scanner.
- OSS uses the official Go V2 SDK with private-bucket/public-block/versioning preflight. Every object read is behind OA authorization. Migration verifies bytes before changing version-checked records and retains originals; no public or presigned browser URL is returned.
- Production startup rejects development seeds, mock providers, weak configuration and non-TLS database connections. The initial administrator is opt-in for an empty identity store; existing accounts are not overwritten. Proxy trust defaults to none and disallows wildcard networks.
- Startup diagnostics expose fixed stage labels only. CLI configuration validation does not connect to DB/providers. Maintenance migration disables background workers and emits JSON without Gin debug output.

## Evidence status

Source review completed. Runtime route enumeration/negative probes and business integration tests are recorded in VALIDATION.md when actually run. This document does not claim a full dependency vulnerability audit, penetration test or production readiness.
