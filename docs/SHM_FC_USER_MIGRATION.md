# SHM FC User Migration

Одноразовая CLI-утилита для миграции пользователей из аудита (`fc_only`):

- старый login: `@<telegram_chat_id>`
- новый login: `@fc_<telegram_chat_id>`
- `settings.brand_id`: `fc`
- `user_id` не меняется

## Источник plan

`--plan` принимает файл `fc-only.json` из read-only аудита.

Команда **не** ищет кандидатов сама и **не** читает `legacy-users.json`.  
Обрабатываются только записи allowlist из plan.

## Dry-run по умолчанию

Без `--apply` команда только:

1. проверяет plan;
2. аутентифицируется в SHM;
3. выполняет preflight всех пользователей;
4. пишет `preflight.json` и `result.json`;
5. **не** вызывает `POST /shm/v1/admin/user`.

## Явный `--apply`

Запись в SHM выполняется только с `--apply` и только после успешного preflight **всего** списка.

Перед первым update создаются:

- `backup-before.json` — live-снимок (включая raw `settings`)
- `preflight.json`
- после выполнения — `result.json`

Автоматический rollback **не** реализован.

## Preflight

Для каждой записи допустимы состояния:

- `ready` — можно мигрировать
- `already_migrated` — уже `@fc_<id>` + `brand_id=fc`, update пропускается

Любая ошибка preflight останавливает запуск: **ни один** update не выполняется.

## Повторный запуск

После частичного apply повторный запуск распознаёт успешно изменённых пользователей как `already_migrated` и продолжает оставшихся `ready`.

## Примеры (placeholder-пути)

Dry-run:

```bash
go run ./cmd/shm-user-migrate-fc \
  --config "$HOME/.cache/vpnbot-fc-rollout/config-fc.json" \
  --plan "$AUDIT_DIR/fc-only.json" \
  --output "$HOME/.cache/vpnbot-fc-migration/<timestamp>"
```

Apply:

```bash
go run ./cmd/shm-user-migrate-fc \
  --config "$HOME/.cache/vpnbot-fc-rollout/config-fc.json" \
  --plan "$AUDIT_DIR/fc-only.json" \
  --output "$HOME/.cache/vpnbot-fc-migration/<timestamp>" \
  --apply
```

Не коммитить plan/backup/result и production-конфиги.
