# Этап 1 — отчёт о готовности

**Дата проверки:** 2026-09-12  
**Основание:** требования 1.4, worker protocol 1.0 и план 1.4

## Результат

C++-часть подготовлена к интеграции с Go Runner. Консольный сценарий v1
сохранён, общая логика вынесена в отдельную библиотечную цель, а
`cpp-defense-worker` обрабатывает один JSON-запрос без интерактивного ввода.

| Область | Статус | Реализация |
|---|---|---|
| Переиспользуемое ядро | Готово | CMake target `cpp-defense-core` |
| Существующий CLI | Сохранён | `cpp-defense`, совместимые include и regression-тесты |
| Headless worker | Готово | `apps/worker/main.cpp`, `src/worker/` |
| Анализ проекта | Готово | `analyze_project` |
| Детерминированная подготовка | Готово | `prepare_defense`, uint64 seed |
| Формирование попытки | Готово | `materialize_attempt` |
| Ответ без внешних скобок | Готово | worker сохраняет исходные `{` и `}` |
| Уникальные workspace | Готово | UUID `session_id` вместо `cache/current` |
| Сохранение подготовки | Готово | атомарный `defense-state.json` |
| Безопасные пути | Готово | только относительные пути внутри session root |
| Неизменяемый вход | Готово | SHA-256 и отдельное дерево результата |
| Вызов из Go | Проверен | `tests/worker_smoke.go` |
| Контрактные тесты | Готово | C++, Python, JSON Schema |
| Sanitizers | Готово | локальный прогон и Linux CI job |

## Границы компонентов

`cpp-defense-core` содержит parser, deterministic candidate picker, timer и
SHA-256. Ядро не зависит от CLI, JSON или `nlohmann/json`.

CLI использует application/infrastructure поверх того же core. Worker имеет
отдельный JSON-adapter и безопасные операции с runner workspace.
`nlohmann/json 3.12.0` загружается только при сборке worker и закреплён SHA-256
архива.

Worker не запускает CMake, бинарные файлы или CTest. Он анализирует и
материализует проект. Выполнение недоверенного проекта остаётся обязанностью
Runner и контейнерного sandbox на Этапах 7–8.

## Протокол

- stdin: один UTF-8 JSON request;
- stdout: одна строка с JSON response;
- stderr: только диагностика процесса;
- protocol major/minor: `1.0`;
- команды: `analyze_project`, `prepare_defense`,
  `materialize_attempt`;
- ответ студента содержит тело функции без внешних скобок;
- одинаковые проект, parser options, top-N и seed возвращают тот же выбор;
- повторная подготовка с другими данными в том же session возвращает
  `SESSION_CONFLICT`;
- повреждённый request или проект возвращает структурированный
  `status: error`.

## Изоляция файлов

Runner заранее создаёт `<workspace>/<session_id>/project`. Worker:

- принимает только канонический UUID и относительные protocol paths;
- отклоняет `..`, абсолютные пути, backslash, symlink, hard link и special file;
- ограничивает глубину, число файлов, размер файла, общий размер request и response;
- блокирует параллельное изменение одной session;
- пишет попытку сначала во временное дерево и публикует её rename;
- удаляет незавершённое временное дерево при исключении;
- сверяет SHA-256 исходника и повторно разбирает функцию перед заменой тела.

## Проверка

Обычная Debug-сборка и отдельная сборка с
AddressSanitizer/UndefinedBehaviorSanitizer прошли полностью:

- 104/104 CTest;
- старые CLI, parser, scanner, picker, masker, session и build-runner тесты;
- одинаковый seed даёт одинаковый выбор;
- две параллельные session сохраняют раздельные состояния;
- исходный проект остаётся неизменным;
- source digest mismatch отклоняется;
- все три живые команды worker и error response проходят JSON Schema;
- Go-процесс запускает worker и читает JSON result.

## Следующий этап

Этап 2 может создавать Go Backend и PostgreSQL независимо от Runner. Для
интеграции worker Go-код должен использовать схемы из
`contracts/worker/v1`, создавать session workspace и считать non-zero exit
аварией процесса, а `status: error` — обработанным результатом протокола.
