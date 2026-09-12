# Этап 2 — отчёт о готовности

**Дата проверки:** 2026-09-12  
**Основание:** требования 1.4, ER-модель 1.0 и план 1.5

## Результат

В монорепозитории создан Go Backend, готовый стать процессом приложения на VM
№1. Он собирается одной командой и предоставляет режимы `api`, `migrate` и
`healthcheck`. Бизнес-операции авторизации, импорта и защиты будут добавляться
на следующих этапах поверх уже зафиксированных слоёв и схемы.

| Область | Статус | Реализация |
|---|---|---|
| Команды процесса | Готово | `api`, `migrate`, `healthcheck` |
| Слои | Готово | domain, application, infrastructure, transport |
| Конфигурация | Готово | environment, строгая проверка секретов и числовых лимитов |
| PostgreSQL pool | Готово | pgx v5.10.0, лимиты и UTC |
| Forward migrations | Готово | embed, транзакции, advisory lock, SHA-256 ledger |
| ER-модель | Готово | все сущности Этапа 0, FK, check/unique/partial indexes |
| State locking | Готово | DB triggers + version для optimistic locking |
| Sessions/CSRF | Основа готова | серверные hash-only tokens и session repository |
| Request ID | Готово | валидный внешний UUID либо новый UUIDv7 |
| Ошибки | Готово | `application/problem+json` |
| Логи | Готово | JSON structured logging без request body |
| Audit | Готово | обязательный service, append-only table и hash chain |
| Health | Готово | liveness процесса и readiness PostgreSQL/migrations |
| CI | Готово | Go 1.27.1, PostgreSQL 18.6, race, vet, build |

## Схема PostgreSQL

Первая миграция создаёт:

- users, GitHub/Telegram identities, auth flows и web sessions;
- groups, group teachers, student records и labs;
- imports, import items и immutable submission versions;
- defenses, candidates и check attempts;
- runners, jobs и append-only lease history;
- idempotency records и append-only audit events.

Уникальны числовой GitHub ID, Telegram `sub`, session hash, активный ожидаемый
GitHub login в группе и idempotency key в своей области. Пути импорта не могут
быть абсолютными и содержать `.`/`..` segments. SHA-256 хранится ровно 32
байтами.

Переходы users, imports, defenses и runner jobs проверяются PostgreSQL
trigger-функциями. Неописанный переход возвращает SQLSTATE `23000`; terminal
состояния нельзя покинуть. Изменение состояния одновременно увеличивает
`version`, что не позволяет тихо перезаписать более свежую запись.

## Проверка

Локально выполнены:

- `go test -race ./...`;
- `go vet ./...`;
- `go build ./cmd/cppdefense`;
- unit-тесты configuration, UUIDv7, state machine, token/CSRF и HTTP middleware.

Интеграционный тест миграций присутствует, но пропускается без
`CPPDEFENSE_TEST_DATABASE_URL`. На текущем компьютере Docker/PostgreSQL ещё не
установлены. Workflow `Go Backend` запускает его с чистой PostgreSQL 18.6 и
проверяет повторную миграцию, unique GitHub ID и запрещённый state transition.

## Сохранность данных

Локальный Compose использует named volume `cppdefense-postgres`. Остановка
контейнера или Backend не удаляет базу. Команда `docker compose down -v`
удаляет volume и поэтому не должна использоваться для обычной остановки.
Резервные копии и проверка восстановления остаются частью Этапа 11.

## Следующий шаг

Для завершения live-проверки установите Docker Desktop и запустите PostgreSQL
по инструкции `backend/README.md`. После успешного integration run можно
переходить к Этапу 3 — интерфейсу `FileStorage`, MinIO/S3 и версиям лабораторных.

