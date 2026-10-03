# CppDefense HTTP API v1

[English](README.md) | **Русский**

`openapi-v1.yaml` — основной OpenAPI 3.1 contract маршрутов, границ
аутентификации, problem details, idempotency и состояний.

Правила:

- браузер использует secure cookie `cppdefense_session` и CSRF для изменений;
- Runner использует отдельный bearer credential и lease token;
- `x-roles` описывает роли, но не заменяет object policy Backend;
- not-found может скрывать существование чужого объекта;
- create/apply/complete операции используют `Idempotency-Key`;
- ошибки имеют `application/problem+json` и стабильный uppercase `code`;
- server URL указывает на текущий public origin; локальная среда меняет только
  origin, но не контракт маршрутов.

Это contract, а не сгенерированный сервер. Совместимые расширения допустимы в
`/api/v1`, ломающее изменение требует `/api/v2`. Правила зависимости полей job
от `job_kind`, которые неудобно выразить схемой, всё равно обязательны в
Backend. CI проверяет документ закреплённым OpenAPI-валидатором.
