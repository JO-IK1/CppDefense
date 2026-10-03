# Go Backend

[English](README.md) | **Русский**

Go-сервис отвечает за вход и права, состояние PostgreSQL, объектное хранилище,
проверяемый импорт ZIP, постоянную очередь защит, web-страницы и закрытый Runner
API. Runner Agent собирается из `cmd/runner`; C++ worker остаётся отдельным
исполняемым файлом.

## Локальная разработка

Нужны Go 1.27.1, Node.js 24 для browser regression tests, Docker и Docker
Compose v2.

Из корня репозитория:

```sh
docker compose -f backend/compose.yaml up -d
cp backend/.env.example backend/.env
```

Замените все `REPLACE_...`. Каждый секрет создавайте отдельно:

```sh
openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
```

Создайте GitHub OAuth App с callback
`http://localhost:8080/api/v1/auth/github/callback`. Нужны только основные
данные профиля; доступ к репозиториям не запрашивается. Числовой GitHub ID
первого администратора укажите в `CPPDEFENSE_BOOTSTRAP_ADMIN_GITHUB_ID`.

Запуск Backend:

```sh
cd backend
set -a
. ./.env
set +a
go run ./cmd/cppdefense migrate
go run ./cmd/cppdefense healthcheck
go run ./cmd/cppdefense api
```

Основные endpoint:

- `GET /health/live` — процесс работает;
- `GET /health/ready` — готовы PostgreSQL и хранилище;
- `GET /api/v1/auth/github/start` — вход OAuth;
- `GET /api/v1/me` — текущий аккаунт;
- `POST /api/v1/auth/logout` — выход с CSRF-заголовком.

Полный контракт: [`contracts/openapi/openapi-v1.yaml`](../contracts/openapi/openapi-v1.yaml).

## Хранилище и миграции

Пример использует приватный MinIO. Для owner-only файлов в
`CPPDEFENSE_STORAGE_LOCAL_ROOT` задайте `CPPDEFENSE_STORAGE_MODE=local`.
Безопасная проверка согласованности без удаления:

```sh
go run ./cmd/cppdefense reconcile-storage
```

Миграции встроены в binary, выполняются только вперёд и транзакционно,
проверяются по checksum и защищены advisory lock PostgreSQL. Не редактируйте
уже применённую миграцию — добавляйте следующий номер.

PostgreSQL и MinIO используют постоянные Compose volumes. Не запускайте
`docker compose down -v`, если данные должны сохраниться.

## Разработка Runner

Runner Agent нужны URL Backend, общий service token, постоянный UUID runner,
путь к C++ worker, rootless container runtime, sandbox image, число slots и
read-only ключи хранилища. См. [`deploy/runner.env.example`](../deploy/runner.env.example)
и [инструкцию развёртывания](../docs/ru/DEPLOYMENT_2.1.md).

Подготовка состоит из двух jobs: поиск кандидатов, затем создание выбранного
замаскированного задания после подтверждения преподавателя. Check job вставляет
ответ и возвращает раздельные configure/build/CTest результаты. Временная
ошибка может повторяться; активную защиту завершает только успешная попытка.

## Проверка

```sh
go test -race ./...
go vet ./...
go build ./cmd/cppdefense ./cmd/runner
node --check webassets/static/app.js
node --test webassets/static/app_test.mjs
```

Для integration tests задайте `CPPDEFENSE_TEST_DATABASE_URL` и переменные
`CPPDEFENSE_TEST_S3_*`; CI поднимает оба сервиса. Полный release gate также
выполняет 105 CTest-сценариев, live worker protocol, JSON Schema, OpenAPI,
Compose validation и сборку образов.

Production-схема одного VPS описана в
[руководстве CppDefense 2.1](../docs/ru/DEPLOYMENT_2.1.md).
