# Этап 3 — отчёт о готовности

**Дата проверки:** 2026-09-12  
**Основание:** требования 1.4 и план 1.6

## Результат

Backend получил независимый интерфейс `FileStorage`, локальный adapter и
S3-совместимый adapter на MinIO Go SDK v7.2.0. PostgreSQL остаётся источником
истины для метаданных, а object storage хранит только байты архивов и будущих
очищенных логов.

| Область | Статус | Реализация |
|---|---|---|
| `FileStorage` | Готово | put/open/stat/list/delete/check |
| Локальное хранилище | Готово | атомарная публикация, права 0600/0700 |
| S3/MinIO adapter | Готово | private bucket, no-overwrite, presigned GET |
| Разделение архивов | Готово | отдельные original/normalized namespaces |
| SHA-256 | Готово | потоковый расчёт при записи и проверка при сверке |
| Без персональных данных | Готово | object key состоит из namespace + UUID + extension |
| Версии работ | Готово | последовательный version number под DB lock |
| Дубликаты | Готово | совпадение student + lab + SHA-256 возвращает старую версию |
| Неизменяемость | Готово | DB trigger запрещает update/delete submission version |
| Контроль скачивания | Готово | access repository до proxy/presigned download |
| Сверка | Готово | missing, orphaned и checksum mismatch |
| Сохранность | Готово | отдельные persistent volumes PostgreSQL и MinIO |

## Логика хранения

Исходный ZIP преподавателя записывается в `original-archives`. После будущей
безопасной нормализации результат записывается в `normalized-submissions`.
Object key генерируется Backend и не зависит от GitHub login, Telegram ID,
группы или имени файла пользователя.

При создании версии Backend сначала потоково записывает объект и вычисляет
SHA-256. PostgreSQL блокирует последовательность версий конкретной пары
student/lab. Если такой SHA-256 уже существует, новая запись не создаётся, а
лишний только что записанный объект удаляется. При новом содержимом создаётся
следующий неизменяемый `submission_version`.

Уже начатая защита хранит FK на конкретную версию. Добавление новой версии не
переключает существующую защиту.

## Доступ и выдача

Student получает только объект своей `student_record`. Teacher получает
объекты назначенных ему групп, admin — все объекты. Проверка выполняется в
PostgreSQL до открытия потока либо создания временной ссылки. Presigned URL
живёт от одной секунды до 15 минут и не изменяет объект или bucket policy.

## Исправление CI этапа 2

Восстановлена директива `//go:embed migrations/*.sql`. Добавлен отдельный
unit-тест, который требует наличия обеих embedded migrations, поэтому прежняя
ошибка теперь обнаруживается даже без запущенной PostgreSQL.

## Проверка

- unit tests для ключей, local storage, persistence, SHA-256 и size mismatch;
- duplicate version и compensating cleanup;
- reconciliation missing/orphaned/checksum;
- недоступный S3 классифицируется как retryable;
- ограничение срока временной ссылки;
- CI поднимает PostgreSQL 18.6 и private MinIO, затем запускает race-тесты;
- отдельная migration запрещает изменение и удаление submission versions.

Локально Docker отсутствует, поэтому живые PostgreSQL/MinIO integration tests
выполняются в GitHub Actions. Unit tests, build, race и vet выполняются без
внешних сервисов.

## Ограничение MinIO

Для локальной alpha/beta используется последняя официально публиковавшаяся
бинарная container release MinIO `RELEASE.2025-09-07T16-13-09Z`. Домен зависит
только от S3-совместимого `FileStorage`; security gate выбора поддерживаемого
production object storage перед 2.0-rc сохраняется.

