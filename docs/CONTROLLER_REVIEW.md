# M1 controller review

2026-11-06. M1 local-development review complete. All five findings below were resolved and verified in the final QA snapshot. This is not production release approval; see `VALIDATION.md` for results and limitations.

## Findings handed to owners

1. **Attachment storage across launch modes.** Initial native and Compose launch modes used the same database with different attachment roots. QA removed the native modes from the launch script, leaving one Compose stack and its named file volume. Restart behavior still requires runtime verification.
2. **Salary integer precision.** Initial backend accepted the whole int64 range while the browser uses Number. Contract now limits cents to `0..9007199254740991`; backend validation and regression checks requested.
3. **Date-only validation.** Initial birth-date comparison mixed UTC midnight and the current Shanghai instant. Calendar-date comparison requested, including the first eight hours of a local day.
4. **Negative route probe validity.** Requests outside the Nginx proxy prefix can return 404 without reaching Go. QA must probe the API directly before claiming the restricted backend runtime is verified.
5. **Browser test fidelity.** Early selectors did not match the actual contract creation/upload steps or download accessible names. QA must align with the actual page and execute the scenarios.

## Source checks so far

- Fixed upstream module and user/authority models are retained. The OA entry uses a dedicated bootstrap, explicit HTTP timeouts and loopback by default.
- Authentication code constrains JWT signing method/issuer/audience/expiry, and looks up the current enabled account and authority from the database.
- Employee list/detail, contract list and file access apply service-level employee scope; viewer projections omit salary before serialization.
- Private attachment storage uses opaque random keys, bounded writes, file synchronization, restricted permissions and authenticated streaming. Failed or unknown database commits may leave an orphan file; no cleanup/backup claim is made.
- Frontend request code uses header tokens in memory, no cookie authentication or browser persistence, cancellation on logout, and no automatic POST retries.

Runtime and browser evidence is consolidated in `VALIDATION.md` after actual execution, including a final rebuild following the multipart cleanup change. Full dependency vulnerability assessment and production acceptance remain outside this review.
