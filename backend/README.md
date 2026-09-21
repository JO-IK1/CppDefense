# Go backend

The backend provides HTTP infrastructure, PostgreSQL migrations, server-side
sessions, CSRF primitives, structured errors and logs, an audit chain, health
checks, private storage for project archives, reviewed ZIP import, the defense
queue, role dashboards, and the private Runner API.

For the complete two-VM installation, Cloudflare, rootless Podman, backups and
restore procedure, use [the CppDefense 2.1 deployment guide](../docs/DEPLOYMENT_2.1.md).

## Run locally

Requirements: Go 1.27.1 and Docker Compose v2.

```sh
docker compose -f backend/compose.yaml up -d
cp backend/.env.example backend/.env
```

Replace all `REPLACE_...` values. Generate each base64url key independently with:

```sh
openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
```

Then run from `backend/`:

```sh
set -a
. ./.env
set +a
go run ./cmd/cppdefense migrate
go run ./cmd/cppdefense healthcheck
go run ./cmd/cppdefense api
```

Create a GitHub OAuth App with callback URL
`<CPPDEFENSE_PUBLIC_ORIGIN>/api/v1/auth/github/callback`. The configured numeric
GitHub user ID becomes the first administrator after its first successful
login. The OAuth App needs only the basic `read:user` scope; repository access
is neither requested nor stored.

- Start login: `GET /api/v1/auth/github/start`
- Current account: `GET /api/v1/me`
- Logout: `POST /api/v1/auth/logout` with the CSRF cookie value in
  `X-CSRF-Token`

`CPPDEFENSE_AUTO_MIGRATE=true` lets `api` prepare a new database. Production
deployments should run `migrate` before switching application versions.

- Liveness: `GET http://127.0.0.1:8080/health/live`
- Database and storage readiness: `GET http://127.0.0.1:8080/health/ready`

## Storage

The example environment uses the private MinIO service from Compose. Set
`CPPDEFENSE_STORAGE_MODE=local` to store owner-only files under
`./var/objects` without MinIO.

Check database/object consistency with:

```sh
go run ./cmd/cppdefense reconcile-storage
```

The command reports missing, orphaned, and checksum-mismatched objects; it does
not delete anything. PostgreSQL and MinIO use persistent Compose volumes, so
avoid `docker compose down -v` when their data must be retained.

## Tests

```sh
go test ./...
```

The release gate also runs `go test -race ./...`, `go vet ./...`, all CTest
scenarios, the live worker protocol test, JSON Schema validation, and OpenAPI
validation. Production Compose and VM acceptance commands are documented in
the deployment guide.

Set `CPPDEFENSE_TEST_DATABASE_URL` and `CPPDEFENSE_TEST_S3_ENDPOINT` to enable
the PostgreSQL and S3 integration tests. CI supplies both services.

Migrations are embedded, forward-only, transactional, checksum-verified, and
serialized with a PostgreSQL advisory lock. Never edit an applied migration;
add a new one instead.
