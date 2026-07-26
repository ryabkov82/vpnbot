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
| Public host | `connect.vpn-for-friends.com` | `connect.friends-connect.club` |
| Landing | `vpn-for-friends.com` | `friends-connect.club` |
| SHM category | `vpn-mz-test` | `vpn-mz-fc` |
| Payment profile | `telegram_bot` | `telegram_friends_connect_bot` |
| Telegram login | `@<chat_id>` | `@fc_<chat_id>` |

Публичный домен FC:

- production FC runtime принимает только `connect.friends-connect.club`;
- старый домен `connect-fc.vpn-for-friends.com` больше не используется как runtime host;
- старый домен оставлен только как HTTP 301 redirect на `connect.friends-connect.club` с сохранением request URI;
- для нового домена настроены отдельные DNS, nginx vhost и TLS.

Дополнительно (из brand profiles / production config):

| Параметр | VFF | Friends Connect |
|----------|-----|-----------------|
| `web_user_login_prefix` | `web_` | `web_fc_` |
| `web_user_source` | `vpn-for-friends.com` | `vpn-for-friends.com` |

Web identities VFF и FC физически разделены разными login prefix (см. §5 / M5 — ✅).

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

## 5. Независимые web identities — ✅ Готово

Production cutover Friends Connect завершён: web identities VFF и FC разделены физически разными login prefix и логически brand membership / brand-bound tokens.

Аудит lifecycle: `docs/MULTIBRAND_WEB_IDENTITY_AUDIT.md` (исторический snapshot).

### Итоговая production-модель web login

- VFF: `web_<hash(email)>`
- Friends Connect: `web_fc_<hash(email)>`
- будущие бренды: `web_<brand_id>_<hash(email)>`

| Поле | VFF | FC |
|------|-----|----|
| `web_user_login_prefix` | `web_` | `web_fc_` |

Один normalized email теперь имеет разные canonical login keys в VFF и FC, поэтому может существовать независимо в обоих брендах.

Конкретные `user_id`, email и hash в roadmap не включаются.

### Завершено

- разные physical login prefixes VFF и FC;
- `settings.brand_id` при web registration;
- web membership validation (`internal/service/web_brand_user.go`);
- brand-bound account/signup/link tokens (`brand_id` в claims);
- fail-closed проверка токена без подходящего `brand_id` (без dual-read);
- повторная brand validation в account handlers (`ValidateWebAccountUser` + `authenticateWebAccount`);
- brand-aware Telegram ↔ web linking;
- отсутствие FC fallback на VFF web identity (`ErrUserIdentityMismatch`);
- migration существующей FC web-привязки `web_<hash(email)>` → `web_fc_<hash(email)>`;
- production rollout FC с новым prefix (`web_fc_`);
- production verification и public smoke.

### Критерии завершения M5 — выполнены

- VFF и FC используют независимые web identity keyspaces;
- session, magic link, OAuth и linking ограничены активным брендом;
- пользователь одного бренда не получает identity другого бренда;
- услуги другого бренда недоступны;
- существующая FC web-привязка мигрирована;
- production rollout и smoke успешны.

---

## 6. Платежи — ✅ Готово

Поддерживаемый vpnbot payment flow изолирован по брендам: разные SHM identities, category gate, Telegram payment profiles и brand-aware YooKassa return routing. Provider callback, зачисление баланса, activation и idempotency выполняются внутри SHM; в vpnbot payment callback handler отсутствует.

Исторический статический snapshot lifecycle до последующих исправлений и production verification: `docs/MULTIBRAND_PAYMENT_AUDIT.md`. Операционные детали overlays: `deploy/shm/yookassa/README.md`, `deploy/shm/templates/tg_payments_webapp/README.md`.

### Архитектурная модель

- vpnbot инициирует заказ услуги и пополнение баланса;
- дальнейший provider/SHM flow (callback, balance credit, activation, idempotency) — вне vpnbot;
- payment metadata в текущем YooKassa CGI содержит `user_id`, но не brand/service identity;
- post-payment email/Telegram notification в vpnbot отсутствуют;
- YooKassa merchant/pay-system config в SHM остаётся общим; отдельная merchant/callback инфраструктура на бренд не утверждается.

### Brand isolation до платежа

- VFF и FC используют разные SHM users (Telegram/web identity isolation);
- категории: VFF `vpn-mz-test`, FC `vpn-mz-fc`; создание/изменение услуги проходит category gate активного бренда;
- Telegram payment profiles: VFF `telegram_bot`, FC `telegram_friends_connect_bot`.

### Brand-aware YooKassa return routing

- обязательный fail-closed `brand.yookassa_pay_system`;
- vpnbot передаёт в CGI только серверные `ps=yookassa` и `brand_id=vff|fc`; клиент не задаёт `brand_id` / `return_url`;
- CryptoCloud не изменён и не получает `brand_id`;
- managed CGI overlay `VPNBOT_BRAND_ROUTING_VERSION=2`: mapping из `deploy/brands/*.json` → публичные `/payment/return` доменов брендов;
- неизвестный непустой `brand_id` отклоняется; отсутствие `brand_id` сохраняет legacy SHM `return_url`;
- безопасная диагностика: `vpnbot_route_check` (без создания платежа).

### Telegram routing

- managed SHM template `tg_payments_webapp` (`VPNBOT_TG_PAYMENT_ROUTING_VERSION=1`);
- launch URL содержит серверные `brand_id` и `yookassa_ps`;
- шаблон строит `shm_url + amount`, затем через `URL.searchParams` добавляет `brand_id` только при совпадении фактического `ps` с `yookassa_ps`;
- CryptoCloud и legacy launch без `brand_id` сохраняют прежнее поведение;
- email кодируется через `searchParams`.

### Managed deployment

Централизованные `check` / `diff` / `deploy` / `rollback` для CGI и Telegram template: backup, валидация candidate, atomic/POST install, probes, автоматический rollback при ошибке. Без ручных one-off правок production.

### Production verification

Подтверждено безопасными probes и public smoke (без реального успешного платежа и без проверки provider callback / post-payment писем):

- route-check VFF/FC → точные brand return URL; invalid/absent → controlled `unknown brand_id`;
- create probes с неизвестным user → controlled rejection; invalid create brand → `unknown brand_id`;
- post-deploy check/diff идемпотентны;
- web и Telegram YooKassa вручную открываются для VFF и FC; CryptoCloud продолжает открываться;
- admin template совпадает с candidate; public template endpoint HTTP 200.

### Границы ответственности SHM

Callback, idempotency, зачисление и активация услуги — контур SHM/provider. Brand isolation на стороне vpnbot обеспечивается identities, category gate, payment profile и brand-aware return routing, а не отдельными YooKassa merchant credentials на бренд.

### Критерии завершения M6 — выполнены

- платежи поддерживаемого vpnbot flow не пересекают бренды на уровне identity/category/profile/return routing;
- публичные payment return URL ведут на домен активного бренда;
- callbacks остаются внутри SHM и привязаны к отдельному SHM user;
- CryptoCloud и legacy без `brand_id` не сломаны;
- overlays управляются из репозитория с безопасной verification.

---

## 7. Контент и коммуникации — ✅ Готово

### Итог M7

P0/P1 runtime findings из content audit закрыты и развёрнуты:

- account/buy/payment/link titles и notices строятся из `brand.name` / `brand.landing_url`;
- payment support и public buy support — через `WebCabinetResolvedSupportURL`;
- Telegram logo fail-open на VFF удалён (`assets.logo_url` обязателен);
- premium-connect support/redirect изолированы (same-origin `/redirect.html`);
- operator lead notifications brand-aware (`EffectiveBrand().Name`);
- brand-specific favicon asset sets (VFF/FC);
- последний email fallback `"VPN for Friends"` удалён (fail-closed `brand.name`).

Исторический snapshot аудита до remediation: `docs/MULTIBRAND_CONTENT_AUDIT.md` (не изменяется).

### Content contract

**Required / fail-closed**

- `brand.id`, `brand.name`, `brand.allowed_hosts`, `brand.public_base_url`, `brand.landing_url`;
- `brand.service_category`, `brand.web_user_login_prefix`, `brand.web_user_source`;
- `brand.payment_profile`, `brand.yookassa_pay_system`;
- `assets.logo_url`;
- favicon asset set для поддерживаемого `brand.id`;
- user-visible titles и operator labels — из `brand.name`;
- public/marketing links — из active brand URLs;
- cross-brand fallback запрещён.

**Optional or intentionally shared**

- `telegram.news_channel` — optional;
- Telegram support contact может быть общим для брендов;
- текущий общий support: `https://t.me/friends_connect_support`;
- generic RU/EN UX copy может быть общей;
- dark Bootstrap theme может быть общей;
- общий SHM/backend/payment merchant не является content identity;
- отдельная visual theme на бренд сейчас не требуется.

**Favicon model**

- VFF и FC используют отдельные embedded asset sets;
- неизвестный `brand.id` отклоняется fail-closed;
- добавление asset pack третьего бренда проверяется в M9.

### Accepted P2 debt

Внутренние идентификаторы (не блокируют M7):

- `window.VFF_ACCOUNT`;
- `window.VFF_I18N`;
- cookie `vff_lang`.

Они не отображаются пользователю, не влияют на brand routing/identity, сохранены ради
минимизации churn и совместимости cookie; могут быть переименованы отдельным refactor
после M9.

Также приняты: shared visual theme; VFF strings в test fixtures и исторических docs.

### Production verification

- VFF и FC binary deploy прошёл;
- explicit brand startup verified;
- public smoke прошёл;
- UI titles/landing/support/logo/favicon проверены;
- favicon production hashes разделены; VFF assets не изменились;
- FC не получает VFF identity/defaults;
- lead operator flow сейчас не используется UI, но builder brand-aware и покрыт tests.

### Прочее (ранее реализовано)

- explicit `email.from_name` имеет приоритет для From header;
- FC Google OAuth Web client и callback на `connect.friends-connect.club`;
- `support@friends-connect.club` (Cloudflare Email Routing / SMTP2GO).

---

## 8. Атрибуция и аналитика — 🟡 Частично

### Статус

Architecture / data-flow audit **completed**; implementation и production rollout **не начаты**.

Артефакт аудита: [`docs/MULTIBRAND_ATTRIBUTION_AUDIT.md`](MULTIBRAND_ATTRIBUTION_AUDIT.md)
(snapshot `587353875d20a51c185086c0a3947084d2ece568`).

### Краткий вывод аудита (не замена audit doc)

- Acquisition **first-touch** отделён от brand identity и от operational `settings.web.source`.
- `settings.web.source` / `web_user_source` **недостаточны** (shared VFF/FC value; overwrite при linking).
- MVP storage recommendation: immutable `settings.attribution.first_touch` в SHM + signed transport
  (signup-token / OAuth state) + read-only export CLI.
- Public lead и admin test APIs исключены из registration first-touch analytics.
- SHM nested filter/query capability и полная persistence unknown settings — **open**, требуют
  staging read-only probes до закрытия implementation.

Цели M8 (без изменения):

- определять источник регистрации;
- разделять аналитику брендов;
- сохранять первоначальный acquisition source;
- не смешивать identity и маркетинговую атрибуцию.

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
| M5 | ✅ | Web identity audit and isolation |
| M6 | ✅ | Payment end-to-end audit and routing isolation |
| M7 | ✅ | Brand-specific content cleanup |
| M8 | 🟡 | Attribution and analytics |
| M9 | ⬜ | Third-brand onboarding validation |

### M5 — Web identity audit and isolation

- **Статус:** ✅ готово — production cutover FC на `web_fc_` выполнен; web identities VFF/FC физически и логически изолированы.
- **Цель:** независимые web identities VFF/FC (email/login/login2/session/OAuth/linking).
- **Результат:** VFF `web_<hash(email)>`, FC `web_fc_<hash(email)>`; brand-bound tokens и membership; migration существующей FC web-привязки; production rollout и smoke успешны.
- **Критерий завершения:** выполнен — независимые keyspaces; session/magic link/OAuth/linking ограничены брендом; услуги другого бренда недоступны.

### M6 — Payment end-to-end audit and routing isolation

- **Статус:** ✅ готово.
- **Цель:** выполнена — подтверждена изоляция поддерживаемого vpnbot payment flow и brand-aware YooKassa return routing без пересечения брендов.
- **Результат:** статический аудит (`docs/MULTIBRAND_PAYMENT_AUDIT.md` как исторический snapshot); managed YooKassa CGI `VPNBOT_BRAND_ROUTING_VERSION=2` и Telegram template `VPNBOT_TG_PAYMENT_ROUTING_VERSION=1`; серверные `brand_id` / `yookassa_ps`; CryptoCloud без `brand_id`; централизованные check/diff/deploy/rollback.
- **Production verification:** безопасные route-check/create probes, public smoke, идемпотентные check/diff; ручное открытие YooKassa в web и Telegram для VFF/FC.
- **Граница:** callback/idempotency/activation выполняются SHM; реальный успешный платёж и replay callback в рамках закрытия M6 не проводились; общая YooKassa merchant config в SHM сохраняется.
- **Критерий завершения:** выполнен для поддерживаемого vpnbot payment flow — brand isolation identities/category/profile/return routing; публичные return URL на домен активного бренда; callbacks остаются в SHM у отдельного SHM user.

### M7 — Brand-specific content cleanup

- **Статус:** ✅ готово.
- **Цель:** выполнена — убраны VFF-oriented runtime defaults; content/communications выровнены по active brand.
- **Результат:** P0/P1 findings закрыты (account/buy/link identity, payment support, logo fail-open, premium-connect URLs, operator lead banner, brand favicons, email fail-closed); зафиксирован content contract; P2 JS/cookie names приняты как non-blocking debt.
- **Критерий завершения:** выполнен — FC UI/email/Telegram не показывают VFF identity; cross-brand content fallback в runtime-критичных путях отсутствует.

### M8 — Attribution and analytics

- **Статус:** 🟡 частично — architecture/data-flow audit completed (`docs/MULTIBRAND_ATTRIBUTION_AUDIT.md`); implementation not started.
- **Цель:** разделить acquisition analytics по брендам без смешения с identity.
- **Основные риски:** преждевременная схема хранения в SHM; смешение marketing fields с auth fields; in-memory Telegram start payload; email hop без signed claims.
- **Ожидаемый результат аудита:** рекомендуемый MVP — immutable `settings.attribution.first_touch` + signed web/Google transport + export CLI; `web.source` не использовать как first-touch.
- **Критерий завершения (implementation):** new registrations получают first-touch; repeat login/linking не перезаписывают; VFF/FC изолированы; read-only reporting; auth/session/payment без изменений.

### M9 — Third-brand onboarding validation

- **Цель:** подтвердить, что третий бренд поднимается конфигурацией и ops-потоком.
- **Основные риски:** скрытые VFF defaults; незакрытые identity/payment gaps из M5–M7.
- **Ожидаемый результат:** тестовый третий бренд проходит validation/deploy/rollout/smoke и изоляцию.
- **Критерий завершения:** onboarding без изменения бизнес-логики и без ручного правления VFF-настроек.

---

## 11. Definition of Done

Мультибрендинг считается завершённым, когда:

- один Telegram ID может независимо существовать в каждом бренде;
- один email может независимо существовать в каждом бренде
  (этот пункт обеспечен завершённой web identity isolation M5);
- пользователь не видит услуги другого бренда;
- платежи не пересекают бренды;
- публичные payment return URL ведут на домен активного бренда;
- callbacks остаются внутри SHM и привязаны к отдельному SHM user;
- публичные ссылки ведут на правильный домен;
- письма и Telegram UI используют правильный бренд;
- добавление тестового третьего бренда не требует изменения бизнес-логики;
- для каждого бренда работают config validation, deploy, rollback и smoke;
- отсутствуют неявные VFF defaults в runtime-критичных путях.

M5–M7 закрыты; общий мультибрендинг ещё не завершён — остаются M8–M9.

---

## 12. Следующий шаг

Основной следующий шаг: **M8 implementation according to the approved attribution storage model**
в [`docs/MULTIBRAND_ATTRIBUTION_AUDIT.md`](MULTIBRAND_ATTRIBUTION_AUDIT.md)
(после staging read-only SHM probes из раздела open questions).

Затем:

- **M9 — Third-brand onboarding validation**.
