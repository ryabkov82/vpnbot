# M8 — Attribution and analytics architecture audit

## Статус документа

| Поле | Значение |
|------|----------|
| Тип | read-only architecture / data-flow audit |
| Branch | `main` |
| Snapshot commit | `587353875d20a51c185086c0a3947084d2ece568` (`refactor: remove final VFF content fallback`) |
| Дата аудита | 2026-07-26 |
| Production | **не проверялся** (нет SSH, mutation probes, чтения runtime secrets) |
| Код / конфиги / tests / SHM | **не изменялись** |
| Артефакты | этот файл + краткое отражение статуса в `docs/MULTIBRAND_ROADMAP.md` |

Классы утверждений:

- **code-confirmed** — видно в репозитории vpnbot;
- **profile-confirmed** — видно в `deploy/brands/*.json`;
- **inference** — логический вывод из code-confirmed фактов;
- **open** — нельзя доказать без read-only SHM/production probe.

M8 **не закрыт**. Документ рекомендует MVP-архитектуру; implementation и production rollout — отдельные этапы.

Активные бренды:

| ID | Display name | Public host (profile) |
|----|--------------|------------------------|
| `vff` | VPN for Friends | `connect.vpn-for-friends.com` |
| `fc` | Friends Connect | `connect.friends-connect.club` |

---

## 1. Цель аудита

Определить минимальную, реализуемую и безопасную модель **acquisition first-touch attribution**, которая позволяет ответить:

1. к какому бренду относится регистрация;
2. через какой **registration channel** создан пользователь;
3. с какого домена / landing path начался onboarding;
4. какие UTM / referrer / Telegram start payload были у первого визита;
5. когда зафиксирована первоначальная атрибуция;
6. как получить данные для отчётности;
7. как **не** смешать marketing attribution с login / session / identity.

Итог аудита — **одна рекомендуемая MVP-архитектура** (не каталог равноправных вариантов).

---

## 2. Термины и инварианты

### 2.1 Brand identity

Авторизационная принадлежность пользователя к активному процессу:

- `settings.brand_id` (`models.UserSettings.BrandID`);
- canonical Telegram login / web login prefix;
- brand-bound tokens (`brand_id` в claims);
- `brand.service_category`.

**Не является** acquisition attribution. Identity уже закрыта M4–M5.

### 2.2 Registration channel

Механизм **фактического создания** SHM user:

| Значение | Смысл |
|----------|--------|
| `telegram` | `RegisterUser` из Telegram `/register` |
| `web_magic_link` | `FindOrCreateWebUser` после signup-token |
| `web_google` | `FindOrCreateWebUser` после Google OAuth для нового email |

Не путать с рекламным `utm_source`.

### 2.3 Acquisition first-touch

Первоначальный marketing source пользователя **внутри конкретного бренда**.

Инварианты (целевые):

| # | Правило |
|---|---------|
| 1 | Запись только при фактическом создании SHM user |
| 2 | После успешной записи — **immutable** |
| 3 | Повторный вход не перезаписывает |
| 4 | Telegram → web linking **не** создаёт новый first-touch |
| 5 | Google OAuth существующего пользователя **не** меняет first-touch |
| 6 | Данные одного бренда не переносятся в другой |
| 7 | Отсутствие UTM допустимо (не ошибка) |
| 8 | Server-derived поля не принимаются из клиентского JSON как источник истины |

### 2.4 Operational source

Техническое происхождение web-identity в SHM:

- `BrandConfig.WebUserSource` / `brand.web_user_source` (profile);
- `settings.web.source` при web registration (`findOrCreateWebUser`);
- overwrite при linking: `telegram_link`, `telegram_link_google` (`LinkWebEmailForTelegramUser`).

**Почему operational source ≠ acquisition first-touch (code-confirmed):**

1. VFF и FC profiles сейчас имеют **одинаковый** `web_user_source = "vpn-for-friends.com"` (`deploy/brands/vff.json`, `deploy/brands/fc.json`) — нельзя разделить marketing brand origin.
2. `settings.web.source` **перезаписывается** при linking (`internal/service/link_web_email.go`: `webBlock["source"] = source`) — нарушает first-touch immutability.
3. Значения `telegram_link` / `telegram_link_google` описывают **операцию связывания**, а не рекламный канал создания Telegram-пользователя.
4. Nested `settings.web.*` **не используются для lookup** (комментарий в `FindUserByWebEmail`: SHM nested filters → ISE) — поле и так не queryable для analytics.

---

## 3. Карта registration / onboarding flows

| # | Flow | Entry point | Где виден источник | Где создаётся SHM user | Что сохраняется сейчас | Где теряется attribution | Класс |
|---|------|-------------|--------------------|------------------------|------------------------|--------------------------|-------|
| 1 | Telegram direct `/start` | `handleStart` | payload пуст | нет (если user есть → menu; иначе registration menu) | trial eligibility только при allowed payload | нет marketing capture | auth gate / UX |
| 2 | Telegram `/start <payload>` | `handleStart` + `Message.Payload` | payload в памяти процесса | нет на этом шаге | `SetTrialEligible(chatID)` **только** если payload ∈ `AllowedStartParams` | payload **не** пишется в SHM; не передаётся в `/register`; in-memory, теряется при restart | capture opportunity → **create later** |
| 3 | Telegram `/register` | `handleRegister` → `Service.RegisterUser` | Telegram profile fields | **да** — `PUT /shm/v1/admin/user` | `settings.brand_id`, `settings.telegram.*`, canonical login | start payload уже недоступен; нет UTM/domain | **creates user** |
| 4 | Web magic-link **нового** | `POST /api/account/login/start` → email → `POST /api/account/session/start` | Host / `PublicBaseURL` на start; query на `/account` **не** в токене | **да** — `FindOrCreateWebUser` (`created=true`) | `brand_id`, `web.email`, `web.source=web_user_source` | UTM/referrer/landing с GET `/account` не входят в `AccountSignupTokenClaims` | **creates user** |
| 5 | Web magic-link **существующего** | тот же login/start → `CreateAccountToken` | — | **нет** | session token | N/A (не registration) | **auth only** |
| 6 | Google OAuth **нового** | `/api/account/google/start` → callback → `FindOrCreateWebUser` | OAuth state cookie (CSRF only) | **да** при `created=true` | как web user + `web.source=web_user_source` | UTM/landing не в state cookie | **creates user** |
| 7 | Google OAuth **существующего** | callback → find existing | — | **нет** | session redirect | N/A | **auth only** |
| 8 | TG → web link (email) | bot link token → `/account/link` → email confirm → `LinkWebEmailForTelegramUser(..., "telegram_link")` | link tokens: shm_user_id, chat_id, email | **нет** (user уже есть) | `web.email`, **overwrite** `web.source=telegram_link`, backfill `brand_id` | если бы first-touch писали сюда — сломали бы immutability | **link only** |
| 9 | TG → web link (Google) | OAuth + link cookie → `LinkWebEmailForTelegramUser(..., "telegram_link_google")` | link claims + OAuth | **нет** | overwrite `web.source=telegram_link_google` | то же | **link only** |
| 10 | Повторный account/session | session token / cookie UX | — | **нет** | — | — | **auth only** |
| 11 | Public `/buy` | static/template page | marketing page view | **нет** | — | любой UTM на `/buy` не связан с user | lead UX / catalog |
| 12 | `POST /api/public/lead` | `servePublicLeadWithLimiter` | email/contact/service_id; slog + Telegram notify | **нет** | structured log + operator Telegram | lead ≠ SHM user; нет durable attribution record | **lead generation** (exclude from registration analytics) |
| 13 | Admin test/order APIs | `serveAdminWebOrderTest` и др. | admin-only | может вызвать `FindOrCreateWebUser` | test users / orders | не production acquisition | **exclude** / ops-only |

### 3.1 Классификация для M8

**Должны писать first-touch (только если `created` / новый RegisterUser):**

- Telegram `/register`;
- Web magic-link new user;
- Google OAuth new user.

**Не должны писать / перезаписывать first-touch:**

- existing magic-link / OAuth login;
- Telegram → web linking (email/Google);
- session reopen;
- `/buy`, public lead, admin test APIs.

**Public lead:** учитывать только в отдельной lead funnel analytics (вне scope registration first-touch MVP), если понадобится позже.

---

## 4. Инвентаризация текущих данных

### 4.1 Поля и классификация

| Поле / сигнал | Где | Класс | Пригодно для acquisition? |
|---------------|-----|-------|---------------------------|
| `settings.brand_id` | `models.UserSettings` | identity | brand dimension отчётов — **да** (server-derived); не first-touch marketing |
| `settings.telegram.*` | `TelegramInfo` | identity / profile | нет (PII) |
| `settings.web.email` | `WebInfo` | identity metadata | нет (PII) |
| `settings.web.source` | `WebInfo` | operational | **нет** как first-touch (mutable, shared value) |
| `BrandConfig.WebUserSource` | config / profiles | operational default | нет |
| `web_user_source` VFF/FC | `deploy/brands/*.json` | profile | оба = `vpn-for-friends.com` (**profile-confirmed**) |
| Telegram `/start` payload | `handleStart` | transient / trial gate | candidate → `telegram_start_param` |
| `AllowedStartParams` / trial map | `Features.Trial`, `trialEligibleUntil` | operational trial | не marketing store; in-memory |
| `AccountSignupTokenClaims` | `account_token.go` | auth transport | сейчас: `typ, brand_id, email, login, exp` — **нет** UTM |
| `AccountTokenClaims` | session | auth | нет UTM |
| Google OAuth cookies | `vff_google_oauth_state`, `vff_google_oauth_link_token` | CSRF / link transport | state = random; **нет** attribution payload |
| Request Host / `PublicBaseURL` | `publicOrderBaseURL` | server-derived domain candidate | да для `registration_domain` |
| `AllowedHosts` | brand config | identity host allowlist | trust boundary для Host |
| Referer header | HTTP | untrusted client | candidate → `referrer_host` only |
| URL query UTM | browser | untrusted client | candidate |
| Public lead payload | `publicLeadRequestJSON` | lead | exclude from registration MVP |
| `slog` public lead / registration | process logs | transient event | не SoT; rotation |
| vpnbot own DB | — | **отсутствует** (code-confirmed: нет `database/sql` / sqlite / postgres) | N/A |

### 4.2 Доказанные ограничения

1. **Одинаковый `web_user_source` у VFF и FC** — profile-confirmed.
2. **`settings.web.source` мутирует при linking** — code-confirmed (`link_web_email.go`).
3. **Nested settings filters проблемны** — code-confirmed comment on `FindUserByWebEmail`; lookup только по login/login2.
4. **Structured logs ≠ durable SoT** — inference + ops reality; lead пишет `slog.Info("public lead", ...)` без user_id.
5. **Нет собственной DB у vpnbot** — code-confirmed.
6. **Raw settings merge при linking сохраняет неизвестные top-level keys** — code-confirmed (`mergeSettingsJSONToMap` + rewrite only `web` / `brand_id`). Это **благоприятный** сигнал для `settings.attribution`, но persistence unknown nested keys через все SHM code paths — **open** (нужен read-only probe).

---

## 5. Candidate attribution contract (MVP)

### 5.1 Рекомендуемая JSON-модель

Хранение: `settings.attribution` (object), schema version внутри блока.

```json
{
  "attribution": {
    "version": 1,
    "first_touch": {
      "registration_channel": "web_magic_link",
      "registration_domain": "connect.friends-connect.club",
      "landing_path": "/account",
      "referrer_host": "example.com",
      "utm_source": "telegram",
      "utm_medium": "post",
      "utm_campaign": "summer",
      "utm_content": "",
      "utm_term": "",
      "telegram_start_param": "",
      "captured_at": "2026-07-26T18:00:00Z"
    }
  }
}
```

### 5.2 Решения по полям

| Вопрос | Решение MVP | Обоснование |
|--------|-------------|-------------|
| `brand_id` внутри attribution? | **Нет** | Уже есть `settings.brand_id`; дублирование создаёт drift risk |
| domain vs full URL | **`registration_domain`** (host only) | минимизация; scheme/port не нужны для отчётов |
| landing_page vs path | **`landing_path`** нормализованный path (`/account`, `/buy`) | без query string (PII/token risk) |
| referrer | **`referrer_host` only** | полный Referer часто содержит path/query с PII |
| `registration_channel` | **обязателен** (server-derived) | главный разрез отчётов |
| `captured_at` | **обязателен** RFC3339 UTC | auditability / immutability proof |
| `telegram_start_param` | **да**, empty OK | единственный Telegram marketing signal сегодня |
| отдельный `campaign_id` | **нет** в MVP | достаточно `utm_campaign` |
| empty values | UTM/referrer/start_param/landing могут быть `""` | organic / unknown без фиктивных значений |
| max lengths (предложение) | utm_* ≤ 64; landing_path ≤ 128; referrer_host ≤ 253; telegram_start_param ≤ 64; domain ≤ 253 | защита storage/logs |
| charset | printable ASCII для utm/start; path: `/` + unreserved; host: DNS labels | reject control chars, CR/LF |
| timestamp | RFC3339 UTC `Z` | единообразие |
| schema version | **`version: 1` обязателен** | эволюция без breaking reads |

### 5.3 Явно не включать

email, Telegram ID, IP, User-Agent, tokens, OAuth code, magic-link token, произвольный full query string, секреты, payment metadata.

### 5.4 Семантика пустоты

| Состояние | Значение |
|-----------|----------|
| Нет блока `settings.attribution` | **unknown** (legacy / pre-M8 user) |
| Блок есть, UTM пустые | **organic** (регистрация зафиксирована, маркетинг не передан) |
| Поле `""` внутри first_touch | пустое optional поле (не ошибка) |
| Отсутствие блока ≠ VFF | **запрещено** синтезировать VFF |

---

## 6. Нормализация и trust boundaries

### 6.1 Server-derived (только сервер)

- active `brand_id` процесса;
- `registration_channel`;
- `registration_domain` из validated public base / allowed host (не из произвольного client JSON);
- `captured_at = time.Now().UTC()`;
- факт создания нового SHM user (`created` / RegisterUser success).

Клиент **не** может утверждать «я новый пользователь» или подменить brand/domain.

### 6.2 Client-derived, untrusted

UTM fields, landing path, referrer, Telegram start payload.

Обязательная политика:

- `TrimSpace`;
- length limits;
- allowlist символов;
- strip control characters;
- forbid CR/LF;
- no HTML/SQL interpretation;
- no arbitrary nested JSON from client;
- safe structured logging (уже без parse_mode injection в Telegram plain text paths);
- deterministic normalization (lowercase host; path without query/fragment).

UTM **не** security-sensitive и **не** участвует в auth decisions.

---

## 7. Transport attribution across flows

### 7.1 Web magic-link

Цепочка сегодня:

`GET /account?...` → JS/form → `POST /api/account/login/start` → email → `GET /account/session?token=...` → `POST /api/account/session/start` → `FindOrCreateWebUser`.

| Вариант | Плюсы | Минусы | Вердикт |
|---------|-------|--------|---------|
| Расширить **signed signup-token claims** attribution snapshot | переживает открытие письма на другом устройстве; HMAC уже есть | увеличить token size; нужна версия claims | **MVP recommend** |
| HttpOnly cookie на start | просто | **теряется**, если email открыт на другом device | недостаточно alone |
| Hidden form fields only | UX | не переживают email hop | нет |
| Server-side temp store | гибко | нужна DB/redis — новой инфры нет | future / avoid for MVP |

**Рекомендация:** при `login/start` нормализовать client attribution → включить в `AccountSignupTokenClaims` (или соседний signed envelope). При `session/start` + `created=true` записать в SHM. Для existing-user token (`AccountTokenClaims`) attribution **не** включать.

### 7.2 Google OAuth

| Вариант | Вердикт |
|---------|---------|
| Signed OAuth state envelope (state = random \|\| signed attr blob) | **MVP recommend** — проходит redirect |
| Отдельная signed HttpOnly cookie с attr | допустимо как дополнение; cookie name legacy `vff_*` — P2 rename later |
| Server-side temp state | требует store |
| UTM в неподписанном callback query | **запрещено** |

Не класть UTM в plain callback query.

### 7.3 Telegram

Факт: `/start <payload>` происходит **до** `/register`; callback `/register` **не** несёт payload; trial eligibility — **in-memory** `map[int64]time.Time` (`Service.trialEligibleUntil`), restart-volatile.

| Вариант | Durability | Restart | Сложность | Вердикт |
|---------|------------|---------|-----------|---------|
| In-memory pending attribution by chat ID (как trial) | process lifetime | теряется | низкая | **acceptable MVP** с явной потерей при restart |
| Signed data в register callback button | до нажатия | переживает restart процесса, пока message живо | средняя | **предпочтительно**, если telebot Data length позволяет |
| Direct SHM write до user create | N/A | — | нельзя без user | нет |
| Отдельный durable pending store | высокая | высокая | новая инфра | future |

**MVP recommend:** короткий pending map `chatID → {start_param, expires}` + best-effort; ideally также embed normalized start_param в register callback `Data` если лимит Telegram позволяет. При `RegisterUser` записать first_touch `registration_channel=telegram`. Не использовать trial eligibility map как SoT для marketing (семантика trial ≠ attribution).

---

## 8. Варианты постоянного хранения

### 8.1 A. SHM `settings.attribution`

| Аспект | Оценка |
|--------|--------|
| Write path | `RegisterUser` / create settings; linking уже делает raw merge |
| Linking | merge сохраняет unknown keys → attribution должен выжить, **если** код не затирает блок |
| Nested query | вероятно **нельзя** фильтровать (как `settings.web`) — open |
| Export | read-only admin/user fetch + CLI |
| Immutability | enforce в vpnbot: write iff absent |
| Legacy | absent block = unknown |

### 8.2 B. Отдельная DB vpnbot

Новая инфра: migrations, backup, HA, race с SHM user create, user merge/delete sync. **Избыточно для MVP** при отсутствии DB сегодня.

### 8.3 C. Structured event log

Хорошо как **дополнительный** stream; плохо как SoT (rotation, duplication, weak user lookup, PII in logs).

### 8.4 D. Reuse `settings.web.source`

**Не подходит** — см. §2.4.

### 8.5 Decision matrix

| Criterion | SHM settings.attribution | Separate DB | Logs | web.source |
|-----------|--------------------------|-------------|------|------------|
| First-touch durability | High (if immutable writes) | High | Low | Low (overwritten) |
| Queryability | Low–med (export/scan) | High | Low | Low |
| Transactionality with user create | Same SHM request / follow-up update | Race risk | None | Same request |
| Existing infrastructure | **Yes** | No | Yes | Yes |
| Brand isolation | Via settings.brand_id + process | Need FK discipline | Weak | Weak / shared value |
| Migration complexity | Low | High | Low | N/A (wrong model) |
| Reporting complexity | Export CLI | SQL | Log pipeline | Misleading |
| Backup/restore | SHM backup | Extra | Log retention | SHM |

### 8.6 Рекомендация

| Горизонт | Модель |
|----------|--------|
| **MVP (M8)** | **A — `settings.attribution.first_touch`** + vpnbot-enforced immutability + signed transport + read-only export CLI |
| **Future** | C как analytics event stream и/или warehouse export; B только если SHM query limits блокируют reporting |

Storage model **не доказана** end-to-end на production SHM — см. open questions.

---

## 9. SHM capability analysis (из vpnbot)

### 9.1 Используемые API (code-confirmed)

| Операция | Метод | Path / helper |
|----------|-------|----------------|
| Create user | `PUT` | `/shm/v1/admin/user` — `APIClient.RegisterUser` |
| Update settings | `POST` (+ PUT fallback) | `PostAdminUserUpdateSettings` |
| Raw row | GET filter user_id | `FetchAdminUserRowRaw` → `settings` as `json.RawMessage` |
| Lookup | GET filter login / login2 | nested settings filters **не** используются |

### 9.2 Raw merge

`mergeSettingsJSONToMap` сохраняет неизвестные ключи при linking; затем обновляются `web` и `brand_id`. **Inference:** top-level `attribution` должен сохраняться при linking, если implementation не удаляет его.

### 9.3 Open questions (read-only probes)

| # | Open question | Почему важно | Безопасная read-only команда / probe (без secrets в docs) | Ожидаемые исходы | Влияние на решение |
|---|---------------|--------------|----------------------------------------------------------|------------------|--------------------|
| Q1 | Сохраняет ли SHM неизвестный top-level `settings.attribution` после `PUT` create и последующего typed read? | MVP storage | На staging: создать test user с attribution block → `GET admin/user?filter={"user_id":N}` → сравнить raw JSON | keep / drop / coerce | keep → A OK; drop → нужен follow-up POST raw или другая модель |
| Q2 | Сохраняется ли `attribution` после `PostAdminUserUpdateSettings` linking-like merge? | immutability across link | Staging link-sim: update только `web` → raw get | preserved / wiped | wiped → изменить merge strategy |
| Q3 | Работает ли filter по nested `settings.attribution.first_touch.utm_source`? | reporting design | Staging GET с nested filter | 200 / 500 ISE | ISE → только export-scan, не SHM filter reports |
| Q4 | Лимит размера `settings` JSON | max lengths | Docs/SHM source или empirically large payload on staging | limit value | tighten field caps |
| Q5 | Есть ли SHM UI/report для custom settings? | CLI necessity | Read SHM admin UI/docs | yes/no | no → CLI/export обязателен |

**Не выполнять** на production mutation probes. Credentials/PII в отчёт не включать.

---

## 10. Reporting requirements (MVP)

Минимальные отчёты:

1. регистрации по `settings.brand_id`;
2. по `registration_channel`;
3. по `utm_source`;
4. по `utm_campaign`;
5. registrations with **unknown** (нет блока) vs **organic** (блок есть, UTM empty);
6. first-touch одного пользователя по SHM `user_id`;
7. разделение VFF/FC **без** анализа login prefixes (использовать `brand_id`).

Реализация отчётов MVP:

| Средство | Роль |
|----------|------|
| Read-only export CLI (JSON/CSV snapshot) | **да** — primary |
| Прямые SHM nested filters | вероятно нет (Q3) |
| Prometheus | только low-cardinality counters (`brand_id`, `registration_channel`); **запрещены** user-level / utm high-cardinality labels |
| Full BI dashboard | **вне** M8 |

---

## 11. Privacy и security

- data minimization: host/path/utm only;
- не хранить IP в attribution (lead logs уже имеют IP — не копировать в first-touch);
- не полный Referer;
- query strings могут содержать email/tokens — **не** сохранять raw query;
- UTM untrusted — normalize only;
- log injection: structured fields, no unsanitized concat into shell;
- attribution **никогда** не влияет на brand membership / auth / payments;
- не добавлять acquisition в payment provider metadata;
- не отдавать attribution в публичные API responses без отдельной необходимости (кабинет/admin export — отдельно).

---

## 12. Legacy и migration

| Правило | MVP |
|---------|-----|
| Нет attribution | `unknown`, не VFF |
| Не выводить бренд из legacy login prefix, если есть `brand_id` | да |
| Не синтезировать фиктивные UTM | да |
| Не перезаписывать существующий first-touch | да |
| Backfill | **не обязателен** для M8 MVP; допустим только server-derived (`brand_id` уже есть; channel — только если доказуем) |

Рекомендация: **без массового backfill** в MVP; отчёты явно показывают `unknown` cohort.

---

## 13. Рекомендуемая MVP architecture (сводка)

```
[Browser/Telegram]
    | client UTM / start_param (untrusted)
    v
[Normalize + length policy]
    v
[Signed transport]
    web: signup-token claims
    google: signed state envelope
    telegram: pending map ± callback data
    v
[User create gate]
    RegisterUser / FindOrCreateWebUser(created=true)
    v
[Write once] settings.attribution.version=1.first_touch
    server: channel, domain, captured_at
    client: utm_*, landing_path, referrer_host, telegram_start_param
    v
[Immutable]
    link/login/oauth existing → no write
    v
[Export CLI] read-only snapshots for reports
```

---

## 14. Implementation plan (future commits — не сейчас)

| # | Commit theme | Scope | Primary files (expected) | Depends | Config | Migration | Rollout | Rollback | Tests | Prod verify |
|---|--------------|-------|--------------------------|---------|--------|-----------|---------|----------|-------|-------------|
| 1 | domain model + normalization | types, validate/normalize helpers | `internal/attribution/*.go` (new) | — | optional max lens constants | none | n/a | revert | unit normalize matrix | n/a |
| 2 | SHM persistence + immutable write | read/write attribution block; refuse overwrite | `internal/service/*`, `internal/models` | 1 | none | none (additive JSON) | after tests | stop writes | create/link/idempotency | staging Q1–Q2 |
| 3 | web magic-link capture | claims + login/start + session/start | `account_token.go`, `account_web.go`, account HTML/JS | 1–2 | none | token claim additive | VFF then FC | disable claim write | token roundtrip; other-device email | create web user + export |
| 4 | Google OAuth capture | signed state envelope | `google_oauth.go` | 1–2 | none | none | VFF/FC | disable | new vs existing user | OAuth new user |
| 5 | Telegram `/start` capture | pending ± callback | `internal/app/bot/service.go`, `service.go` | 1–2 | none | none | VFF/FC | disable | start→register; restart loss documented | TG register + export |
| 6 | structured registration event | slog/metric low-cardinality | service/web/bot | 2 | none | none | both | disable log field | — | log sample |
| 7 | export CLI | read-only JSON/CSV | `cmd/` or `scripts/` | 2 | admin API creds via existing config | none | ops | n/a | golden | snapshot both brands |
| 8 | tests + roadmap closure | DoD evidence | tests + roadmap | 1–7 | — | — | both runtimes | — | full suite | smoke + spot export |

Порядок rollout: staging SHM probes Q1–Q2 → VFF → FC. Rollback: feature flag / skip attribution write; auth paths без изменений.

---

## 15. Proposed Definition of Done for M8

M8 считается ✅ только когда:

1. new Telegram registration получает server-derived active brand + first_touch (`registration_channel=telegram`);
2. new web magic-link registration получает domain + channel + optional UTM first_touch;
3. new Google registration — аналогично (`web_google`);
4. repeat login не меняет first_touch;
5. Telegram → web linking не меняет first_touch (и не требует UTM);
6. VFF и FC attribution не пересекаются (process isolation + `brand_id`);
7. отсутствие UTM → organic (блок есть) vs unknown (блока нет) — семантика зафиксирована;
8. существующий пользователь не получает фиктивный first_touch;
9. данные доступны read-only export для отчётности;
10. production rollout обоих runtime проверен;
11. auth/session/payment behavior не изменены;
12. no cross-brand fallback.

**Недостаточно** для закрытия M8: только browser UTM capture без durable immutable persistence.

---

## 16. Evidence index (symbols / paths)

| Topic | Path / symbol |
|-------|----------------|
| User settings model | `internal/models/models.go` — `UserSettings`, `WebInfo`, `TelegramInfo` |
| Web create | `internal/service/web_user.go` — `findOrCreateWebUser`, `FindOrCreateWebUser` |
| Telegram create | `internal/app/bot/service.go` — `handleStart`, `handleRegister`; `internal/service/service.go` — `RegisterUser` |
| Linking + source overwrite | `internal/service/link_web_email.go` — `LinkWebEmailForTelegramUser` |
| Raw merge | `mergeSettingsJSONToMap`, `FetchAdminUserRowRaw`, `PostAdminUserUpdateSettings` |
| Signup claims | `internal/app/web/account_token.go` — `AccountSignupTokenClaims` |
| Magic-link flow | `internal/app/web/account_web.go` — `serveAccountLoginStart`, `serveAccountSessionStart` |
| Google OAuth cookies | `internal/app/web/google_oauth.go` — `googleOAuthCookieName` |
| Public lead (no user) | `internal/app/web/public_lead.go` — `servePublicLeadWithLimiter` |
| Trial in-memory | `internal/service/service.go` — `trialEligibleUntil` |
| Domain helper | `internal/app/web/public_buy_urls.go` — `publicOrderBaseURL` |
| Profiles | `deploy/brands/vff.json`, `deploy/brands/fc.json` — `web_user_source` |
| Nested filter warning | `FindUserByWebEmail` comment in `web_user.go` |

---

## 17. Out of scope / non-goals

- реализация кода в этом коммите;
- изменение M5–M7 historical audits;
- Prometheus user-level series;
- полноценный BI;
- изменение `web_user_source` profile values (отдельное ops-решение; не блокирует attribution model);
- rename `vff_*` cookies/globals (P2 debt из M7).
