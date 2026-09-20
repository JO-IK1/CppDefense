# CppDefense 2.0: запуск сайта

Ниже описана рекомендуемая схема для домашнего Windows-ноутбука: приложение
работает в Ubuntu VM №1, а недоверенные студенческие проекты — в Ubuntu VM №2.
Обе VM должны иметь статические адреса в приватной сети Hyper-V.

## 0. Превращаем Windows-ноутбук в сервер

### Железо и режим работы

Практический минимум — 16 ГБ RAM, SSD со 150 ГБ свободного места и процессор с
виртуализацией. Комфортный вариант — 32 ГБ RAM. Для начала выделите:

| Система | CPU | RAM | Диск |
|---|---:|---:|---:|
| Windows-хост | оставить не меньше 4 потоков | 6–8 ГБ | системный диск |
| VM №1, приложение | 2–4 vCPU | 4–6 ГБ | 50–60 ГБ, dynamic |
| VM №2, проверки | 4–6 vCPU | 6–10 ГБ | 70–100 ГБ, dynamic |

В BIOS/UEFI включите Intel VT-x/VT-d или AMD-V/SVM. В Windows нужен Hyper-V,
то есть редакция Pro/Enterprise/Education. В PowerShell от администратора:

```powershell
Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All
```

После перезагрузки отключите автоматический сон при питании от сети. Ноутбук
можно закрывать только если действие закрытия крышки также выставлено в
«Ничего не делать». Для постоянной работы желательно проводное подключение,
охлаждаемая поверхность и ИБП. Не отключайте экран блокировки и пароль Windows.

### Сеть Hyper-V

1. В **Hyper-V Manager → Virtual Switch Manager** создайте **External switch**
   на Ethernet-адаптере и разрешите Windows использовать тот же адаптер.
2. Создайте две VM поколения 2 из ISO Ubuntu Server 24.04 LTS, подключите их к
   этому switch и включите Secure Boot с шаблоном `Microsoft UEFI Certificate
   Authority`.
3. В роутере закрепите DHCP-адреса по MAC, например VM №1 —
   `192.168.100.10`, VM №2 — `192.168.100.11`. Это надёжнее ручной настройки
   адресов внутри Ubuntu.
4. Назначьте понятные имена: `cppdefense-app` и `cppdefense-runner`. Установите
   OpenSSH Server во время установки Ubuntu.
5. Проверьте с Windows: `ssh USER@192.168.100.10` и
   `ssh USER@192.168.100.11`.

Пробрасывать порты на домашнем роутере не нужно: внешний HTTPS-трафик пойдёт
через исходящее соединение Cloudflare Tunnel. External switch нужен для связи
VM между собой, обновлений и администрирования из домашней сети.

## 1. Что установить

### VM №1 — приложение

- Ubuntu Server 24.04 LTS;
- Git;
- Docker Engine с Compose v2;
- `curl`, `openssl` и `rsync`;
- каталог резервных копий на отдельном физическом диске или внешнем
  накопителе.

После установки Ubuntu выполните:

```sh
sudo apt update && sudo apt full-upgrade -y
sudo apt install -y ca-certificates curl git openssl rsync ufw
```

Docker Engine и плагин Compose устанавливайте из официального apt-репозитория
Docker для Ubuntu, а не случайным скриптом. После установки проверьте
`sudo docker run --rm hello-world` и `docker compose version`. Добавьте своего
административного пользователя в группу `docker`, затем выйдите из SSH и
войдите снова:

```sh
sudo usermod -aG docker "$USER"
```

### VM №2 — runner

- Ubuntu Server 24.04 LTS;
- CMake 3.20+, Ninja, компилятор с C++23, Go 1.27 и Git для сборки;
- rootless Podman;
- отдельный непривилегированный пользователь `cppdefense-runner`;
- доступ по приватной сети только к Backend API и MinIO на VM №1.

Установите зависимости сборки:

```sh
sudo apt update && sudo apt full-upgrade -y
sudo apt install -y build-essential cmake ninja-build git curl podman uidmap slirp4netns fuse-overlayfs ufw
```

Go должен соответствовать версии из `backend/go.mod` (сейчас 1.27.1). Ставьте
его с официальной страницы Go и проверьте `go version`; пакет Ubuntu может быть
старее.

Не устанавливайте runner на VM №1: CMake-проекты студентов считаются
недоверенными. Контейнер уменьшает риск, но отдельная VM остаётся обязательной
границей.

## 2. GitHub OAuth и Cloudflare

1. В GitHub откройте **Settings → Developer settings → OAuth Apps** и создайте
   OAuth App. Homepage URL — будущий HTTPS-адрес сайта, callback URL —
   `https://ДОМЕН/api/v1/auth/github/callback`.
2. Узнайте свой числовой GitHub ID (не login). Он станет первым администратором.
3. В Cloudflare Zero Trust создайте Tunnel и public hostname, направленный на
   `http://backend:8080`. Скопируйте tunnel token.
4. Не публикуйте порты PostgreSQL и MinIO в интернет. Внешний вход идёт только
   через Cloudflare Tunnel.

Приложение запрашивает у GitHub только основную информацию профиля. Доступ к
репозиториям студентов не нужен: лабораторные загружаются ZIP-архивами.

Последовательность настройки важна:

1. Добавьте домен в Cloudflare и дождитесь активного статуса DNS-зоны.
2. Создайте remotely-managed Tunnel, но пока не запускайте отдельную команду
   установки connector: `cloudflared` уже включён в Compose.
3. Добавьте **Published application**: hostname `cppdefense.example`, service
   `http://backend:8080`, затем скопируйте токен Tunnel.
4. Создайте GitHub OAuth App уже с окончательным HTTPS-доменом. Callback должен
   совпадать посимвольно, включая `/api/v1/auth/github/callback`.
5. Токены Cloudflare и GitHub считайте паролями: не вставляйте их в issue,
   скриншоты и Git. Файл `production.env` уже исключён из репозитория.

## 3. VM №1 — запуск приложения

```sh
git clone https://github.com/JO-IK1/CppDefense.git
cd CppDefense/deploy
cp production.env.example production.env
chmod 600 production.env
```

Сгенерируйте независимые секреты:

```sh
openssl rand -hex 32
openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
```

Заполните `production.env`: приватный IP VM №1, домен, три разных
base64url-ключа, пароли, GitHub client ID/secret, числовой GitHub ID, общий
runner token и Cloudflare token. Для трёх ключей выполните команду три раза и
не повторяйте значения. Числовой GitHub ID можно получить через
`https://api.github.com/users/ВАШ_LOGIN`. Затем проверьте и запустите:

```sh
./release-check.sh production.env
docker compose --env-file production.env -f production.compose.yaml config
docker compose --env-file production.env -f production.compose.yaml build
docker compose --env-file production.env -f production.compose.yaml up -d
./healthcheck.sh http://VM1_PRIVATE_IP:8080
```

Проверьте состояние контейнеров: `docker compose --env-file production.env -f
production.compose.yaml ps`. Все основные сервисы должны быть `healthy`, а
`cloudflared` — `running`. Если backend не поднялся, сначала смотрите
`docker compose --env-file production.env -f production.compose.yaml logs
--tail=200 backend postgres minio`.

Откройте домен и войдите через GitHub. Аккаунт с
`CPPDEFENSE_BOOTSTRAP_ADMIN_GITHUB_ID` автоматически станет администратором.
Данные сохраняются в именованных томах `postgres-data` и `minio-data` после
перезапуска контейнеров и VM. Не выполняйте `docker compose down -v`, если
нужно сохранить данные.

## 4. VM №2 — сборка и запуск runner

Соберите C++ worker и Go runner из того же revision, что развернут на VM №1:

```sh
git clone https://github.com/JO-IK1/CppDefense.git
cd CppDefense
cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build --parallel 2 --target cpp-defense-worker
go -C backend build -trimpath -o ../build/cppdefense-runner ./cmd/runner
sudo install -m 0755 build/cpp-defense-worker /opt/cppdefense/bin/
sudo install -m 0755 build/cppdefense-runner /opt/cppdefense/bin/
```

Создайте пользователя и каталоги, включите rootless Podman и соберите sandbox:

```sh
sudo useradd --system --create-home --home-dir /var/lib/cppdefense-runner --shell /usr/sbin/nologin cppdefense-runner
sudo usermod --add-subuids 100000-165535 cppdefense-runner
sudo usermod --add-subgids 100000-165535 cppdefense-runner
sudo loginctl enable-linger cppdefense-runner
sudo -u cppdefense-runner podman build -t localhost/cppdefense-sandbox:2.0 -f deploy/sandbox.Dockerfile .
sudo install -d -m 0750 /etc/cppdefense
sudo cp deploy/runner.env.example /etc/cppdefense/runner.env
sudo chmod 600 /etc/cppdefense/runner.env
```

В `runner.env` укажите приватный URL VM №1, одинаковый runner token, UUID
runner и отдельные read-only MinIO credentials. Значение
`CPPDEFENSE_RUNNER_SLOTS=4` подходит для старта; увеличивайте до 6 только после
проверки RAM и CPU. Установите сервис:

```sh
sudo cp deploy/cppdefense-runner.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cppdefense-runner
sudo systemctl status cppdefense-runner
```

В `runner.env` предпочтительно укажите приватный адрес
`CPPDEFENSE_BACKEND_URL=http://192.168.100.10:8080`: задания не будут ходить
через публичный интернет. Проверьте rootless-режим командой
`sudo -u cppdefense-runner podman info`; в выводе `rootless` должно быть
`true`. Если сервис не стартовал, используйте
`sudo journalctl -u cppdefense-runner -n 200 --no-pager`.

Compose привязывает Backend `:8080` и MinIO `:9000` только к адресу
`CPPDEFENSE_PRIVATE_BIND_IP`. Добавьте на VM №1 firewall-правила для этих
портов только с адреса VM №2. Более строгий вариант — отдельный S3 reverse
proxy с TLS. Никогда не используйте root-пару MinIO в runner: создайте
отдельного MinIO-пользователя с политикой чтения bucket `cppdefense` и внесите
его ключи в `/etc/cppdefense/runner.env`.

На VM №1 включите firewall после проверки SSH. Подставьте свою домашнюю сеть и
адрес VM №2:

```sh
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from 192.168.100.0/24 to any port 22 proto tcp
sudo ufw allow from 192.168.100.11 to any port 8080 proto tcp
sudo ufw allow from 192.168.100.11 to any port 9000 proto tcp
sudo ufw enable
```

На VM №2 оставьте входящим только SSH из домашней сети. Runner сам создаёт
исходящие соединения к VM №1 и не требует публичных портов.

## 5. Первый рабочий сценарий

1. Администратор создаёт группу и назначает преподавателя.
2. Преподаватель создаёт лабораторную.
3. Преподаватель загружает общий или одиночный ZIP, проверяет preview и SHA-256,
   затем подтверждает и применяет импорт.
4. Студент входит через GitHub. Совпавший login привязывается автоматически;
   неоднозначную запись подтверждает преподаватель или администратор.
5. Студент запускает защиту. Runner выбирает функцию, принимает ответ и запускает
   CMake → build → CTest в контейнере без сети.
6. Преподаватель и администратор видят историю и JSON-результаты компиляций.
7. Администратор может включить read-only режим просмотра интерфейса
   преподавателя или демонстрационного студента.

До приглашения настоящих студентов выполните сценарий целиком тремя тестовыми
GitHub-аккаунтами. После этого одновременно запустите четыре защиты и проверьте,
что лишние задания ждут очередь, VM №2 не уходит в swap, а в истории остаются
результаты и логи.

## 6. Резервные копии и восстановление

`backup.sh` делает согласованный dump PostgreSQL и, ненадолго остановив MinIO,
архивирует объектное хранилище через отдельный служебный контейнер. Скрипту не
нужны shell или `tar` внутри образа MinIO:

```sh
sudo CPPDEFENSE_BACKUP_DIR=/mnt/backup/cppdefense ./backup.sh
```

Храните хотя бы одну копию вне ноутбука. Для ежедневного запуска добавьте
systemd timer или cron. Раз в месяц проверяйте восстановление на отдельной
тестовой VM. Восстановление заменяет текущую БД и MinIO и поэтому требует явного
подтверждения:

```sh
sudo CPPDEFENSE_RESTORE_CONFIRM=RESTORE ./restore.sh /mnt/backup/cppdefense/20260920T120000Z
```

После восстановления запустите `healthcheck.sh`, войдите тестовым аккаунтом и
откройте одну ранее импортированную работу.

Compose закрепляет последний security-релиз legacy MinIO. Upstream прекратил
развитие community-репозитория, поэтому для домашнего запуска образ оставлен
фиксированным, а перед длительной публичной эксплуатацией стоит запланировать
переезд на поддерживаемое S3-совместимое хранилище. Backend и Runner уже
используют S3-интерфейс, поэтому прикладную логику для этого менять не нужно.

## 7. Обновление и диагностика

Перед обновлением сделайте backup. Затем:

```sh
git pull --ff-only
cd deploy
docker compose --env-file production.env -f production.compose.yaml build
docker compose --env-file production.env -f production.compose.yaml up -d
./healthcheck.sh
```

Полезные проверки:

```sh
docker compose --env-file production.env -f production.compose.yaml ps
docker compose --env-file production.env -f production.compose.yaml logs --tail=200 backend cloudflared
sudo journalctl -u cppdefense-runner -n 200 --no-pager
```

Базовый тест доступности для расчётных 100 одновременных пользователей:

```sh
./load-test.sh https://ДОМЕН 1000 100
```

Он проверяет HTTP/БД/MinIO readiness под параллельными запросами. Отдельно
запустите 4–6 настоящих защит на тестовых аккаунтах: это проверяет CPU, RAM и
Podman VM №2, чего HTTP-тест намеренно не имитирует.

Перед постоянной эксплуатацией нужны: внешний backup, проверка восстановления,
алерты на свободное место/недоступный runner, тест 4–6 одновременных сборок и
регулярное обновление Ubuntu, Docker/Podman и образов.

## 8. Что переживает выключение ноутбука

PostgreSQL и MinIO хранят данные в именованных Docker volumes на виртуальном
диске VM №1. Обычное выключение контейнеров, VM или Windows их не удаляет.
Перед выключением хоста корректно остановите VM из Ubuntu (`sudo poweroff`) или
средствами Hyper-V. Опасны только удаление VHDX, `docker compose down -v` и
повреждение единственного SSD — поэтому именованные volumes не заменяют backup.

Рекомендуемый режим:

1. Ежедневно запускать `backup.sh` на VM №1.
2. Каталог backup хранить на отдельном физическом диске или NAS, а не внутри
   того же VHDX.
3. Раз в месяц восстанавливать свежую копию на временной VM.
4. Перед обновлением проекта делать отдельный backup.
5. В Hyper-V настроить автоматический запуск обеих VM вместе с Windows; VM №1
   запускается первой, VM №2 — с задержкой 60 секунд.
