# Идентификаторы и корреляция CppDefense 2.0

## Форматы

| Значение | Формат | Генератор | Секрет |
|---|---|---|---|
| Domain/entity ID | RFC 9562 UUIDv7 lowercase | Backend | Нет |
| `request_id` | UUIDv7 | edge Backend; входной принимается только если валиден | Нет |
| `session_id` защиты | UUIDv7 | Backend | Нет |
| `job_id` | UUIDv7 | Backend в транзакции с attempt | Нет |
| `runner_id` | UUIDv7 | Admin registration | Нет |
| `lease_token` | 32 random bytes, base64url без padding | Backend при lease | Да |
| `idempotency_key` | UUIDv4 или UUIDv7 lowercase | HTTP client | Условно |
| OAuth `state` | минимум 32 random bytes, base64url | Backend | Да, краткоживущий |
| PKCE verifier | 32 random bytes, base64url | Backend | Да, краткоживущий |
| OIDC `nonce` | минимум 32 random bytes, base64url | Backend | Да, краткоживущий |
| Worker `seed` | unsigned 64-bit decimal string | Backend CSPRNG | Нет |
| SHA-256 в API | 64 lowercase hex chars | Компонент, читающий bytes | Нет |

## Правила

- Идентификаторы не содержат роль, группу, login или иной смысл.
- UUID сериализуется в canonical lowercase `8-4-4-4-12`.
- Raw session/lease/OAuth tokens не пишутся в логи и БД; хранится hash или encrypted short-lived value, если flow требует восстановления.
- `request_id` возвращается в `X-Request-ID`, problem details и внутренних логах.
- Backend передаёт request ID в audit и порождённый job; runner добавляет job/session IDs в каждый log event.
- Невалидный внешний `X-Request-ID` заменяется новым, а не отражается обратно.
- Сравнение secret token выполняется constant-time после hash.

## Idempotency

- Заголовок: `Idempotency-Key: <uuid>`.
- Область уникальности: `(authenticated principal, HTTP method, canonical route, key)`.
- Backend сохраняет hash нормализованного request body, status и response reference.
- Повтор с тем же key и тем же body возвращает исходный результат.
- Повтор с тем же key и другим body возвращает `409 IDEMPOTENCY_KEY_REUSED`.
- Срок хранения ключей: минимум 24 часа; ключи создания попыток защиты хранятся не меньше срока жизни защиты.

## Lease

- Raw lease token показывается runner только один раз.
- В job хранится `lease_token_hash`, а в lease history — безопасный fingerprint.
- Heartbeat и complete требуют совпадения job ID, runner ID, актуального token и непросроченной аренды.
- Новый lease немедленно делает старый token недействительным.
