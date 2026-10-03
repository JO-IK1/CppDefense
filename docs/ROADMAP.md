# Roadmap

**English** | [Русский](ru/ROADMAP.md)

## Implemented in 2.1

- C++23 CLI, reusable core, and headless worker with 105 CTest scenarios;
- GitHub-only OAuth identity, approval, roles, sessions, CSRF, and admin view;
- reviewed bounded ZIP import and immutable submission versions;
- PostgreSQL queue, Runner Agent, leases, retries, attempts, and history;
- rootless Podman checks with no network and resource limits;
- student repository browser, animated wheel, resizable workspace, drafts, and
  readable configure/build/CTest reports;
- teacher/admin candidate confirmation and compilation history;
- Docker Compose, Caddy HTTPS, backup/restore, protected-main CI/CD;
- English canonical documentation with Russian translations.

## Production hardening priorities

1. Automate off-host encrypted backups and prove restore on another VPS.
2. Add monitoring and alerts for disk, memory, containers, runner heartbeat,
   queue age, certificate renewal, and backup age.
3. Move Runner Agent and untrusted containers to a separate VM/server.
4. Create a least-privilege read-only MinIO identity for runner and rotate any
   credentials used during initial setup.
5. Add browser end-to-end tests for OAuth stubs, import, teacher confirmation,
   defense polling, pane resizing, attempts, and role preview.
6. Run malicious-project tests plus one/two concurrent compilations on the
   actual 2 vCPU / 4 GiB VPS and record safe limits.
7. Add an explicit runner-upgrade stage or version compatibility gate to deploy.
8. Evaluate a maintained S3 implementation before long-term public operation.

## Future product work

- continuous-operation “contest” enrollment after a lab upload;
- immediate score assignment after a successful submission;
- a non-graded test-defense mode alongside an official defense;
- teacher reporting, filters, export, and clearer recovery controls;
- optional independent runner pools for larger groups.

The local CLI remains supported. Security and contract correctness take
priority over new features.
