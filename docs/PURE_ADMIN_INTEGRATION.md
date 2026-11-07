# Pure Admin source integration

User explicitly requested importing Pure Admin and building the actual frontend on it on 2026-11-07. This supersedes the earlier visual-reference-only approach.

- Official repository: https://github.com/pure-admin/vue-pure-admin
- Release: v7.0.0
- Exact commit: b8177a202dceb1412d3bf56f3d15483dc1606c9c
- Import: `web-pure-admin/`, 636 upstream files exported from the pinned Git tree; no nested `.git` directory.
- License: upstream MIT text retained at `web-pure-admin/LICENSE`; the GVA pin retains its BSL 1.1 license. The earlier GVA Apache-2.0 description was corrected during publication review on 2026-11-07; see `PUBLICATION.md`.
- Original import metadata: `PURE_ADMIN_UPSTREAM.json`; original per-file hashes: `PURE_ADMIN_UPSTREAM_SHA256SUMS`.
- The original hash manifest intentionally describes the unmodified upstream import. Integration changes will be listed separately and bound to the final acceptance manifest, not falsely described as pristine upstream.

## Runtime integration requirements

The PC entry must use the real upstream layout, menus, tab system, router and styling/components. Existing employee/contract/file workflows move inside that framework. Only actual OA routes and required error/login routes are registered. Unused upstream examples may remain in source for provenance, but must not become runtime menus or executable mock endpoints.

The independent Vant H5 retains its tested mobile workflow. Both PC and H5 use the same OA backend and account/role model. The Pure build uses a separate asset prefix to avoid collisions. The existing `/oa.html` and `/h5.html` entry URLs remain usable.

Replace demo login and cookie/localStorage identity handling with real OA API authentication and memory-only tokens. Preserve row and salary scope, fail closed on expired/disabled identities, and clear sensitive component caches when logging out or changing role. Non-sensitive UI theme preferences may persist. Production builds must not include the mock server, external CDN substitutions, online icon requests, map sample credentials, demo SSO or token refresh endpoints.

## Parallel M3

The user also authorized remaining planned local features. Frontend follows `M3_API_CONTRACT.md` for departments, employee editing, annual leave, Excel preview/confirmation/export and contract reminders. Backend owns these interfaces; QA owns packaging, end-to-end tests and isolated backup restore. WeCom and Alibaba Cloud OSS are selected but not configured.

## Status

Source import and application adaptation complete. The actual Pure PC entry, independent H5, M3 APIs, browser flows, and isolated backup restore passed local acceptance on 2026-11-07. A sidebar highlight defect during rejected navigation was fixed and retested in the final Web image. See `VALIDATION_M3.md` for evidence and limitations, `PURE_ADMIN_CHANGES.md` for the source inventory, and `PURE_M3_SOURCE_SHA256SUMS` for the final accepted file hashes. Earlier M2 validation remains historical.
