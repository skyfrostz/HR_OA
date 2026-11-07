# External integration status

Updated 2026-11-07. The user confirmed that enterprise/cloud resources are not ready and requested complete implementation with simulated integration first. M6 development acceptance and real customer acceptance are separate.

| Integration | Decision | Current evidence |
| --- | --- | --- |
| Enterprise platform | User selected WeCom (企业微信) | M6 implements server OAuth, explicit binding and outbox adapters; simulated results belong in VALIDATION_M6.md. Real enterprise login/messages NOT CONNECTED / NOT TESTED. |
| Contract object storage | User selected Alibaba Cloud OSS (阿里云 OSS) | M6 implements official SDK private storage and verified migration; no customer OSS bucket configured or tested. |
| Public deployment | Not requested | Loopback local Compose only. |

## WeCom handoff prerequisites

Prepare a customer-owned application and provide its organization/application identifiers, server-side application credential, allowed member scope and the intended HTTPS deployment/callback origin through the project's secure local configuration channel. Map enterprise member IDs explicitly to existing OA account/employee identities; never infer or silently create HR administrator permissions. Application credentials belong only on the server, never in `VITE_*`, browser bundles/URLs, logs, screenshots or reports. Official server-to-server token calls use the provider's prescribed parameters over HTTPS and must not be request-logged.

Official OAuth, user identity, token, application messaging and desktop login documents were successfully read through Jina during M6; direct web retrieval failed. Source URLs and the verification boundary are recorded in M6_SOURCE_REUSE.md. Recheck provider console settings when customer resources become available.

Actual login and message delivery require configured enterprise resources and a test member. Until then keep the local adapters clearly marked. No real enterprise messages have been authorized or sent in this development pass. A delivery test needs an explicitly identified recipient and message purpose before sending.

## Cloud storage handoff

For Alibaba Cloud OSS, configure a private bucket, public-access blocking, region/endpoint, restricted server credential and approved object prefix. The current no-overwrite implementation requires bucket versioning to be unset. Preserve authenticated download/preview and employee/contract classification authorization. Local attachments must be read back and verified before each database reference switches; originals remain. Changing the write-backend setting alone does not migrate old files. No real cloud upload or migration has occurred.

Server environment fields and maintenance commands are in [M6 operations](M6_OPERATIONS.md). The public integration status endpoint reports disabled, mock or configured modes only. It does not publish secrets and never claims real provider verification from configuration alone.
