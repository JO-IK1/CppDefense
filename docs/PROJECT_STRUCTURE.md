# CppDefense 2.1 project structure

**English** | [Русский](ru/PROJECT_STRUCTURE.md)

The repository is organized by executable boundary. Public C++ headers are in
`include/`, implementations in `src/`, entry points in `apps/`, and tests in
`tests/`. The Go application is an `internal/` modular monolith.

```text
CppDefense/
├── apps/
│   ├── cli/                       # local cpp-defense entry point
│   └── worker/                    # headless cpp-defense-worker
├── include/cpp_defense/           # public C++ headers
├── src/
│   ├── core/                      # hashing and low-level domain code
│   ├── application/               # defense use cases
│   ├── infrastructure/            # files, parser, workspaces, processes
│   ├── ui/                        # CLI interaction
│   └── worker/                    # JSON adapter and workspace policy
├── tests/
│   ├── unit/                      # C++ tests grouped by layer
│   └── integration/worker/        # Python and Go live worker tests
├── backend/
│   ├── cmd/cppdefense/            # API/migrate/health/reconcile command
│   ├── cmd/runner/                # Runner Agent entry point
│   ├── internal/application/      # auth, import, storage, audit use cases
│   ├── internal/domain/           # identifiers and state validation
│   ├── internal/infrastructure/   # PostgreSQL, S3/local, GitHub adapters
│   ├── internal/runner/           # queue client, worker, sandbox, archive
│   ├── internal/transport/httpapi/# routes, middleware, pages
│   └── webassets/                 # embedded HTML, CSS, and browser JS
├── contracts/
│   ├── manifests/                 # import JSON Schemas
│   ├── worker/v1/                 # C++ worker JSON protocol
│   └── openapi/                   # public and Runner HTTP API
├── deploy/                        # Compose, Caddy, systemd, backup/restore
├── docs/                          # canonical English documentation
│   └── ru/                        # Russian translations
├── examples/                      # standalone educational CMake projects
└── .github/workflows/             # C++, Go, contracts, and production deploy
```

Headers under `src/` are private implementation details. For example, process
launching lives entirely under `src/infrastructure/process/`; downstream code
must use interfaces from `include/cpp_defense/`.

## Where to change a feature

| Concern | Primary location |
|---|---|
| HTTP route or problem response | `backend/internal/transport/httpapi/` |
| SQL query or migration | `backend/internal/infrastructure/postgres/` |
| Browser layout or behavior | `backend/webassets/` |
| ZIP validation/import | `backend/internal/application/importer/` |
| Runner loop and sandbox | `backend/internal/runner/` |
| API shape | `contracts/openapi/openapi-v1.yaml` |
| Worker request/response | `contracts/worker/v1/` |
| Function discovery/selection | `src/infrastructure/` and `src/application/` |
| Production service topology | `deploy/*.compose.yaml`, `deploy/Caddyfile` |
| Operations | `deploy/*.sh`, `docs/DEPLOYMENT_2.1.md` |

A contract change should be made and validated before the implementations on
both sides are changed. Applied migrations are immutable; add a new migration.
