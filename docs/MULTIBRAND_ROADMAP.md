# Roadmap мультибрендинга

Рабочий технический документ и источник истины для дальнейшей реализации мультибрендинга в `vpnbot`.

Статусы:

- ✅ Готово
- 🟡 Частично
- ⬜ Не начато

---

## 1. Цель

Целевая модель:

- одна кодовая база;
- один binary;
- один активный бренд на процесс;
- отдельный runtime-контур на каждый бренд;
- общий SHM backend;
- строгое разделение пользователей, услуг, платежей и публичных URL.

Добавление нового бренда в будущем не должно требовать изменения общей бизнес-логики. Brand-specific поведение задаётся конфигурацией и identity-правилами, а не разветвлением кода по `if brand == ...`.

---

## 2. Архитектурные принципы

Принятые решения:

1. **one process = one BrandConfig**  
   Процесс загружает ровно одну явную и полностью валидную секцию `brand`. Неявный fallback к VFF запрещён.

2. **Отдельный runtime-контур на бренд**  
   Каждому бренду соответствуют:
   - systemd service;
   - runtime directory;
   - explicit config;
   - public domain / allowed hosts;
   - SHM service category;
   - payment profile.

3. **Общие секреты и backend допустимы, brand identity — явная**  
   Несколько процессов могут использовать общий SHM/API backend, но активный бренд и его identity-правила всегда берутся из runtime config текущего процесса.

4. **Cross-brand fallback запрещён**  
   Поиск пользователя не должен «догонять» запись другого бренда. Несовпадение identity нельзя трактовать как обычный not found (риск последующей регистрации в занятый login).

Ключевые реализации:

- `internal/config/brand.go` — модель и строгая валидация `BrandConfig`;
- `internal/service/brand_user.go` — Telegram login / brand membership / `ErrUserIdentityMismatch`;
- `deploy/brands/vff.json`, `deploy/brands/fc.json` — deployment profiles;
- `scripts/lib/brand_profile.sh` — операционные brand profiles.

---

## 3. Матрица действующих брендов

Значения из `deploy/brands/vff.json` и `deploy/brands/fc.json`.

| Параметр | VFF | Friends Connect |
|----------|-----|-----------------|
| Brand ID | `vff` | `fc` |
| Название | VPN for Friends | Friends Connect |
| Service | `bot.service` | `bot-friends-connect.service` |
| Runtime | `/opt/bot` | `/opt/bot-friends-connect` |
| Explicit config | `config-vff.json` | `config-fc.json` |
| Public host | `connect.vpn-for-friends.com` | `connect-fc.vpn-for-friends.com` |
| Landing | `vpn-for-friends.com` | `friends-connect.club` |
| SHM category | `vpn-mz-test` | `vpn-mz-fc` |
| Payment profile | `telegram_bot` | `telegram_friends_connect_bot` |
| Telegram login | `@<chat_id>` | `@fc_<chat_id>` |

Дополнительно (из brand profiles):

| Параметр | VFF | Friends Connect |
|----------|-----|-----------------|
| `web_user_login_prefix` | `web_` | `web_` |
| `web_user_source` | `vpn-for-friends.com` | `vpn-for-friends.com` |

Одинаковые web prefix/source — переходный разрыв: логическая изоляция есть, физическое разведение prefix ещё впереди (см. §5 / M5).

---

## 4. Статус реализации

### 4.1 Конфигурационная модель — ✅ Готово

Реализовано в `internal/config/brand.go` и связанных проверках:

- `BrandConfig`;
- обязательная валидация при старте;
- `allowed_hosts`;
- `public_base_url`;
- `landing_url`;
- `service_category`;
- `web_user_login_prefix`;
- `web_user_source`;
- `payment_profile`.

Runtime без полной секции `brand` не допускается.

### 4.2 Независимые runtime-контуры — ✅ Готово

Реализовано через `deploy/brands/*` и brand ops/rollout scripts:

- отдельные systemd units;
- отдельные каталоги и explicit configs;
- brand profiles (`scripts/lib/brand_profile.sh`);
- smoke / status / logs / deploy / rollout;
- binary-only deployment без передачи config: `make deploy-fc`;
- coordinated rollout при изменении конфигурации: `make rollout-fc CONFIG=/path/to/config-fc.json`.

### 4.3 Разделение услуг — ✅ Готово

- VFF и FC используют разные SHM categories (`vpn-mz-test` / `vpn-mz-fc`);
- операции с услугами ограничены категорией активного бренда;
- нельзя отображать или изменять услуги другого бренда через runtime активного процесса.

### 4.4 Telegram identity — ✅ Готово

Правила (`internal/service/brand_user.go`):

**VFF**

- login = `@<telegram_chat_id>`
- `settings.brand_id = vff` либо legacy empty для существующих пользователей

**FC**

- login = `@fc_<telegram_chat_id>`
- `settings.brand_id = fc`

**Будущий произвольный бренд**

- login = `@<brand_id>_<telegram_chat_id>`

Дополнительно зафиксировано:

- один Telegram-аккаунт может иметь независимые VFF и FC identities;
- FC не выполняет fallback на `@<chat_id>`;
- проверяются login, `telegram.chat_id` и `settings.brand_id`;
- legacy-совместимость сохраняется только для VFF;
- миграция 11 FC-пользователей завершена;
- одноразовая migration CLI удалена после завершения операции;
- read-only `cmd/shm-user-audit` / `internal/shmaudit` / `docs/SHM_USER_AUDIT.md` сохранены для будущих аудитов.

Персональные Telegram ID пользователей в этот документ не включаются.

---

## 5. Независимые web identities — 🟡 Частично

Этап **частично реализован** (логическая изоляция). Физическое разведение login prefix ещё впереди.

Аудит lifecycle: `docs/MULTIBRAND_WEB_IDENTITY_AUDIT.md` (исторический snapshot).

### Реализовано

- web membership validation (`internal/service/web_brand_user.go`);
- `settings.brand_id` при новой web registration;
- brand-bound account/link/signup tokens (`brand_id` в claims, fail-closed без dual-read);
- повторная brand validation account handlers (`ValidateWebAccountUser` + `authenticateWebAccount`);
- brand-aware Telegram ↔ web linking (canonical Telegram login + brand membership).

Переходное состояние deploy profiles (без изменения на этом этапе):

| Поле | VFF | FC |
|------|-----|----|
| `web_user_login_prefix` | `web_` | `web_` |
| `web_user_source` | `vpn-for-friends.com` | `vpn-for-friends.com` |

При общем prefix одинаковый email по-прежнему занимает один SHM login. Второй runtime возвращает `ErrUserIdentityMismatch` и не выдаёт чужую сессию / не регистрирует нового user.

### Осталось

- coordinated переход FC с `web_` на `web_fc_`;
- ручная корректировка единственной известной FC web-привязки;
- production verification;
- e2e smoke VFF/FC;
- подтверждение независимой регистрации одного email в двух брендах.

### Предварительное целевое направление (не утверждено)

- VFF: `web_<hash(email)>`
- FC: `web_fc_<hash(email)>`

либо общий шаблон `web_<brand_id>_`.

### Критерии полного завершения M5

- один email может независимо зарегистрироваться в VFF и FC;
- session/magic link/OAuth/linking не пересекают бренды;
- услуги другого бренда недоступны;
- существующие web-пользователи проаудированы; миграция при необходимости — allowlist-based, dry-run-first.

---

## 6. Платежи — 🟡 Частично

Разные `payment_profile` уже заданы в brand profiles (`telegram_bot` / `telegram_friends_connect_bot`).

Это **не** означает, что весь платёжный контур уже полностью разделён. Требуется end-to-end аудит:

- выбор payment profile;
- создание счёта;
- категория услуги;
- metadata/comment платежа;
- callback/webhook;
- success URL;
- fail URL;
- возврат на домен активного бренда;
- письма после оплаты;
- повторная обработка callback;
- невозможность активировать услугу другого бренда.

Статус: профили существуют; полнота изоляции — ещё не подтверждена.

---

## 7. Контент и коммуникации — 🟡 Частично

Точки проверки:

- название бренда;
- логотип;
- Telegram-тексты;
- support URL;
- news channel;
- email sender/name;
- email templates;
- заголовки страниц;
- favicon;
- static assets;
- тексты ошибок;
- ссылки на landing;
- отсутствие VFF-oriented fallback в FC.

Известный технический долг: в bot runtime есть VFF-oriented default logo URL  
(`internal/app/bot/service.go`, `defaultLogoURL = "https://vpn-for-friends.com/logobot.jpg"`).

---

## 8. Атрибуция и аналитика — ⬜ Не начато

Предполагаемый набор данных (схема хранения **не утверждена**):

- `brand_id`
- `registration_domain`
- `landing_page`
- `referrer`
- `utm_source`
- `utm_medium`
- `utm_campaign`
- `utm_content`
- `utm_term`

Цели:

- определять источник регистрации;
- разделять аналитику брендов;
- сохранять первоначальный acquisition source;
- не смешивать identity и маркетинговую атрибуцию;
- определить место хранения только после анализа возможностей SHM и требований к отчётности.

---

## 9. Добавление третьего бренда

Целевой onboarding:

1. добавить `deploy/brands/<brand>.json`;
2. подготовить explicit runtime config;
3. назначить домен и TLS;
4. создать systemd unit/drop-in;
5. назначить SHM service category;
6. назначить payment profile;
7. определить Telegram и web identity prefixes;
8. настроить support/assets/content;
9. выполнить config validation;
10. выполнить coordinated rollout;
11. выполнить public smoke;
12. проверить независимость пользователей, услуг и платежей.

Добавление третьего бренда **не должно** требовать:

- копирования бизнес-логики;
- специальных `if brand == ...` по всему проекту;
- ручного редактирования общих VFF-настроек;
- неявных fallback на VFF.

Практическая валидация третьего бренда относится к M9 и зависит от закрытия M5–M8 в критичных путях.

---

## 10. Этапы дальнейшей работы

| Milestone | Статус | Название |
|-----------|--------|----------|
| M1 | ✅ | BrandConfig и строгая валидация |
| M2 | ✅ | runtime/deployment profiles |
| M3 | ✅ | service category isolation |
| M4 | ✅ | Telegram identity isolation |
| M5 | 🟡 | Web identity audit and isolation |
| M6 | ⬜ | Payment end-to-end audit |
| M7 | ⬜ | Brand-specific content cleanup |
| M8 | ⬜ | Attribution and analytics |
| M9 | ⬜ | Third-brand onboarding validation |

### M5 — Web identity audit and isolation

- **Статус:** 🟡 частично — логическая изоляция (membership, brand_id, tokens, handler revalidation, linking) реализована; общий prefix `web_` у VFF/FC сохранён как переходное состояние.
- **Цель:** независимые web identities VFF/FC (email/login/login2/session/OAuth/linking).
- **Основные риски:** коллизии login по email при общем prefix; незавершённый переход FC на `web_fc_`; необходимость production verification.
- **Ожидаемый результат:** brand-aware web login + запись `settings.brand_id`; brand-scoped поиск и linking; allowlist-миграция при необходимости.
- **Критерий завершения:** один email независимо существует в каждом бренде; session/magic link/OAuth/linking не пересекают бренды; услуги другого бренда недоступны.

### M6 — Payment end-to-end audit

- **Цель:** подтвердить, что платёжный контур не пересекает бренды.
- **Основные риски:** callback активирует услугу чужой category; success/fail URL уводят на чужой домен; письма/profile смешивают бренды.
- **Ожидаемый результат:** чеклист e2e по созданию счёта, callback, URL, письмам и активации услуг.
- **Критерий завершения:** платежи и активации строго ограничены активным брендом и его category/profile.

### M7 — Brand-specific content cleanup

- **Цель:** убрать VFF-oriented defaults и выровнять brand content/communications.
- **Основные риски:** скрытые hardcoded URL/тексты/logo/support в bot и web.
- **Ожидаемый результат:** контент и коммуникации берутся из brand/runtime config без VFF fallback в FC.
- **Критерий завершения:** FC UI/email/Telegram не показывают VFF identity; default logo debt закрыт.

### M8 — Attribution and analytics

- **Цель:** разделить acquisition analytics по брендам без смешения с identity.
- **Основные риски:** преждевременная схема хранения в SHM; смешение marketing fields с auth fields.
- **Ожидаемый результат:** утверждённый минимальный набор атрибуции и место хранения после анализа.
- **Критерий завершения:** можно определить источник регистрации по бренду без влияния на login/session.

### M9 — Third-brand onboarding validation

- **Цель:** подтвердить, что третий бренд поднимается конфигурацией и ops-потоком.
- **Основные риски:** скрытые VFF defaults; незакрытые identity/payment gaps из M5–M7.
- **Ожидаемый результат:** тестовый третий бренд проходит validation/deploy/rollout/smoke и изоляцию.
- **Критерий завершения:** onboarding без изменения бизнес-логики и без ручного правления VFF-настроек.

---

## 11. Definition of Done

Мультибрендинг считается завершённым, когда:

- один Telegram ID может независимо существовать в каждом бренде;
- один email может независимо существовать в каждом бренде;
- пользователь не видит услуги другого бренда;
- платежи не пересекают бренды;
- callback не активирует услугу другого бренда;
- публичные ссылки ведут на правильный домен;
- письма и Telegram UI используют правильный бренд;
- добавление тестового третьего бренда не требует изменения бизнес-логики;
- для каждого бренда работают config validation, deploy, rollback и smoke;
- отсутствуют неявные VFF defaults в runtime-критичных путях.

---

## 12. Следующий шаг

**Продолжение M5:** coordinated переход FC prefix `web_` → `web_fc_` (после ручной корректировки известной FC web-привязки и production verification), затем e2e smoke и подтверждение независимой регистрации одного email в двух брендах.

Параллельно / далее по приоритету: **M6 — Payment end-to-end audit**.
