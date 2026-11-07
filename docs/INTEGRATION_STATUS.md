# External integration status

Updated 2026-11-07.

| Integration | Decision | Current evidence |
| --- | --- | --- |
| Enterprise platform | User selected WeCom (企业微信) | Local development login and mock notifications only; real enterprise login/messages NOT CONNECTED / NOT TESTED. |
| Contract object storage | User selected Alibaba Cloud OSS (阿里云 OSS) | Authenticated local private volume works; no OSS bucket configured or tested. |
| Public deployment | Not requested | Loopback local Compose only. |

## WeCom handoff prerequisites

Prepare a customer-owned application and provide its organization/application identifiers, server-side application credential, allowed member scope and the intended HTTPS deployment/callback origin through the project's secure local configuration channel. Map enterprise member IDs explicitly to the existing OA account/employee identities; never infer or silently create HR administrator permissions. Application credentials belong only on the server, never in `VITE_*`, browser bundles, URLs, screenshots or reports.

Exact provider configuration and endpoint details must be checked against the current official documentation during integration. The official developer documentation could not be accessed in this session, so this document is a handoff checklist, not a claim that provider-specific settings or endpoints have been verified.

Actual login and message delivery require configured enterprise resources and a test member. Until then keep the local adapters clearly marked. No real enterprise messages have been authorized or sent in this development pass. A delivery test needs an explicitly identified recipient and message purpose before sending.

## Cloud storage handoff

For Alibaba Cloud OSS, configure a private bucket, region/endpoint, restricted server credential and approved object prefix. Preserve existing authenticated download and employee/contract authorization semantics. Local attachments must be migrated with a byte-integrity check before switching the active storage backend. No cloud upload or migration has occurred.
