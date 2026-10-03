# CppDefense

**English** | [Русский](README.ru.md)

> Source available for review. Proprietary software — not open source.

Copyright (c) 2026 Zakharev Georgii. All rights reserved.

CppDefense turns a CMake-based C++ project into a timed code-restoration
exercise. It discovers suitable functions, selects one reproducibly, hides its
body, accepts a body-only answer, then runs CMake, the build, and CTest in an
isolated workspace. The repository contains both the original local CLI and a
web platform for students, teachers, and administrators.

## Highlights

- C++23 core, interactive `cpp-defense` CLI, and headless JSON worker;
- deterministic candidate selection and UUID-scoped workspaces;
- Go HTTP backend with GitHub OAuth, server-side sessions, CSRF, RBAC, and
  object-scoped authorization;
- reviewed group/lab ZIP import with traversal and resource-exhaustion guards;
- PostgreSQL state, immutable submission versions, and S3-compatible storage;
- persistent defense queue with leases, heartbeats, retries, and audit events;
- rootless Podman sandbox with no network and CPU, memory, PID, filesystem,
  timeout, and log limits;
- role-based web UI, animated function wheel, masked repository browser,
  resizable source/answer panes, and readable compilation reports;
- Linux, macOS, and Windows C++ CI plus Go, contract, and deployment checks.

## How a web defense works

1. A teacher or administrator imports a ZIP and approves its preview.
2. The student signs in through GitHub and starts a defense for an immutable
   submission version.
3. The runner finds eligible production functions. Test files and build output
   are excluded from candidate selection.
4. A teacher confirms automatic or manual selection and the time limit.
5. The runner masks the selected body; only then does the timer start.
6. Each answer is patched into a disposable copy and checked with
   CMake → build → CTest. Success finishes the defense; a failed attempt may be
   retried before the deadline.

The local CLI performs the same basic exercise flow for trusted local projects.
The original source tree is never modified.

## Repository map

| Path | Purpose |
|---|---|
| `apps/`, `include/`, `src/` | C++ CLI, worker, core, and infrastructure |
| `backend/` | Go API, runner agent, PostgreSQL/S3 adapters, and browser UI |
| `contracts/` | ZIP JSON Schemas, OpenAPI 3.1, and worker protocol v1 |
| `deploy/` | Docker Compose, Caddy, rootless Podman, backup, and restore |
| `tests/` | C++ and worker integration tests |
| `docs/` | Architecture, deployment, operations, audit, and roadmap |

See [Project structure](docs/PROJECT_STRUCTURE.md) for the detailed map.

## Build and verify

Local CLI requirements are a C++23 compiler and CMake 3.24+:

```sh
cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build --parallel 2
ctest --test-dir build --output-on-failure
```

The current C++ suite contains **105 CTest scenarios**. The Go backend requires
Go 1.27.1; its normal gate is:

```sh
go -C backend test -race ./...
go -C backend vet ./...
node --check backend/webassets/static/app.js
node --test backend/webassets/static/app_test.mjs
```

See [Local CLI usage](docs/USAGE.md), [Go backend](backend/README.md), and the
[VPS deployment guide](docs/DEPLOYMENT_2.1.md) for complete instructions.

## Current deployment

The documented small production target is one Ubuntu 22.04 VPS with 2 vCPU,
4 GiB RAM, and roughly 120 GiB of storage, intended for about 10–20 registered
users and one or two concurrent checks:

- Caddy terminates HTTPS for `cppdefense.jo-a1.ru`;
- Docker Compose runs the backend, PostgreSQL, and MinIO;
- a systemd service runs the Go Runner Agent as an unprivileged user;
- rootless Podman starts a disposable container for every check.

This single-host layout is economical but weaker than a separate runner host:
untrusted code still shares the VPS kernel with the application services. Use a
dedicated runner machine before increasing the threat model or concurrency.

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — components, data flow, state, and
  security boundaries.
- [Contracts](contracts/README.md) — versioned integration boundaries.
- [Roadmap](docs/ROADMAP.md), [security policy](SECURITY.md), and
  [release guide](docs/RELEASING.md).
- [UI preview](docs/UI_PREVIEW.html) — a standalone browser mock.

Every Markdown document has an English canonical version and a linked Russian
translation. Technical schemas and executable examples remain language-neutral.

## License

The public repository is available for portfolio and employment review under
the [CppDefense Proprietary Source-Available License](LICENSE). Viewing and
GitHub forking for evaluation are permitted; redistribution, derivative use,
and self-hosting require written permission unless an earlier license already
granted those rights. See the [license transition record](docs/LICENSING.md).
