# SRE Incident Tracker

Простое клиент-серверное приложение: каталог сервисов и учёт инцидентов. Go 1.26 + JS + PostgreSQL.

## Запуск (предпочтительно)

```bash
docker compose up --build
```

UI: http://localhost:8080

## Локальный запуск без Docker (API)

Нужен запущенный PostgreSQL и переменные из `.env.example`.

```bash
copy .env.example .env
go run ./cmd/migrate
go run ./cmd/api
```

## API

REST JSON, HTTP/1.1:

- `/api/services` — CRUD сервисов
- `/api/incidents` — CRUD инцидентов
- `/healthz` — здоровье процесса
