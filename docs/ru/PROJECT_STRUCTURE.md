# Структура CppDefense 2.1

[English](../PROJECT_STRUCTURE.md) | **Русский**

Репозиторий разделён по исполняемым границам. Public C++ headers находятся в
`include/`, реализации — в `src/`, entry points — в `apps/`, тесты — в
`tests/`. Go-приложение является модульным монолитом внутри `internal/`.

```text
CppDefense/
├── apps/
│   ├── cli/                       # точка входа local CLI
│   └── worker/                    # headless cpp-defense-worker
├── include/cpp_defense/           # public C++ headers
├── src/
│   ├── core/                      # hashing и низкоуровневая логика
│   ├── application/               # сценарии защиты
│   ├── infrastructure/            # файлы, parser, workspace, процессы
│   ├── ui/                        # консольный интерфейс
│   └── worker/                    # JSON adapter и path policy
├── tests/
│   ├── unit/                      # C++ tests по слоям
│   └── integration/worker/        # live tests на Python и Go
├── backend/
│   ├── cmd/cppdefense/            # API/migrate/health/reconcile
│   ├── cmd/runner/                # Runner Agent
│   ├── internal/application/      # auth, import, storage, audit
│   ├── internal/domain/           # identifiers и states
│   ├── internal/infrastructure/   # PostgreSQL, S3/local, GitHub
│   ├── internal/runner/           # queue, worker, sandbox, archive
│   ├── internal/transport/httpapi/# routes, middleware, pages
│   └── webassets/                 # встроенные HTML, CSS и browser JS
├── contracts/
│   ├── manifests/                 # JSON Schema импорта
│   ├── worker/v1/                 # JSON protocol C++ worker
│   └── openapi/                   # HTTP API browser и Runner
├── deploy/                        # Compose, Caddy, systemd, backup/restore
├── docs/                          # основная английская документация
│   └── ru/                        # русские переводы
├── examples/                      # отдельные учебные CMake-проекты
└── .github/workflows/             # C++, Go, contracts и production deploy
```

Headers внутри `src/` — приватные детали. Например, запуск процессов полностью
находится в `src/infrastructure/process/`; внешний код использует интерфейсы из
`include/cpp_defense/`.

## Где менять конкретную функцию

| Задача | Основное место |
|---|---|
| HTTP route или problem response | `backend/internal/transport/httpapi/` |
| SQL или migration | `backend/internal/infrastructure/postgres/` |
| Layout и поведение browser | `backend/webassets/` |
| ZIP validation/import | `backend/internal/application/importer/` |
| Runner loop и sandbox | `backend/internal/runner/` |
| API shape | `contracts/openapi/openapi-v1.yaml` |
| Worker request/response | `contracts/worker/v1/` |
| Поиск и выбор функций | `src/infrastructure/`, `src/application/` |
| Production topology | `deploy/*.compose.yaml`, `deploy/Caddyfile` |
| Эксплуатация | `deploy/*.sh`, `docs/DEPLOYMENT_2.1.md` |

Сначала меняйте и проверяйте contract, затем обе реализации. Уже применённая
migration неизменяема — добавляйте новую.
