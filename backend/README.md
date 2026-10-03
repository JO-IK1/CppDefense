# Go backend

**English** | [Русский](README.ru.md)

The Go service owns authentication, authorization, PostgreSQL state, object
storage, reviewed ZIP import, the persistent defense queue, browser pages, and
the private Runner API. The Runner Agent is built from `cmd/runner`; the C++
worker remains a separate executable.

## Local development

Requirements: Go 1.27.1, Node.js 24 for browser regression tests, Docker, and
Docker Compose v2.

From the repository root:

```sh
docker compose -f backend/compose.yaml up -d
cp backend/.env.example backend/.env
```

Replace every `REPLACE_...` value. Generate each secret independently:

```sh
openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
```

Create a GitHub OAuth App with callback
`http://localhost:8080/api/v1/auth/github/callback`. Only basic profile data is
used; CppDefense does not request repository access. Put the numeric GitHub ID
of the first administrator into `CPPDEFENSE_BOOTSTRAP_ADMIN_GITHUB_ID`.

Run the backend:

```sh
cd backend
set -a
. ./.env
set +a
go run ./cmd/cppdefense migrate
go run ./cmd/cppdefense healthcheck
go run ./cmd/cppdefense api
```

Useful endpoints:

- `GET /health/live` — process liveness;
- `GET /health/ready` — PostgreSQL and object-storage readiness;
- `GET /api/v1/auth/github/start` — OAuth login;
- `GET /api/v1/me` — current account;
- `POST /api/v1/auth/logout` — logout with the CSRF header.

The complete API boundary is documented in
[`contracts/openapi/openapi-v1.yaml`](../contracts/openapi/openapi-v1.yaml).

## Storage and migrations

The example environment uses private MinIO. Set
`CPPDEFENSE_STORAGE_MODE=local` for owner-only files under
`CPPDEFENSE_STORAGE_LOCAL_ROOT`. Reconcile database and object storage without
deleting anything:

```sh
go run ./cmd/cppdefense reconcile-storage
```

Migrations are embedded, forward-only, transactional, checksum-verified, and
serialized by a PostgreSQL advisory lock. Never edit a migration already
applied to any environment; create the next numbered file.

PostgreSQL and MinIO use persistent Compose volumes. Do not run
`docker compose down -v` when their data must survive.

## Runner development

The Runner Agent needs the backend URL, shared service token, stable runner
UUID, C++ worker path, rootless container runtime, sandbox image, slot count,
and read-only object-storage credentials. See
[`deploy/runner.env.example`](../deploy/runner.env.example) and the
[deployment guide](../docs/DEPLOYMENT_2.1.md).

Preparation has two jobs: discover candidates, then materialize the selected
masked challenge after teacher confirmation. Check jobs materialize the answer
and return separate configure/build/CTest results. A transient failure is
retryable; only a passing attempt completes an active defense.

## Verification

```sh
go test -race ./...
go vet ./...
go build ./cmd/cppdefense ./cmd/runner
node --check webassets/static/app.js
node --test webassets/static/app_test.mjs
```

Set `CPPDEFENSE_TEST_DATABASE_URL` and the `CPPDEFENSE_TEST_S3_*` variables to
enable PostgreSQL and S3 integration tests. CI supplies both services. The full
release gate additionally runs 105 CTest scenarios, the live worker protocol,
JSON Schema checks, OpenAPI validation, Compose validation, and image builds.

For the supported single-VPS production topology, use the
[CppDefense 2.1 deployment guide](../docs/DEPLOYMENT_2.1.md).
