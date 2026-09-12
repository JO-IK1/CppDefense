# Модель угроз CppDefense 2.0

**Версия:** 1.0  
**Дата:** 2026-09-12  
**Метод:** trust boundaries + STRIDE  
**Пересмотр:** перед 2.0-rc и после изменения auth, ZIP, runner, sandbox или storage

## Защищаемые активы

1. Исходный код, ZIP и ответы студентов.
2. Результаты защит, история компиляций и audit.
3. GitHub/Telegram identities, web sessions и OAuth/OIDC secrets.
4. PostgreSQL, S3 objects и резервные копии.
5. VM №1, VM №2, runner credentials и container runtime.
6. Доступность сайта и справедливость очереди/deadline.
7. Публичный исходный код CppDefense без production secrets.

## Участники угроз

- обычный студент, пытающийся получить чужой код/результат или обойти защиту;
- студент, загрузивший специально вредоносный ZIP/CMake/C++ проект;
- пользователь с украденной OAuth identity/session;
- преподаватель, выходящий за пределы своей группы;
- скомпрометированный runner или dependency;
- внешний неаутентифицированный атакующий;
- случайная ошибка администратора, сбой питания/диска или утечка backup.

## Границы доверия

```text
Browser ──[Internet/Cloudflare]── Backend ── PostgreSQL
                                   │
                                   └── S3/MinIO
                                   │ authenticated runner API
                                   ▼
                              Runner VM ── container sandbox
GitHub/Telegram ──[OAuth/OIDC]─────┘/Backend
```

- Всё из browser, ZIP и student project недоверенно.
- Runner доверен только как service principal с узкими правами.
- Container sandbox считается враждебным даже при корректном проекте.
- VM №2 — дополнительная граница на случай container escape.

## Реестр угроз и меры

| ID | Область/угроза | Риск | Обязательные меры | Проверка |
|---|---|---|---|---|
| AUTH-01 | OAuth login CSRF/code injection | Чужая identity привязывается к session | random `state`, PKCE S256, exact callback, one-time flow, short expiry | negative integration tests |
| AUTH-02 | Подмена/повтор OIDC token | Account takeover | nonce, issuer/audience/signature/exp, one-time flow | invalid token matrix |
| AUTH-03 | Ошибочное auto-link по изменяемому login | Доступ к чужим лабам | auto-link только свободного exact login; далее numeric GitHub ID; conflict → pending | duplicate/rename tests |
| AUTH-04 | Account merge takeover | Объединение разных пользователей | запрет auto-merge занятой identity, recent re-auth, admin audit | linking tests |
| AUTH-05 | Session theft/fixation | Захват роли | 256-bit token, hash at rest, rotate after login, Secure/HttpOnly/SameSite, revoke on block | cookie/session tests |
| RBAC-01 | IDOR по UUID/object key | Чужой код/результаты | object-level policy every endpoint, generic 404/403 policy, scoped query | student A/B and group A/B tests |
| RBAC-02 | Teacher выходит за группу | Массовая утечка | group scope in application service and SQL predicates, audit archive reads | negative API tests |
| RBAC-03 | Подмена view-as-role | Admin/student privilege escalation | real role from server session; view-as admin-only, read-only except demo | tampered request tests |
| ZIP-01 | ZIP Slip/absolute/Windows path | Запись вне workspace | canonical relative path, NFC/case checks, reject `..`, drive, UNC, backslash | malicious corpus |
| ZIP-02 | ZIP bomb/resource exhaustion | DoS/disk loss | streaming limits for bytes/files/ratio/depth, quotas, timeout, temp volume | bomb tests |
| ZIP-03 | Symlink/device/encrypted entry | Escape/hidden payload | reject links, devices, encrypted and nested ZIP | crafted archives |
| ZIP-04 | TOCTOU after review | Другой архив применяется | immutable object + original/reviewed SHA-256 equality | replace/race test |
| ZIP-05 | Partial import | Несогласованные версии | atomic DB transaction, object staging/finalization, idempotency | injected failures |
| QUEUE-01 | Двойное выполнение job | Двойной итог/балл | atomic lease, random token hash, expiry, idempotent complete | concurrent runners |
| QUEUE-02 | Stale runner records result | Перезапись свежего результата | verify runner/token/lease/version at complete | expired lease test |
| QUEUE-03 | Queue starvation/DoS | Защита не укладывается во время | per-user pending limit, 4–6 slots, rate limits, metrics/alerts | load/soak test |
| RUN-01 | Arbitrary CMake command | Host compromise | execute only disposable container on VM №2 | hostile CMake tests |
| RUN-02 | Container escape | VM №2 compromise | unprivileged UID, no caps, no-new-privileges, seccomp/AppArmor, patched runtime, separate VM | hardening audit |
| RUN-03 | Network exfiltration | Утечка кода/secrets | `--network none`, no DNS, no secrets/env, egress test | network probe |
| RUN-04 | Fork bomb/OOM/disk/log bomb | DoS | CPU/RAM/PID/disk/wall/log limits and forced cleanup | malicious corpus |
| RUN-05 | Cross-job read | Утечка между студентами | new workspace/container, random path, no shared mount, cleanup verification | parallel isolation test |
| WORKER-01 | Path/offset manipulation | Запись не в тот файл | schema, workspace canonicalization, source SHA-256, offset invariants | contract/fuzz tests |
| STORE-01 | Public bucket/presigned URL leak | Утечка исходников | private buckets, short TTL, scoped object, auth before issue, redact URL | access tests |
| STORE-02 | Object/DB inconsistency | Потеря работы | content hash, reconciliation, staged upload, backups | reconciliation test |
| DB-01 | SQL injection/overbroad query | Data breach | parameterized SQL, least-privilege DB role, review generated queries | SAST/integration |
| LOG-01 | Secrets/code in logs | Data breach | structured allowlist, truncate/scrub paths/tokens/source, protected access | log snapshot tests |
| BACKUP-01 | Backup theft | Массовая утечка | encryption, separate credentials/location, access log, restore-only access | restore drill |
| BACKUP-02 | Ноутбук/диск потерян | Потеря системы | external verified copies, retention, monthly clean restore | restore drill |
| SUPPLY-01 | Malicious dependency/image | Host compromise | lock/checksum/digest, SBOM, vulnerability and license scanning | CI policy |
| SUPPLY-02 | Public repo secret | Credential compromise | no real `.env`/data, secret scanning/push protection, rotate leaked secrets | history scan |
| EDGE-01 | Direct VM exposure | Обход edge controls | firewall closes public ports, outbound Tunnel only | external port scan |
| AVAIL-01 | Cloudflare/GitHub outage | Login unavailable | existing sessions continue where safe, clear status, monitored dependency | failure drill |

## Sandbox minimum profile

- New container per attempt.
- Network mode none.
- Non-root fixed UID/GID; root filesystem read-only.
- Drop all capabilities; `no-new-privileges`; default-deny seccomp/AppArmor profile.
- No Docker socket, host PID/IPC/network, devices or arbitrary mounts.
- Writable tmpfs/workspace with size cap.
- CPU, memory, swap, pids, wall-time and log caps.
- Kill process group/container on timeout; verify no residual processes/mounts/workspace.
- Sandbox image contains no service credentials and is pinned by digest.

Container isolation is not treated as a perfect security boundary. VM №2 contains no PostgreSQL/MinIO data and is replaceable after compromise.

## OAuth/OIDC minimum profile

- Exact HTTPS callback; no wildcard.
- `state` and PKCE required for GitHub; `state`, nonce and signature checks for Telegram.
- Minimal scopes; GitHub repository access is forbidden for 2.0.
- Provider errors and profile data are untrusted input and safely escaped.
- OAuth client secrets exist only on VM №1 secret store/environment, never Git, browser or runner.
- Raw access tokens are not persisted when profile lookup completes.

## Data minimization

- Store only identity fields required for login/matching.
- Object keys do not contain FIO, Telegram sub, username or GitHub login.
- User-visible compiler logs redact host paths and are capped.
- CSV excludes source, full answer, identities not needed for grade export.
- Audit metadata uses identifiers, not full sensitive payloads.

## Residual risks and release gates

1. Container/runtime zero-day remains possible; separate VM and patch cadence reduce impact.
2. A public source repository helps attackers study implementation; controls must not rely on obscurity.
3. Home laptop remains a physical/availability single point of failure in 2.0; external backup is mandatory.
4. MinIO OSS upstream is archived; a maintained S3 implementation or explicit accepted risk is required before 2.0.
5. GitHub/Telegram availability is external; target-network smoke tests are required.

## Security Definition of Done

- Every threat above has an automated test, manual checklist item or explicit accepted-risk record.
- No critical/high unresolved vulnerability in Internet-facing or sandbox components.
- Malicious project suite cannot access network/host/other workspace or exceed configured resources.
- Restore drill proves DB/object consistency.
- RBAC negative tests cover every object type and role pair.
