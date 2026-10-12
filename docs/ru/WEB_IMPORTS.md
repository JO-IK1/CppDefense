# Импорт групп и лабораторных в веб-версии

## Состав групп

Преподаватель загружает JSON, проверяет предварительный список изменений и
только затем подтверждает импорт. Все изменения применяются одной транзакцией.
Повторная загрузка обновляет ФИО и группу существующей записи по
GitHub-логину, не создавая дубликат.

```json
{
  "schema_version": 1,
  "organization": {
    "github_login": "is-itmo-c-26",
    "name": "ИС-26"
  },
  "groups": [
    {
      "code": "26-013",
      "name": "26-013",
      "students": [
        {
          "full_name": "Иванов Иван Иванович",
          "github_login": "ivanov"
        }
      ]
    }
  ]
}
```

## ZIP студента

Студент загружает обычный ZIP проекта без `cppdefense-manifest.json`. Имя
файла имеет вид `labwork1-USERNAME-deadline_0.zip`, где `USERNAME` должен
совпадать с GitHub-логином текущего аккаунта. Код группы берётся из привязанной
записи студента. Корень проекта может находиться непосредственно в ZIP или в
единственной верхней папке, как в архиве GitHub.

Система проверяет пути, лимиты архива, наличие корневого `CMakeLists.txt` и
C/C++ исходников, создаёт внутренний manifest и отправляет версию на проверку
преподавателю. До подтверждения версия не появляется в списке доступных защит.

## GoogleTest

Проверка выполняется без сети в `cppdefense-sandbox`. Образ содержит CMake,
CTest, GCC и системный GoogleTest. Рекомендуемый CMake-код лаборатории:

```cmake
find_package(GTest REQUIRED)
add_executable(lab_tests tests.cpp)
target_link_libraries(lab_tests PRIVATE GTest::gtest_main)
include(GoogleTest)
gtest_discover_tests(lab_tests)
```

Старые проекты с `FetchContent_Declare(googletest ...)` также используют
локальный `/usr/src/googletest`: runner передаёт
`FETCHCONTENT_SOURCE_DIR_GOOGLETEST` и запрещает сетевую загрузку зависимостей.
Стандартная последовательность проверки — configure, build и
`ctest --output-on-failure`.

При production-деплое `deploy/install-runner-release.sh` пересобирает sandbox
из `deploy/sandbox.Dockerfile`, затем устанавливает новые runner/worker и
перезапускает службу. Поэтому отдельная установка GoogleTest в Ubuntu VPS не
нужна.

После первого обновления этого защищённого root-скрипта его нужно один раз
переустановить на VPS вручную:

```bash
cd /opt/cppdefense/source
sudo install -o root -g root -m 0755 \
  deploy/install-runner-release.sh \
  /usr/local/sbin/cppdefense-install-runner
```

Обычный деплой проверяет точное совпадение установленной копии и останавливается
до изменения приложения, если администратор ещё не выполнил этот шаг. Так
пользователь `cppadmin` не получает возможность незаметно подменить скрипт,
исполняемый от `root`.
