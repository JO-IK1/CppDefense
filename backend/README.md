# CppDefense Go Backend

Каркас Этапа 2 предоставляет HTTP API process, PostgreSQL pool, forward-only
миграции, server-side session primitives, CSRF token primitives, request ID,
единые problem details, JSON-логи, audit chain и health checks.

## Локальный запуск

Требуются Go 1.27.1 и Docker с Compose v2.

```sh
docker compose -f backend/compose.yaml up -d
cp backend/.env.example backend/.env
```

Замените оба `REPLACE_...` независимыми секретами. Секрет можно получить так:

```sh
openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
```

Из каталога `backend` экспортируйте переменные и запустите приложение:

```sh
set -a
. ./.env
set +a
go run ./cmd/cppdefense migrate
go run ./cmd/cppdefense healthcheck
go run ./cmd/cppdefense api
```

При `CPPDEFENSE_AUTO_MIGRATE=true` команда `api` сама подготавливает чистую
базу. Отдельная команда `migrate` нужна для production-развёртывания, где
изменение схемы обычно выполняют до переключения приложения.

Проверка процесса: `GET http://127.0.0.1:8080/health/live`. Проверка базы и
миграций: `GET http://127.0.0.1:8080/health/ready`.

## Тесты

```sh
go test ./...
```

Интеграционные тесты включаются переменной
`CPPDEFENSE_TEST_DATABASE_URL`. Они повторно запускают мигратор, проверяют
уникальность GitHub ID и запрет обхода машины состояний.

## Миграционная политика

Миграции встроены в бинарный файл, выполняются по одной транзакции и защищены
PostgreSQL advisory lock. Уже применённый SQL нельзя редактировать: мигратор
сверяет SHA-256. Исправления выпускаются новой forward migration; production
downgrade не обещается.
