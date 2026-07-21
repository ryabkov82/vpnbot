# SHM User Audit (read-only)

Отдельная CLI-утилита для безопасного read-only аудита legacy Telegram-пользователей SHM перед миграцией Friends Connect.

## Назначение

Утилита загружает пользователей, пользовательские услуги, каталог услуг, списания и платежи через SHM Admin API и классифицирует legacy Telegram-пользователей без `settings.brand_id`.

Результат — локальные JSON/CSV-отчёты. Данные SHM **не изменяются**.

## Гарантии read-only

- Отдельный audit client, не связанный с runtime `APIClient`.
- Custom `http.RoundTripper` разрешает только:
  - `POST /shm/user/auth.cgi`
  - `GET /shm/v1/admin/user`
  - `GET /shm/v1/admin/user/service`
  - `GET /shm/v1/admin/service`
  - `GET /shm/v1/admin/user/service/withdraw`
  - `GET /shm/v1/admin/user/pay`
- Любые `PUT` / `PATCH` / `DELETE` и `POST` вне auth блокируются **до** отправки в сеть.
- Нет `--apply`, `--update`, `--migrate`, `--write`, `--fix`.
- Утилита только предлагает действия в отчёте; ничего не применяет.

## Классы

| Класс | Смысл | Proposed action |
|-------|--------|-----------------|
| `fc_only` | Валидная legacy identity, только FC evidence, целевой `@fc_<chat_id>` свободен | rename login + set `brand_id=fc` (**кандидат**, не авторазрешение) |
| `vff_only` | Только VFF evidence | `do_not_migrate` |
| `shared` | Есть и FC, и VFF evidence | `manual_review` |
| `empty` | Нет услуг/списаний/платежей и нулевые балансы | `do_not_migrate_automatically` |
| `ambiguous` | Любая неоднозначность / конфликт / unknown category / unresolved service | `manual_review` |

**Важно:** `fc_only` означает migration candidate для последующего планирования. Это **не** разрешение автоматически менять пользователей.

## CLI flags

| Flag | Required | Default | Описание |
|------|----------|---------|----------|
| `--config` | yes | — | Путь к JSON с секцией `api` (`base_url`, `api_login`, `api_pass`, `timeout_seconds`) |
| `--output` | yes | — | Каталог отчётов (создаётся с mode `0700`) |
| `--fc-category` | yes | — | Точная FC category |
| `--vff-category` | yes | — | Точная VFF category (≠ FC) |
| `--page-size` | no | `250` | `1..1000` |
| `--request-delay` | no | `0` | Задержка между page requests |

Запросы выполняются последовательно, без concurrency.

## Формат output

Каталог: mode `0700`. Файлы: mode `0600`, атомарная запись. Существующие одноимённые файлы **не** перезаписываются.

Файлы:

- `summary.json`
- `legacy-users.json` / `legacy-users.csv`
- `fc-only.json` / `fc-only.csv`
- `vff-only.json` / `vff-only.csv`
- `shared.json` / `shared.csv`
- `empty.json` / `empty.csv`
- `ambiguous.json` / `ambiguous.csv`

В отчёты не попадают: полный `settings`, payment `comment`, credentials, cookies, session id.

## Пример будущего запуска

Пути — placeholders. Не коммитьте реальные конфиги и отчёты.

```bash
go run ./cmd/shm-user-audit \
  --config "$HOME/.cache/vpnbot-fc-rollout/config-fc.json" \
  --output "$HOME/.cache/vpnbot-fc-audit/YYYY-MM-DD" \
  --fc-category vpn-mz-fc \
  --vff-category vpn-mz-test \
  --page-size 250 \
  --request-delay 50ms
```

Либо собранный binary:

```bash
go build -o /tmp/shm-user-audit ./cmd/shm-user-audit
/tmp/shm-user-audit \
  --config /path/to/config-fc.json \
  --output /path/to/audit-out \
  --fc-category vpn-mz-fc \
  --vff-category vpn-mz-test
```

## Чего нет

- Режима apply / migrate / write
- Изменения пользователей SHM
- Автоматического deployment или production-запуска из CI/агента без явного оператора

## Git hygiene

Отчёты аудита, production-конфиги, session cookies и credentials **не коммитить**.
