# Выпуск CppDefense

[English](../RELEASING.md) | **Русский**

1. Начните с чистой ветки от актуальной `main`.
2. Обновите `project(CppDefense VERSION ...)` и версии images/app.
3. Обновите английские и русские документы; английские являются основными.
4. Запустите полный локальный gate:

   ```sh
   ./deploy/release-check.sh deploy/production.env
   ```

5. Проверьте workflow контрактов: JSON Schema, negative fixtures, OpenAPI и
   live worker.
6. Откройте Pull Request и дождитесь всех обязательных checks.
7. Merge без bypass, deploy `main`, публичные live/ready и одна синтетическая
   сквозная защита.
8. Создайте и отправьте annotated tag:

   ```sh
   git tag -a v2.1.0 -m "CppDefense 2.1.0"
   git push origin v2.1.0
   ```

Tag публикует platform archives через GitHub Actions. CMake install включает
CLI, worker, README, license и docs. Production deploy из `main` обновляет
только backend/frontend. Если менялись C++, Runner Agent, systemd config или
sandbox, выполните ручное обновление runner по deployment guide.

Перед production release создайте off-host backup и проверьте rollback. Не
ставьте tag, пока migration не проверена на копии production data.
