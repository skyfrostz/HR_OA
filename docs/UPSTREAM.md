# Upstream and local modifications

- Repository: https://github.com/flipped-aurora/gin-vue-admin
- Tag: v2.9.2-stable
- Exact commit: a6882210a80bb27e3aa5dff0b4c21aa4afe8988a
- Local branch: hr-oa/m1
- LICENSE: upstream BSL 1.1 retained unmodified; original attribution preserved. The earlier Apache-2.0 description was incorrect and was corrected during publication review on 2026-11-07. Apache-2.0 is the stated future Change License, not the current license of this pin. See `PUBLICATION.md`.
- Initial default-branch candidate cee04b08d1262036ca33e3026e1e13d29471c7d5 requires Go1.26.10. Stable tag uses go1.24.0 + toolchain go1.24.2 and was selected for compatibility with installed Go1.26.4. No global Go settings changed.
- OA additions: server/oa layered business module, server/cmd/oa controlled bootstrap; web OA entry keeps upstream Vue/Element Plus dependencies. Runtime uses GVA response/identity models and same module, Gin/GORM/Casbin/MySQL. No replacement framework or additional account store.
- Original server/main.go is upstream development/admin runtime and is NOT the OA launch path. Use the explicit cmd/oa entry only.
- Modified upstream files carry local modification notice or are recorded in the final change manifest. Existing upstream source remains available for provenance; presence is not registration in OA runtime.

Dependency security is not proven by a tag. Read SECURITY_BASELINE.md for assessed exposure and limits. A production dependency audit and deployment hardening remain release gates.

## Local change manifest (M1, 2026-11-06)

- Modified upstream files: `.gitignore` (exclude local data/secrets/results), `aiDoc/frontend-backend/boundary.md` (OA contract/identity boundary), `web/.gitignore` (retain the OA-tested npm lockfile), `web/package.json` (OA build/dev/test commands and test dependencies).
- Added backend: `server/cmd/oa/`, `server/oa/` (bootstrap, model, service, API, router and tests).
- Added frontend: `web/oa.html`, `web/vite.oa.config.js`, `web/package-lock.json`, `web/src/oa/`.
- Added delivery: `Dockerfile`, `.dockerignore`, `compose.yaml`, `deploy/oa/`, `scripts/`, `tests/`, `AGENTS.md`, `docs/`.
- LICENSE, server/go.mod, server/go.sum and the original GVA web entry/config remain unchanged. QA result files, local secrets and storage are excluded from source control.
- No remote push or public deployment was performed. Original source and attribution are retained; this is a local OA extension of the pinned upstream code.

## M2 extension (accepted local development, 2026-11-06)

- Additional workflow models/services/APIs inside `server/oa/`; controlled entry adds graceful outbox lifecycle and local mock-failure configuration.
- Added `web/h5.html` and `web/src/h5/`, built alongside the existing PC entry. Vant supplies mobile components; existing Vue/Element Plus PC stack remains.
- PC/H5 visual work independently references Pure Admin, Vben Admin and Soybean Admin. Reference links and concrete design choices are recorded in `M2_DESIGN.md`; reference does not imply copying their source or assets.
- M2 QA scripts add real MySQL race/rollback/outbox checks and browser scenarios. M1 acceptance artifacts remain historical evidence; M2 results are recorded separately.
