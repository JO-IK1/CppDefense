# Roadmap

## Available

- Local C++23 CLI with isolated workspaces, deterministic selection, timed
  retries, CMake/CTest validation, bounded logs, and cross-platform tests.
- Headless C++ worker with a versioned JSON protocol and safe path handling.
- Go backend foundation with PostgreSQL migrations, sessions, CSRF, audit,
  health checks, GitHub-only identity schema, and private object storage.
- Local and S3-compatible storage with immutable submission versions,
  deduplication, integrity checks, and reconciliation.
- Versioned ZIP, worker, and HTTP contracts.

## Next

1. Complete GitHub OAuth, approval, and object-level authorization endpoints.
2. Implement bounded ZIP import with review and atomic apply.
3. Add defense lifecycle and idempotent runner queue operations.
4. Build and harden Runner Agent on an isolated machine.
5. Add the server-rendered web interface and operational monitoring.
6. Run backup/restore, load, security, and end-to-end release checks.

The local CLI remains supported while the web platform is developed. Contract
or security changes take priority over feature expansion.
