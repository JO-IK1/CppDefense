# Security Policy

**English** | [Русский](SECURITY.ru.md)

CppDefense processes untrusted C++ projects and private educational data.
Never place real student work, credentials, tokens, backups, or unnecessary
personal information in a report or public issue.

## Reporting a vulnerability

Use GitHub Private Vulnerability Reporting. If it is unavailable, contact the
repository owner privately and request a secure channel. Do not publish exploit
details before a fix is available.

Include, when possible:

- affected version or commit;
- component and deployment topology;
- minimal reproduction with synthetic data;
- expected and observed impact;
- a suggested mitigation.

## In scope

- GitHub OAuth, account linking, sessions, CSRF, roles, and object access;
- access to another student's source code, archives, defenses, or results;
- ZIP traversal, links, decompression abuse, or manifest bypass;
- runner authentication, lease reuse, stale/duplicate completion, and replay;
- sandbox escape, unexpected network access, cross-job access, or host damage;
- secret or personal-data leakage through logs, Git, object storage, or backup.

Do not run destructive payloads against the live educational deployment. Use a
local/demo environment and synthetic projects.

## Deployment boundary

The current small VPS deployment uses rootless Podman and hardened disposable
containers, but application services and the runner share one host kernel. This
is not a perfect boundary. A higher-risk deployment must move the runner to a
dedicated machine or VM and expose only the authenticated Runner API and
read-only object access.

## Supported versions and handling

Only the current 2.1 release line and `main` are evaluated for security fixes.
The owner will acknowledge valid private reports, assess severity, prepare a
fix, and coordinate disclosure when practical. No response-time guarantee
applies before the first stable public release.
