# Матрица совместимости CppDefense 2.0

**Baseline:** 2026-09-12. Все production images дополнительно фиксируются полным digest в release manifest.

| Компонент | Зафиксированная версия | Политика |
|---|---|---|
| Язык C++ | C++23 | Флаги `-std=c++23`; расширения GNU не считаются частью контракта core |
| GCC / libstdc++ | 15.3 | Единственный оценивающий toolchain 2.0; другой компилятор допустим только в developer CI |
| CMake / CTest | 4.4.2 | Одна версия внутри runner image; student project обязан поддерживать эту версию |
| Go | 1.27.1 | `go.mod` фиксирует `go 1.27`; обновление patch допускается после CI |
| PostgreSQL | 18.6 | Major 18 на весь цикл 2.0; использовать актуальные security/bugfix minor 18.x |
| Docker Engine | 29.6.2 | VM №2; daemon недоступен Backend и пользовательским контейнерам |
| Ubuntu Server | 24.04.4 LTS, amd64 | Обе VM; unattended security updates с контролируемой перезагрузкой |
| MinIO OSS | `RELEASE.2025-10-15T17-29-55Z` | Временный baseline по digest; см. обязательный gate ниже |
| Worker protocol | 1.0 | Совместимость в пределах major 1 |
| Manifest schemas | 1 | Неизвестная версия отклоняется |
| HTTP API | OpenAPI 3.1, `/api/v1` | Несовместимые изменения только в `/api/v2` |

## Правила обновления

- Release manifest хранит точные версии и SHA-256/digest Backend, runner и sandbox image.
- Go patch, PostgreSQL minor и security patch container runtime проходят integration/e2e и backup-restore test.
- Major-обновление PostgreSQL, Go, CMake, GCC или Docker требует ADR и отдельного compatibility run.
- Оценка одной попытки всегда сохраняет версии GCC, CMake, worker и sandbox image.
- Нельзя использовать плавающие tags `latest` в production.

## Gate по объектному хранилищу

Репозиторий MinIO OSS был архивирован владельцем 2026-04-25. Последняя опубликованная OSS security release — `RELEASE.2025-10-15T17-29-55Z`. Поэтому:

1. MinIO разрешён для локальных alpha/beta и миграционных тестов только с закрытым management API.
2. Перед 2.0-rc выполняется security review этой версии и проверка известных CVE.
3. До 2.0 выбирается поддерживаемая S3-совместимая реализация либо документированно принимается риск.
4. Домен работает через `FileStorage`, поэтому смена реализации не меняет API, БД и object keys.

## Источники baseline

- Go release history: <https://go.dev/doc/devel/release>
- PostgreSQL versioning: <https://www.postgresql.org/support/versioning/>
- CMake releases: <https://cmake.org/cmake/help/latest/release/index.html>
- GCC releases: <https://gcc.gnu.org/releases.html>
- Docker Engine release notes: <https://docs.docker.com/engine/release-notes/29/>
- Ubuntu 24.04 releases: <https://releases.ubuntu.com/24.04/>
- MinIO OSS releases: <https://github.com/minio/minio/releases>
