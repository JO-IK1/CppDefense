# Использование локального CLI

[English](../USAGE.md) | **Русский**

CppDefense превращает доверенный CMake-проект в защиту по восстановлению кода.
Он собирает и запускает проект, поэтому не используйте local CLI с недоверенным
кодом; для него предназначен sandbox веб-версии.

## Требования и сборка

- компилятор C++23;
- CMake 3.24+ и CTest;
- C/C++ CMake-проект с тестами.

```sh
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

Single-config генератор создаёт `build/cpp-defense`; multi-config может положить
его в `build/Debug` или `build/Release`.

## Запуск

```sh
./build/cpp-defense ./examples/labwork_simple -n 5 -t 10
```

```text
-p, --path <directory>     каталог проекта
-n, --functions <count>    число кандидатов (1–50)
-t, --timer <minutes>      лимит времени (1–180)
    --functions-only       только функции (по умолчанию)
    --all                  все поддерживаемые entity
-h, --help                 справка
```

Путь проекта можно передать первым positional argument.

## Сценарий защиты

Команда `start` создаёт сессию, копирует проект, выбирает entity, скрывает
реализацию, создаёт `result.txt` и запускает таймер.

В `result.txt` пишется только содержимое внутри внешних скобок. Не повторяйте
signature и сами скобки:

```cpp
return first + second;
```

`check` создаёт временную копию, вставляет ответ, выполняет CMake configure,
build и CTest. Ошибка оставляет сессию активной до дедлайна, успех завершает её.
`quit` или EOF завершает незаконченный сеанс как failed.

```text
start                    начать или перезапустить защиту
check                    проверить result.txt
info                     показать entity и пути workspace
time                     показать остаток времени
help                     показать команды
quit                     сохранить и выйти
```

## Workspace и результат

Runtime root: `%LOCALAPPDATA%/CppDefense` на Windows,
`~/Library/Caches/CppDefense` на macOS и `$XDG_CACHE_HOME/cpp-defense` (или
`~/.cache/cpp-defense`) на Unix. Переменная `CPP_DEFENSE_HOME` меняет путь.

```text
cache/<session-id>/
├── project/              замаскированная копия
├── logs/                 configure/build/test logs
├── metadata/
├── result.txt            ответ
└── defense_result.txt    итоговый отчёт
```

Каталог `check/` пересоздаётся при каждой попытке. Отчёт содержит entity, число
попыток, время, status и последние ограниченные logs. Оригинал не изменяется.

Подробности: [архитектура](ARCHITECTURE.md).
