# Roadmap

## CppDefense 2.1.0

- Local C++23 CLI with isolated workspaces, deterministic selection, timed
  retries, CMake/CTest validation, bounded logs, and cross-platform tests.
- Headless C++ worker with a versioned JSON protocol and safe path handling.
- Go backend foundation with PostgreSQL migrations, sessions, CSRF, audit,
  health checks, GitHub-only identity schema, and private object storage.
- Local and S3-compatible storage with immutable submission versions,
  deduplication, integrity checks, and reconciliation.
- GitHub OAuth with PKCE, one-time encrypted authorization flows, automatic or
  teacher-approved student linking, server-side sessions, CSRF protection, and
  first-admin bootstrap by numeric GitHub ID.
- Versioned ZIP, worker, and HTTP contracts.
- Bounded ZIP import with teacher/admin review, immutable versions and
  object-level authorization.
- Persistent defense lifecycle, PostgreSQL queue, lease heartbeat and history.
- Separate Runner Agent with 1–6 slots and rootless Podman sandbox.
- Server-rendered role-based web UI and read-only admin role preview.
- Production Compose topology, health checks, backup and guarded restore.

## Before public production

1. Perform a restore drill on a separate VM.
2. Run an end-to-end test with real GitHub OAuth and Cloudflare Tunnel.
3. Run malicious-project and 4–6 concurrent compilation tests on the actual
   runner hardware.
4. Configure disk, memory, health and backup-age alerts.

## Future versions

- Continuous operation and enrollment into a lab "contest" after upload.
- Automatic score assignment immediately after a successful submission.
- A non-graded test-defense mode alongside the official defense.

The local CLI remains supported while the web platform is developed. Contract
or security changes take priority over feature expansion.
