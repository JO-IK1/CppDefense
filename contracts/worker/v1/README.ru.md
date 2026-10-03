# Протокол cpp-defense-worker 1.0

[English](README.md) | **Русский**

## Транспорт

- один UTF-8 JSON request в stdin и один JSON response в stdout;
- stdout содержит только JSON, diagnostics идут в ограниченный stderr;
- один процесс обслуживает одну команду и завершается;
- exit `0` означает schema-valid response, non-zero — crash/protocol failure;
- `status: error` — обработанная ошибка, повтор определяется `retryable`;
- если envelope нельзя безопасно разобрать, идентификаторы в ответе равны
  `null`.

## Команды

- `analyze_project` — найти все поддерживаемые кандидаты;
- `prepare_defense` — вернуть детерминированный top-N и выбор по seed;
- `materialize_attempt` — скопировать неизменяемый проект и вставить ответ,
  только если digest и offsets всё ещё совпадают.

Все пути относительны workspace runner. Worker канонизирует путь, проверяет его
принадлежность workspace и отклоняет symlinks.

Offsets считаются в байтах исходной последовательности. `body_begin` указывает
на `{`, `body_end` — на байт после `}`:

`signature_begin <= body_begin < body_end <= source_size`.

Ответ содержит только содержимое тела без внешних скобок. Вложенные блоки
разрешены. Перед вставкой проверяется `source_sha256`; несовпадение даёт
`SOURCE_CHANGED`. `seed` передаётся decimal string и не может превышать
`18446744073709551615`.

## Совместимость

- major `1` — граница совместимости;
- optional fields добавляются только вместе с обновлением schema 1.x;
- неизвестные команды, поля и major версии отклоняются;
- Go и C++ contract tests используют общие golden examples.
