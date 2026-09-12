# CppDefense HTTP API v1

`openapi-v1.yaml` is an OpenAPI 3.1 skeleton. It fixes route names, authentication boundaries, common problem details, idempotency header and core state enums before Backend implementation.

Rules:

- Browser/API session uses the secure `cppdefense_session` cookie plus CSRF protection for state-changing browser requests.
- Runner uses a separate bearer service credential and lease token; it never uses a user cookie.
- `x-roles` documents allowed roles but does not replace Backend object policy.
- Endpoints returning not-found may intentionally hide an existing object outside caller scope.
- State-changing create/apply/complete endpoints require `Idempotency-Key`.
- Errors use `application/problem+json` and stable uppercase `code` values.
- The placeholder server/domain must be replaced before OAuth app registration and public release.

This is a baseline, not a generated server implementation. Endpoint details may be extended compatibly inside `/api/v1`; breaking changes require `/api/v2`.

CI validates the document as OpenAPI 3.1 with the pinned validator in `contracts/requirements-ci.txt`. Cross-field job rules that cannot be expressed cleanly in this initial skeleton are still mandatory in Backend validation: `prepare_defense` jobs carry `PrepareJobInput`, `check_attempt` jobs carry `CheckJobInput`, and completion fields must match `job_kind` and terminal outcome.
