# Структура CppDefense 2.1.0

Репозиторий разделён по исполняемым компонентам и контрактам. Исходный код и
тесты не смешиваются: C++ public headers находятся в `include/`, реализации —
в `src/`, точки входа — в `apps/`, тесты — в `tests/`.

```text
CppDefense/
├── apps/                         # точки входа C++
│   ├── cli/                      # локальный cpp-defense
│   └── worker/                   # headless cpp-defense-worker
├── include/cpp_defense/          # публичные C++ headers
├── src/                          # C++ реализации и private headers
│   ├── core/
│   ├── application/
│   ├── infrastructure/
│   ├── ui/
│   └── worker/
├── tests/
│   ├── unit/                     # C++ unit-тесты по слоям
│   │   ├── application/
│   │   ├── core/
│   │   ├── infrastructure/
│   │   ├── ui/
│   │   └── worker/
│   └── integration/worker/       # Python/Go проверки JSON worker
├── backend/
│   ├── cmd/cppdefense/           # Go API server
│   ├── cmd/runner/               # только точка входа Runner Agent
│   ├── internal/application/     # сценарии приложения
│   ├── internal/domain/          # доменная модель
│   ├── internal/infrastructure/  # PostgreSQL, S3, GitHub
│   ├── internal/runner/          # агент, worker-клиент, sandbox и ZIP
│   ├── internal/transport/httpapi/ # HTTP API handlers
│   └── webassets/                # HTML, CSS и browser JavaScript
├── contracts/
│   ├── manifests/                # JSON Schema ZIP-манифестов
│   ├── worker/v1/                # JSON-протокол C++ worker
│   └── openapi/                  # HTTP API
├── deploy/                       # Compose, Dockerfiles, systemd, backup
├── docs/                         # архитектура и эксплуатация
└── examples/                     # отдельные учебные CMake-проекты
```

Headers, расположенные внутри `src/`, являются приватными деталями реализации и
не входят в public API библиотеки. Например, платформенный запуск процессов
целиком расположен в `src/infrastructure/process/`. Пользовательские интерфейсы
C++ доступны только через `include/cpp_defense/`.

## Где искать конкретную функцию

- Web endpoint: `backend/internal/transport/httpapi/`.
- SQL и миграции: `backend/internal/infrastructure/postgres/`.
- Frontend: `backend/webassets/`.
- JSON API: `contracts/openapi/openapi-v1.yaml`.
- Worker JSON: `contracts/worker/v1/`.
- Алгоритм выбора функций: `src/application/candidate_picker.cpp`.
- Цикл Runner Agent: `backend/internal/runner/agent.go`.
- Вызов C++ worker: `backend/internal/runner/worker_client.go`.
- Изоляция проверки: `backend/internal/runner/sandbox.go` и
  `deploy/sandbox.Dockerfile`.
- Распаковка задания: `backend/internal/runner/archive.go`.
