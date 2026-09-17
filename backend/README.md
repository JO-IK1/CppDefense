# Go backend

The backend provides HTTP infrastructure, PostgreSQL migrations, server-side
sessions, CSRF primitives, structured errors and logs, an audit chain, health
checks, and private storage for project archives.

## Run locally

Requirements: Go 1.27.1 and Docker Compose v2.

```sh
docker compose -f backend/compose.yaml up -d
cp backend/.env.example backend/.env
```

Replace both `REPLACE_...` values with independent secrets. Generate one with:

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

Set `CPPDEFENSE_TEST_DATABASE_URL` and `CPPDEFENSE_TEST_S3_ENDPOINT` to enable
the PostgreSQL and S3 integration tests. CI supplies both services.

Migrations are embedded, forward-only, transactional, checksum-verified, and
serialized with a PostgreSQL advisory lock. Never edit an applied migration;
add a new one instead.
