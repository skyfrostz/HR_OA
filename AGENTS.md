# HR OA project boundaries

Follow supplied parent AGENTS.md and upstream AGENT.MD. CodeGraph first if .codegraph exists; do not index automatically. Upstream licenses and attribution remain.

Controller owns docs/, root project files, server/cmd/oa/, integration runtime glue and coordination. Backend exclusively owns server/oa/ (layered model/service/api/router/bootstrap) and tests there. Frontend exclusively owns web/ OA additions and its dependency files. QA exclusively owns tests/, deploy/oa/, scripts/, root Dockerfile/compose.yaml/.dockerignore after handoff. Never modify another role's files without coordination.

Pure Admin migration authorized 2026-11-07: controller imports the official pinned source into web-pure-admin/, then frontend exclusively owns that directory. Keep upstream MIT license and provenance. Use real Pure Admin layout/router/components for the PC app; retain the existing Vant H5 and current backend APIs. QA owns combined packaging and local runtime. Existing M1/M2 data volumes and credentials must be preserved. No remote push is authorized by a local source import.

M3 remaining local scope also authorized 2026-11-07. Canonical extension: docs/M3_API_CONTRACT.md. Backend may change server/go.mod and server/go.sum only for required M3 dependencies (reuse pinned Excelize where possible). QA owns consistent local backup and isolated restore scripts. Enterprise platform selected by user: WeCom. Object storage selected by user: Alibaba Cloud OSS. Credentials and deployment configuration are not yet supplied. Do not label local or mock integration as real provider acceptance.

Canonical API: docs/OA_API_CONTRACT.md plus docs/M2_API_CONTRACT.md for sequential approvals. M2 visual baseline: docs/M2_DESIGN.md. No employee/salary/contract PII or tokens in URLs/browser persistent storage/logs. Local test data only. No cloud deployment, remote push or real enterprise messages. No go generate/global Go config edits. Actual tests distinguish PASS/FAIL/NOT RUN. Dev runtime explicit route allowlist and MySQL only. Controllers may delegate authorized roles.

Publication authorization, 2026-11-07: user explicitly requested pushing this project to skyfrostz/HR_OA main and writing an enterprise-oriented README. That authorization supersedes the earlier remote-push restriction for this target. Preserve remote history, private local files and all upstream notices. Actual GVA license at the fixed pin is BSL 1.1, not Apache-2.0; Pure Admin remains MIT. Publication details and correction are recorded in docs/PUBLICATION.md.
