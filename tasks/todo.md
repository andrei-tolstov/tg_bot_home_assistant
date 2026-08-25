# Задачи: Создание Docker Compose конфигурации для бота

- [x] Создать `.dockerignore` для оптимизации контекста сборки
- [x] Создать `Dockerfile` с многоэтапной сборкой (multi-stage build), `ca-certificates` и non-root пользователем
- [x] Создать `docker-compose.yml` с автоматическим перезапуском и подключением `.env`
- [x] Зафиксировать результаты в `tasks/todo.md` и `tasks/lessons.md`

## Результаты
- Создан [.dockerignore](file:///c:/Users/andrei/tg_bot_home_assistant/.dockerignore) (исключены лишние бинарники, старый python-код, логи и задачи).
- Создан оптимизированный [Dockerfile](file:///c:/Users/andrei/tg_bot_home_assistant/Dockerfile) с multi-stage сборкой:
  - Этап 1 (`builder`): сборка статического Go-бинарника без CGO (`CGO_ENABLED=0 GOOS=linux`).
  - Этап 2 (`runner`): минимальный образ `alpine:latest` (~15 МБ), SSL-сертификаты `ca-certificates` для работы с Telegram API / Home Assistant API, таймзона `tzdata` и запуск от непривилегированного пользователя `botuser`.
- Создан [docker-compose.yml](file:///c:/Users/andrei/tg_bot_home_assistant/docker-compose.yml) с подключением `.env`, политикой перезапуска `unless-stopped` и ротацией логов.
