# CppDefense Go Backend

Backend предоставляет HTTP API process, PostgreSQL pool, forward-only
миграции, server-side session primitives, CSRF token primitives, request ID,
единые problem details, JSON-логи, audit chain, health checks и приватное
файловое хранилище для архивов лабораторных.

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

Проверка процесса: `GET http://127.0.0.1:8080/health/live`. Проверка базы,
миграций и object storage: `GET http://127.0.0.1:8080/health/ready`.

## Хранилище лабораторных

В `.env.example` включён S3-режим, который использует локальный MinIO из
Compose. Bucket создаётся без публичной policy. В object key находится только
namespace, UUID и расширение — логины, ФИО и другие персональные данные не
используются.

- `original-archives/` хранит исходные ZIP преподавателя;
- `normalized-submissions/` хранит неизменяемые нормализованные проекты;
- `safe-logs/` зарезервирован для очищенных логов runner.

Для быстрых unit-тестов или разработки без MinIO установите
`CPPDEFENSE_STORAGE_MODE=local`. Файлы попадут в `./var/objects` с правами
только для владельца процесса.

Сверка БД и хранилища запускается командой:

```sh
go run ./cmd/cppdefense reconcile-storage
```

Она сообщает о ссылках на отсутствующие объекты, лишних объектах и ошибках
SHA-256. Команда ничего автоматически не удаляет.

## Тесты

```sh
go test ./...
```

Интеграционные тесты включаются переменной
`CPPDEFENSE_TEST_DATABASE_URL` и `CPPDEFENSE_TEST_S3_ENDPOINT`. Они повторно
запускают мигратор, проверяют ограничения БД и выполняют round-trip через
приватный S3 bucket.

Compose использует persistent volumes `cppdefense-postgres` и
`cppdefense-minio`. Обычные stop/start и `docker compose down` сохраняют
данные. Не используйте `docker compose down -v`, если данные нужны.

## Миграционная политика

Миграции встроены в бинарный файл, выполняются по одной транзакции и защищены
PostgreSQL advisory lock. Уже применённый SQL нельзя редактировать: мигратор
сверяет SHA-256. Исправления выпускаются новой forward migration; production
downgrade не обещается.
