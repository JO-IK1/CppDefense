# План развития

[English](../ROADMAP.md) | **Русский**

## Реализовано в 2.1

- C++23 CLI, переиспользуемое ядро и headless worker, 105 CTest-сценариев;
- GitHub OAuth, подтверждение identity, роли, sessions, CSRF и admin preview;
- ограниченный ZIP import с review и неизменяемыми версиями работ;
- PostgreSQL queue, Runner Agent, leases, retries, attempts и history;
- rootless Podman без сети и с resource limits;
- repository browser студента, колесо, изменяемые панели, drafts и понятные
  configure/build/CTest reports;
- подтверждение функции и история компиляций для teacher/admin;
- Docker Compose, Caddy HTTPS, backup/restore и CI/CD защищённой `main`;
- основная английская документация и русские переводы.

## Приоритеты усиления production

1. Автоматизировать encrypted off-host backup и доказать restore на другом VPS.
2. Добавить monitoring/alerts диска, RAM, containers, runner heartbeat, queue
   age, сертификата и возраста backup.
3. Вынести Runner Agent и недоверенные containers на отдельный сервер/VM.
4. Создать read-only MinIO identity для runner и сменить setup credentials.
5. Добавить browser E2E tests для OAuth stubs, import, teacher confirmation,
   polling, resizer, attempts и role preview.
6. Проверить вредоносные проекты и одну/две параллельные компиляции на реальном
   VPS 2 vCPU / 4 ГБ, зафиксировать безопасные limits.
7. Добавить обновление runner или compatibility gate в deploy.
8. До длительной публичной работы оценить поддерживаемое S3-хранилище.

## Будущие функции

- запись в «контест» после загрузки лабораторной;
- автоматическая постановка балла после успешной сдачи;
- незачётный режим тестовой защиты рядом с официальным;
- отчёты преподавателя, filters, export и понятное восстановление;
- независимые runner pools для крупных групп.

Local CLI остаётся поддерживаемым. Безопасность и контракты важнее новых
возможностей.
