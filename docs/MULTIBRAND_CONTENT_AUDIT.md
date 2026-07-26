# M7 — Brand-specific content audit

## Статус документа

| Поле | Значение |
|------|----------|
| Тип | read-only аудит кода репозитория |
| Branch | `main` |
| Snapshot commit | `864d53922195197eb98966677b0409dd37e4f654` (`docs: close multibrand payment milestone`) |
| Дата аудита | 2026-07-22 |
| Production | **не проверялся** (нет SSH, API, deploy, чтения runtime configs/secrets) |
| Код / конфиги / tests | **не изменялись** |
| Единственный артефакт | этот файл |

Классы findings:

- **code-confirmed** — поведение видно в коде и активно на runtime path обоих брендов (или доказано fail-open);
- **reachability-dependent** — поведение зависит от того, открыт ли конкретный route / заполнен ли optional config;
- **production-config-dependent** — итог зависит от explicit runtime config на сервере (не читался);
- **test/docs-only** — совпадение в тестах, docs, fixtures; не production leak.

M7 **не закрыт**. Документ фиксирует gaps для последующих независимых коммитов.

Активные бренды (из `deploy/brands/*.json` и roadmap):

| ID | Display name |
|----|--------------|
| `vff` | VPN for Friends |
| `fc` | Friends Connect |

---

## 1. Цель и критерии

Цель: найти runtime-контент и defaults, из‑за которых процесс Friends Connect может:

- показывать identity VPN for Friends;
- использовать VFF logo/favicon/title/domain;
- вести в VFF support/news;
- отправлять VFF-oriented email/Telegram-тексты;
- использовать неявный VFF fallback;
- смешивать контакты разных брендов.

Критерии закрытия M7 — см. §15. Слово `VPN` само по себе **не** считается brand leak.

---

## 2. Executive summary

| Priority | Count |
|----------|------:|
| **P0** — cross-brand leak | **8** |
| **P1** — обязательная brand-specific доработка | **5** |
| **P2** — maintainability / consistency debt | **4** |

**Главные cross-brand risks (code-confirmed):**

1. Web account i18n (`account_i18n.go`) жёстко содержит «VPN for Friends» в titles/H1/footer для **обоих** runtime.
2. Footer/marketing links ведут на `vpn-for-friends.com`, игнорируя `brand.landing_url`.
3. Блок оплаты смешивает Telegram `@friends_connect_support` и email `support@vpn-for-friends.com` на **обоих** брендах.
4. Публичная `/buy` отдаёт VFF title/H1 и FC support URL из одного static HTML.
5. Страницы Telegram→web linking содержат hardcoded «VPN for Friends» в `<title>`.
6. Telegram bot: пустой `assets.logo_url` → fail-open на `https://vpn-for-friends.com/logobot.jpg`.
7. `/premium-connect` активен в общем mux: support = FC Telegram; `redirectBase` = VFF domain.
8. Operator lead notification всегда помечает заявку как «с сайта VPN for Friends» (не клиентский UI, но смешивает бренды в ops).

**Уже brand-aware:**

- `BrandConfig` (id/name/hosts/public_base_url/landing_url/category/payment/…) с fail-closed `Normalize()`;
- email subject/body/From display name из `EffectiveBrand().Name` (при валидном config);
- `/payment/return` title/H1 из `brand.name`;
- Telegram support/news кнопки бота из `telegram.support_chat` / `telegram.news_channel`;
- web session кнопка «Поддержка» из resolved `telegram.support_chat` (+ env override);
- public URLs писем/account link — через `PublicBaseURL()` / host allowlist модели (не предмет content copy).

**Где content всё ещё VFF-oriented:** account i18n, buy page, link pages, default Telegram logo, premium-connect redirect helper, shared favicon embeds, operator lead label.

**Требуют production config verification:** фактические `assets.logo_url`, `telegram.support_chat` / `news_channel`, `email.from_name` / `from_email`, `leads_chat_id` / `support_chat_id`, наличие ли FC logo/favicon отличаются от VFF.

---

## 3. Текущая content architecture

### BrandConfig (`internal/config/brand.go`)

Явный контракт процесса: `id`, `name`, `allowed_hosts`, `public_base_url`, `landing_url`, `service_category`, web login/source, payment profile, yookassa ps.

- `brand.name` **required** при `Normalize()` → пустое имя отклоняет старт.
- `brand.landing_url` **required** и валидируется как absolute HTTP(S) URL.
- **Не используется** account/web UI copy (footer SiteURL берётся из hardcoded констант, не из `LandingURL`).

### Assets (`config.Assets.LogoURL`)

Опциональный `assets.logo_url`. Нет fail-closed validation. Bot fallback → VFF URL (§5, M7-CONTENT-007).

### Telegram config

| Field | Usage |
|-------|--------|
| `telegram.support_chat` | Bot URL-кнопки «Поддержка»; web session support button (через `WebCabinetResolvedSupportURL`) |
| `telegram.news_channel` | Bot URL-кнопка «Новости» (если не пусто) |
| `telegram.support_chat_id` / `leads_chat_id` | Operator notifications (chat destination, не user-visible copy) |
| env `TELEGRAM_SUPPORT_URL` / `SUPPORT_TELEGRAM_URL` | Override только для web cabinet support href |

Bot **не** подставляет brand name в menu copy (`commands.go` — generic RU descriptions).

### Email config

| Field | Usage |
|-------|--------|
| `email.from_email` | Envelope/From address (required for `IsConfigured`) |
| `email.from_name` | Optional; иначе `brandDisplayName` → `EffectiveBrand().Name` |
| `legacyBrandDisplayName` | Fallback только если `Name == ""` / nil cfg — **не** valid production path |

### i18n

`internal/app/web/account_i18n.go` — единственный крупный RU/EN словарь кабинета. Identity и marketing URLs **зашиты**, не читают `BrandConfig`.

### Embedded static assets

| Asset | Embed | Routes |
|-------|-------|--------|
| `static/favicon.ico` (3223 B, sha256 `8e4d6e63…`) | `favicon.go` | `/favicon.ico` |
| `static/favicon-32x32.png` (2410 B, sha256 `c255b9d2…`) | `favicon.go` | `/favicon-32x32.png` |
| `static/apple-touch-icon.png` (41116 B, sha256 `95763a33…`) | `favicon.go` | `/apple-touch-icon.png` |
| `static/account/*.html` | `account_pages.go` / handlers | `/account`, `/account/session`, `/account/link*` |
| `static/buy/index.html` | `buy_page.go` | `/buy` |
| `static/payment/return.html` | `payment_return.go` | `/payment/return` |
| `static/premium-connect/index.html` | `server.go` | `/premium-connect`, `/premium-connect-test` |

Cache: favicon routes — `Cache-Control: public, max-age=604800` (неделя). HTML account/buy/return — `no-store` где задано.

### Deployment profiles (`deploy/brands/{vff,fc}.json`)

Содержат identity/routing (name, hosts, public_base_url, landing_url, category, payment, web login/source).
**Не содержат** content/assets/support/email fields — это **не ошибка само по себе**: часть данных живёт в explicit runtime config (`config-vff.json` / `config-fc.json`, не в репозитории). Gap: нет единого content contract между profile и runtime UI.

---

## 4. Runtime surface map

| Surface | Route/trigger | Source | VFF | FC | Reachability |
|---------|---------------|--------|-----|----|--------------|
| Telegram bot UI | `/start`, menus, lists, help, pays | `internal/app/bot/service.go`, `commands.go` | yes | yes | active-Telegram |
| Telegram logo photo | many handlers via `logoPhoto` | `assets.logo_url` or `defaultLogoURL` | yes | yes | active-Telegram; config-dependent |
| Telegram support/news buttons | inline menus | `telegram.support_chat` / `news_channel` | yes | yes | config-dependent |
| Account login | `GET /account` | `account_pages.go` + `static/account/index.html` + i18n | yes | yes | active-public |
| Account session | `GET /account/session` | `session.html` + i18n | yes | yes | active-authenticated |
| Account link start/invalid/conflict | `GET /account/link` | embed HTML + `standaloneLinkNoticePage` | yes | yes | active-public (token flow) |
| Buy / lead catalog | `GET /buy` | `static/buy/index.html` | yes | yes | active-public |
| Payment return | `GET /payment/return` | template + `brand.name` | yes | yes | active-public |
| Premium connect | `GET /premium-connect*` | embedded HTML | yes | yes | active-public (token UX) |
| Favicon / apple-touch | `/favicon*`, `/apple-touch-icon.png` | `favicon.go` embeds | yes | yes | active-public |
| Email magic-link / link-confirm | SMTP send | `internal/email/sender.go` | yes | yes | config-dependent |
| Lead operator notify | `POST /api/public/lead` → notifier | `telegram_notifier.go` | yes | yes | active-operator |
| Web user registered notify | signup path | `telegram_account_notifier.go` | yes | yes | active-operator |
| Admin test APIs | `/api/admin/*` | handlers | yes | yes | active-operator (token) |
| brand profiles | deploy tooling | `deploy/brands/*.json` | yes | yes | config-dependent (ops) |
| Tests / docs with VFF examples | `*_test.go`, `docs/**`, README | fixtures | n/a | n/a | test/docs-only |

Нет отдельного `configs/` каталога в репозитории.

---

## 5. Findings

### P0

#### Summary table

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-001 | P0 | Web account | yes | yes | `account_i18n.go` | Shows «VPN for Friends» (correct identity, wrong source) | Shows «VPN for Friends» | FC UI identity leak | Render titles/H1/footer from `brand.name` |
| M7-CONTENT-002 | P0 | Web account footer | yes | yes | `accountMarketingSiteURL*` | Links to VFF landing | Links to VFF landing | FC → VFF domain | Use `brand.landing_url` (+ locale path policy) |
| M7-CONTENT-003 | P0 | Web payment modal | yes | yes | `PaymentMethodSupport` | FC Telegram + VFF email | Same mixed pair | Bidirectional contact mix | Brand support URL + brand support email from config |
| M7-CONTENT-004 | P0 | Public buy | yes | yes | `static/buy/index.html` | VFF title/H1 + FC support | Same | FC identity + mixed support | Templated brand name + brand support |
| M7-CONTENT-005 | P0 | Account link pages | yes | yes | link `*.html` + conflict embed | VFF in `<title>` | VFF in `<title>` | FC identity leak | Brand name in titles |
| M7-CONTENT-006 | P0 | Account link notices | yes | yes | `standaloneLinkNoticePage` | Title suffix «VPN for Friends» | Same | FC identity leak | `brand.name` suffix |
| M7-CONTENT-007 | P0 | Telegram logo | yes | if `logo_url` empty | `defaultLogoURL` | VFF logo URL | VFF logo URL | FC shows VFF asset | Require `assets.logo_url` or brand asset; **no** VFF fallback |
| M7-CONTENT-008 | P0 | Premium connect | yes | yes (both brands) | `premium-connect/index.html` | FC support + VFF redirect helper | FC support + VFF redirect | VFF users→FC support; FC users→VFF domain | Brand support + brand redirect base |

---

#### M7-CONTENT-001 — Account i18n identity hardcoded to VFF

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-001 | P0 | Web account | yes | active-public / authenticated | `internal/app/web/account_i18n.go` | VFF copy | VFF copy | FC shows wrong brand | `brand.name`-driven i18n |

**Доказательство**

- Path: `internal/app/web/account_i18n.go`
- Symbols: `accountI18nRU`, `accountI18nEN` — поля `PageTitleLogin`, `PageTitleSession`, `FooterBrand`, `LoginH1`
- Lines (RU): ~258–268; (EN): ~346–356 — литералы `"… VPN for Friends"` / `"VPN for Friends account"`
- Call path: `loadAccountI18n` → `serveAccount` / `serveAccountSession` → templates `static/account/index.html` (`{{.I18n.PageTitleLogin}}`, `{{.I18n.FooterBrand}}`), `session.html`
- Active for **both** brands: один binary, один i18n dictionary, нет ветвления по `BrandID()`
- Не false positive: строки попадают в HTML `<title>` и footer, видимы в browser chrome

**Ожидание:** titles/H1/footer = active `brand.name` (и согласованные локализованные шаблоны).

---

#### M7-CONTENT-002 — Marketing / footer SiteURL hardcoded to VFF

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-002 | P0 | Web footer link | yes | yes | `accountMarketingSiteURLRU/EN` | VFF landing | VFF landing | FC → VFF | `brand.landing_url` |

**Доказательство**

- Constants: `accountMarketingSiteURLRU = "https://vpn-for-friends.com/"`, `…EN = "https://vpn-for-friends.com/en/"` (`account_i18n.go` ~21–22, 124–128)
- Wired in `account_pages.go` (`SiteURL: accountMarketingSiteURL(locale)`)
- Templates: footer `<a href="{{.SiteURL}}">{{.I18n.FooterBrand}}</a>`
- `BrandConfig.LandingURL` **существует и обязателен**, но **не читается** UI (grep: usage только config validation / tests / deploy profiles)
- FC profile declares `landing_url: https://friends-connect.club`, но кабинет всё равно ссылает на VFF

---

#### M7-CONTENT-003 — Mixed payment support contacts (FC Telegram + VFF email)

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-003 | P0 | Top-up payment methods | yes | authenticated session | `PaymentMethodSupport` | Mixed contacts | Mixed contacts | Cross-brand support | Per-brand support URL + email |

**Доказательство**

- `account_i18n.go` ~338 (RU), ~427 (EN):
  `t.me/friends_connect_support` **и** `mailto:support@vpn-for-friends.com`
- Injected via `buildAccountTopUpPaymentMethodsHTML` → `i.PaymentMethodSupport` (raw HTML)
- Session header support button (`buildAccountSessionSupportLinkHTML`) **отдельно** берёт `telegram.support_chat` — корректная модель; payment modal её **не** использует
- Не false positive: пользователь видит оба контакта в UI оплаты

---

#### M7-CONTENT-004 — Public `/buy` page VFF identity + FC support

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-004 | P0 | Public buy | yes | active-public | `static/buy/index.html` + `buy_page.go` | VFF branding + FC TG | Same static bytes | Identity + contact mix | Template from brand + support config |

**Доказательство**

- Handler: `serveBuy` → writes embedded `buyPageHTML` for `GET /buy` (`buy_page.go`, registered in `server.go`)
- HTML: `<title>VPN for Friends — купить VPN</title>`, `<h1>…VPN for Friends</h1>`, support `https://t.me/friends_connect_support`
- Нет templating / brand injection
- Reachability: active-public на **обоих** hostnames процесса

---

#### M7-CONTENT-005 — Account link static titles hardcode VFF

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-005 | P0 | Telegram↔web linking | yes | active-public | `link_start.html`, `link_invalid.html`, `link_standalone_conflict.html` | VFF titles | VFF titles | FC identity | Brand in `<title>` |

**Доказательство**

- Embeds in `account_pages.go`; served from `serveAccountLink` (`account_link_handlers.go`)
- Titles contain `— VPN for Friends`
- Conflict path uses `accountLinkStandaloneConflictHTML` (active, not dead)

---

#### M7-CONTENT-006 — Dynamic link notice pages append «VPN for Friends»

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-006 | P0 | Link error/notice HTML | yes | yes | `standaloneLinkNoticePage` | VFF suffix | VFF suffix | FC identity | `cfg.EffectiveBrand().Name` |

**Доказательство**

- `account_link_handlers.go` ~55–64: `body.WriteString(\` — VPN for Friends</title>\`)`
- Called for several link error/edge cases (~156+)
- Generated HTML, не dead code

---

#### M7-CONTENT-007 — Telegram `defaultLogoURL` VFF fail-open

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-007 | P0 | Telegram photos | yes | if `assets.logo_url` empty | `bot/service.go` `defaultLogoURL` / `logoPhoto` | VFF logo (ok if intentional) | VFF logo | Cross-brand asset | Fail-closed require logo; no VFF URL fallback |

**Доказательство**

- `const defaultLogoURL = "https://vpn-for-friends.com/logobot.jpg"` (line 23)
- `logoPhoto`: if `s.config.Assets.LogoURL == ""` → default (lines 51–55)
- Used widely: start/menu, keys list, pricelist, service preview, help, payments captions
- `assets.logo_url` **not** required by `Normalize()`
- Classification: **code-confirmed fail-open**; actual FC production impact = **production-config-dependent** (если logo задан — leak скрыт)

**Hotspot #1: подтверждён.**

---

#### M7-CONTENT-008 — Premium-connect: FC support + VFF redirect domain

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-008 | P0 | Premium connect | yes | active-public both brands | `static/premium-connect/index.html` | Wrong support (FC) + VFF redirect | Support OK-ish + VFF redirect domain | Bidirectional | Brand support + brand redirect/public base |

**Доказательство**

- Registered for **both** runtimes: `server.go` embeds + handles `/premium-connect`, `/premium-connect-test` (no brand gate)
- Support button ~182: `https://t.me/friends_connect_support` (fixed)
- JS `redirectBase = 'https://vpn-for-friends.com/redirect.html'` (~293) — FC user may leave FC host to VFF domain for Happ redirect helper
- Title page generic («Премиум AntiBlock VPN — Happ») — не VFF name, но support/redirect — brand-sensitive
- **Hotspot #5: подтверждён** как active на VFF и FC; severity P0 из‑за cross-brand URLs, не dead code

---

### P1

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-009 | P1 | Favicon / apple-touch | yes | active-public | `favicon.go` embeds | Shared binary | Shared binary | Likely shared VFF-derived icons | Brand-specific embeds or config-driven assets |
| M7-CONTENT-010 | P1 | Operator leads | operator-only | active-operator | `telegram_notifier.go` | Label «VPN for Friends» | Same wrong label | Ops mix / misroute triage | `brand.name` in message |
| M7-CONTENT-011 | P1 | Config vs UI | n/a (gap) | yes | `LandingURL` unused by UI | Profile landing ignored by account | Same | Silent drift | Wire UI to `brand.landing_url` |
| M7-CONTENT-012 | P1 | Content contract | n/a | architecture | BrandConfig + runtime JSON | Partial | Partial | Missing required content fields | Explicit content contract + validation |
| M7-CONTENT-013 | P1 | Contact model inconsistency | yes | yes | session support vs payment vs buy vs premium | Multiple sources | Multiple sources | Easy to misconfigure | One support/news/email model per brand |

#### M7-CONTENT-009 — Shared favicon / apple-touch for both brands

**Доказательство**

- `favicon.go` `//go:embed` трёх файлов; routes в `server.go`
- Один набор байтов в binary для VFF и FC
- Cache `max-age=604800` — смена иконки требует rebuild + cache bust
- README (documentation-only) описывает генерацию из `logobot.jpg` — косвенный признак VFF origin; бинарники здесь не интерпретировались как изображение
- **Hotspot #4: подтверждён** (shared embeds)

Severity P1 (не P0): без визуальной верификации нельзя утверждать pixel-level VFF trademark в каждой вкладке, но shared asset при разных брендах — обязательная доработка.

#### M7-CONTENT-010 — Lead notification hardcodes VFF

**Доказательство**

- `buildLeadTelegramMessage`: `"🆕 Заявка с сайта VPN for Friends\n\n"` (`telegram_notifier.go` ~51)
- Chat: `LeadsChatID` else `SupportChatID` — destination config-dependent; **label** всегда VFF
- Customer-visible: **no** (operator-only)
- **Hotspot #6: подтверждён** — operator-only impact; P1 не P0

#### M7-CONTENT-011 — `brand.landing_url` not consumed by account UI

См. M7-CONTENT-002. Отдельный P1 gap: поле есть в контракте BrandConfig и profiles, UI его игнорирует → ложное ощущение «landing уже brand-aware».

#### M7-CONTENT-012 — No explicit brand content contract

Отсутствуют validated fields для: web page titles, tagline, support email, news URL (web), favicon set, apple-touch, operator notification label, payment-support copy.

Profiles (`deploy/brands/*.json`) намеренно без content/email/assets — **Hotspot #8: подтверждён как observation, не auto-error**; gap = отсутствие связки profile ↔ runtime UI content + fail-closed validation для обязательных content fields.

#### M7-CONTENT-013 — Fragmented contact model

| Channel | Source today |
|---------|----------------|
| Bot support/news | `telegram.support_chat` / `news_channel` |
| Web session support button | same (+ env override) |
| Payment modal | hardcoded mixed HTML |
| `/buy` | hardcoded FC Telegram |
| Premium-connect | hardcoded FC Telegram |
| Email From | `email.from_*` / brand name |

Риск: даже при правильном bot config web surfaces расходятся.

---

### P2

| ID | Priority | Surface | User-visible | Runtime reachable | Source | Current VFF behavior | Current FC behavior | Risk | Recommended target |
|----|----------|---------|--------------|-------------------|--------|----------------------|---------------------|------|--------------------|
| M7-CONTENT-014 | P2 | JS/cookies | no (internal) | yes | `VFF_ACCOUNT`, `VFF_I18N`, `vff_lang` | Legacy names | Legacy names | Maintainability / confusion | Rename to brand-neutral |
| M7-CONTENT-015 | P2 | Email fallback | only invalid cfg | test / nil cfg | `legacyBrandDisplayName` | Fallback VFF | Fallback VFF | Legacy debt | Fail or require name; drop legacy |
| M7-CONTENT-016 | P2 | Shared theme | visual | yes | Bootstrap dark `#282a36` | Shared look | Shared look | Optional brand theme | Decide shared vs themed |
| M7-CONTENT-017 | P2 | Comments / docs / tests | no | test/docs-only | many `*_test.go`, docs | Examples | Examples | Noise for greps | Keep; do not treat as leaks |

#### M7-CONTENT-014 — Legacy internal `VFF_*` / `vff_lang`

- Cookie `accountLangCookieName = "vff_lang"` (`account_i18n.go` ~19)
- `window.VFF_ACCOUNT` / `window.VFF_I18N` in `index.html`, `session.html`, snippet in `account_pages.go`
- **Hotspot #3 (partial): подтверждён** — browser title из i18n (P0 via 001); favicon общий (009); JS globals `VFF_*` = P2
- Не user-facing brand string, но усложняет M7/M9

#### M7-CONTENT-015 — `legacyBrandDisplayName` unreachable in valid production

**Доказательство reachability**

- `brandDisplayName` uses `cfg.EffectiveBrand().Name`; empty → `"VPN for Friends"`
- `Normalize()` / brand validation: `brand.name is required` (`brand.go` ~209–210)
- Production load path rejects empty name → **valid runtime does not hit legacy fallback**
- Tests explicitly cover empty name (`TestSendAccountLoginEmail_EmptyBrandNameFallback`)
- With valid FC name, emails use «Friends Connect» (`sender_test.go`)
- **Hotspot #7: опровергнут как production leak**; оставлен как P2 technical debt / test helper path
- `email.from_name` / `from_email` values: **production-config-dependent** (не в репо)

#### M7-CONTENT-016 — Shared visual theme

Dark Bootstrap theme / `#2383e2` accents shared across buy/account/premium-connect. Generic, не содержит «VPN for Friends». Accepted shared unless product wants distinct themes (open question).

#### M7-CONTENT-017 — test/docs-only VFF strings

Примеры: `*_test.go` fixtures, `docs/MULTIBRAND_*.md`, root `README.md` (support URL docs, favicon rebuild from logobot). Класс: **test/docs-only**, не P0/P1.

---

## 6. Telegram content audit

| Area | Brand-specific? | Notes |
|------|-----------------|-------|
| Menu commands | Shared generic RU | `commands.go` — no brand name; comment mentions both brands |
| User messages / errors | Shared generic | «Ошибка системы…», услуга/тест/баланс — OK shared |
| Support button | Config | `telegram.support_chat` raw URL |
| News button | Config | `telegram.news_channel` if set |
| Logo | Config + **VFF fallback** | M7-CONTENT-007 |
| Account link message | Uses public base | brand-aware URL building (not copy identity) |
| Operator texts | N/A in bot package | leads via web notifier |

Нет hardcoded «VPN for Friends» в user-facing bot strings (кроме logo URL domain).

---

## 7. Web content audit

| Page | Brand identity | Support | Favicon | Notes |
|------|----------------|---------|---------|-------|
| `/account` | P0 VFF i18n | session N/A; login generic | shared | M7-CONTENT-001/002/014 |
| `/account/session` | P0 VFF i18n | config button OK; payment modal P0 mix | shared | M7-CONTENT-003 |
| `/account/link*` | P0 titles | generic copy | shared | M7-CONTENT-005/006 |
| `/buy` | P0 VFF | hardcoded FC TG | none in HTML head icons? (no favicon links in buy head — browser may still hit `/favicon.ico`) | M7-CONTENT-004 |
| `/payment/return` | **brand.name** | none | shared | Good pattern to reuse |
| `/premium-connect*` | generic title | FC hardcoded + VFF redirect | theme-color only | M7-CONTENT-008 |
| Admin APIs | not end-user HTML | — | — | out of customer UI |

JS alerts / network errors in account: generic («Сеть недоступна», «Ошибка») — OK shared.

Cookie `vff_lang`: P2 naming only.

---

## 8. Email and operator notifications

### Email

| Item | Source | Valid production | Notes |
|------|--------|------------------|-------|
| From display name | `email.from_name` or `brand.name` | brand-aware | OK if from_name not wrongly set |
| From email | `email.from_email` | config-dependent | Must be correct mailbox per brand in prod |
| Subject/body brand line | `brandDisplayName` → `brand.name` | brand-aware | |
| Legacy VFF name | empty name only | **not** valid prod path | P2 M7-CONTENT-015 |
| Login/link URLs | public base / token URLs | brand host | Not audited as content leak |

**Hotspot #7:** production leak **опровергнут** при обязательном `brand.name`; legacy — test/partial-config debt.

### Operator

| Notification | Label | Destination |
|--------------|-------|-------------|
| Public lead | Hardcoded «VPN for Friends» | LeadsChatID → SupportChatID |
| Web user registered | Generic «Web user registered» | same resolver |

Lead label = M7-CONTENT-010 (P1, operator-only).

---

## 9. Assets / favicon / theme audit

| Asset | Source | Route/usage | VFF | FC | Configurable? | Shared OK? | Cache | New field? |
|-------|--------|-------------|-----|----|---------------|------------|-------|------------|
| Telegram logo | `assets.logo_url` / `defaultLogoURL` | bot photos | URL or fallback | fallback risk | partial | No if logos differ | TG CDN | require `assets.logo_url` or brand asset URL |
| Favicon ICO | embed `static/favicon.ico` | `/favicon.ico` | shared | shared | no | No if brands differ | 7d | brand asset pack or path |
| Favicon 32 PNG | embed | `/favicon-32x32.png` | shared | shared | no | No | 7d | same |
| Apple touch | embed | `/apple-touch-icon.png` | shared | shared | no | No | 7d | same |
| Web logo in HTML | (none distinct) | — | — | — | — | — | — | optional `assets.web_logo_url` |
| Theme CSS | inline Bootstrap vars | all pages | shared | shared | no | Possibly yes | n/a | optional later |
| External VFF `logobot.jpg` | defaultLogoURL | Telegram | yes | risk | fallback | No | n/a | remove fallback |
| premium redirect | hardcoded VFF | Happ helper | yes | leak | no | No | n/a | brand public/landing helper |

Binary assets inventoried by name/size/hash only (no image interpretation).

---

## 10. Support / news / contact matrix

Secrets, chat IDs, tokens **не включались**. Значения production unknown.

| Surface | VFF source/value (code) | FC source/value (code) | Status | Evidence |
|---------|-------------------------|------------------------|--------|----------|
| Bot support button | runtime `telegram.support_chat` | same field per process | config-dependent OK model | `bot/service.go` |
| Bot news button | runtime `telegram.news_channel` | same | config-dependent | `bot/service.go` |
| Web session support | resolved support_chat (+env) | same | config-dependent OK | `support_url.go` |
| Payment modal support | **hardcoded** FC TG + VFF email | **same hardcoded** | **broken mix** | `account_i18n.go` PaymentMethodSupport |
| `/buy` support | hardcoded FC TG | same | broken for VFF; OK-ish for FC TG only | `buy/index.html` |
| Premium-connect support | hardcoded FC TG | same | broken for VFF | `premium-connect/index.html` |
| Support email in UI | `support@vpn-for-friends.com` hardcoded | same | FC leak / VFF maybe OK | PaymentMethodSupport |
| Marketing landing footer | `vpn-for-friends.com` hardcoded | same | FC leak | accountMarketingSiteURL* |
| Profile landing_url | `https://vpn-for-friends.com` | `https://friends-connect.club` | **unused by UI** | `deploy/brands/*.json` vs i18n |
| Operator lead label | hardcoded VFF | hardcoded VFF | ops mix | `telegram_notifier.go` |
| Email From | runtime email.* / brand.name | same mechanism | prod-dependent | `email/sender.go` |
| Shared support team? | unknown | unknown | open question | cannot decide from repo |

Не предполагается, что общий support team — ошибка; фиксируется **фактическая** фрагментация источников.

---

## 11. Error and localization audit

| Class | Examples | Brand leak? |
|-------|----------|-------------|
| Generic technical | «Ошибка системы, попробуйте позже», HTTP 405 text | no |
| Brand identity in errors | link notice titles (001/005/006) | yes (via titles) |
| Inconsistent contacts | payment vs session vs buy | yes (003/004/008) |
| Untranslated | many bot strings RU-only; account has RU/EN | copy debt, not brand |
| Security-sensitive | admin token routes exist; not content identity | out of M7 content scope |
| Email user-visible errors | generic send failures in UI | no brand |

RU/EN parity for account identity strings: **both** locales hardcode VFF (EN equally wrong for FC).

---

## 12. Configuration gap matrix

| Content item | Hardcoded | Config field | BrandConfig | Runtime profile | Production-dependent |
|--------------|-----------|--------------|-------------|-----------------|----------------------|
| display name (account UI) | yes (i18n) | — | `name` exists unused by i18n | profile `name` | no (code ignores) |
| display name (email) | legacy only if empty | `email.from_name` optional | `name` used | — | from_name |
| display name (payment return) | no | — | `name` used | — | no |
| landing URL (UI) | yes VFF | — | `landing_url` unused by UI | profile landing | no |
| public base URL | no for pages | — | `public_base_url` | profile | host routing |
| Telegram logo | default VFF URL | `assets.logo_url` | — | not in profile | yes |
| web logo | n/a | — | — | — | — |
| favicon / apple-touch | embed shared | — | — | — | rebuild |
| page title (account) | i18n VFF | — | unused | — | no |
| tagline | generic i18n | — | — | — | no |
| support URL (bot/session) | no | `telegram.support_chat` | — | not in profile | yes |
| support URL (buy/payment/premium) | yes mixed | — | — | — | no |
| support email | hardcoded VFF | — | — | — | no |
| news URL | bot only | `telegram.news_channel` | — | — | yes |
| operator notification label | hardcoded VFF | — | unused | — | no |
| email From name | — | `email.from_name` | fallback `name` | — | yes |
| email From address | — | `email.from_email` | — | — | yes |
| Telegram menu copy | shared generic | — | — | — | no |
| payment support text | hardcoded HTML | — | — | — | no |

**Возможные целевые источники (рекомендация, не финальная схема):**

1. Existing explicit config: `telegram.*`, `email.*`, `assets.logo_url`, `brand.name`, `brand.landing_url`
2. Extend `BrandConfig` for content-critical URLs (support_email, marketing paths) — **needs architecture decision**
3. Separate `BrandContentConfig` — optional if content grows
4. Keep shared generic UX copy in i18n without brand tokens
5. Fail-closed validation: missing required content → refuse start (no VFF silence)

Принцип: один процесс = один explicit brand; **cross-brand fallback запрещён**; shared generic content допустим если задокументирован.

---

## 13. False positives / accepted shared content

| Item | Class | Why accepted / not P0 |
|------|-------|------------------------|
| Word «VPN» in help/buy copy | generic | Not brand identity |
| FooterTagline «Secure access to your VPN services» | generic | No brand name |
| Bot menu command descriptions | generic | Shared UX |
| Bootstrap / dark theme colors | shared visual debt | P2 unless product requires distinct theme |
| `web_user_source: vpn-for-friends.com` in FC profile | attribution / identity storage | Not customer-visible content; covered by prior web-identity work / M8 |
| `legacyBrandDisplayName` with valid `brand.name` | unreachable prod | P2 only |
| `*_test.go` / docs VFF examples | test/docs-only | Fixtures |
| README support URL documentation | documentation-only | Not runtime |
| Payment return using `brand.name` | correct pattern | Not a finding |
| Session support from `telegram.support_chat` | correct pattern | Not a finding when config correct |
| Shared SHM / YooKassa merchant | infra | Not content UI |
| `commands.go` comment naming both brands | comment-only | P2 noise |

---

## 14. Recommended remediation plan

Независимые будущие коммиты (порядок по зависимостям):

### 1. `config: define explicit brand content contract`

- **Scope:** решить минимальный required set (name already; add support_email? web titles from name?; require `assets.logo_url`; document which telegram fields are mandatory for content).
- **Files:** `internal/config/brand.go` / config docs; possibly profile schema later.
- **Dependency:** none (architecture first).
- **Production rollout:** validation may fail closed — coordinate config update before deploy.
- **Rollback risk:** medium (start refusal).
- **Acceptance:** unit tests reject missing required content; FC/VFF fixtures updated.

### 2. `fix: remove VFF Telegram logo fallback`

- **Scope:** delete `defaultLogoURL` fail-open; require non-empty `assets.logo_url` (or embed per-brand — architecture from step 1).
- **Files:** `internal/app/bot/service.go`, config validation, bot tests.
- **Dependency:** step 1 or simultaneous prod config guarantee.
- **Rollout:** ensure both configs have logo URL before binary.
- **Rollback:** low if config already set.
- **Acceptance:** empty logo_url → process refuse or bot tests; no `vpn-for-friends.com/logobot` in runtime path.

### 3. `feat: render account identity from active brand`

- **Scope:** PageTitle/Footer/LoginH1/link titles/buy H1 from `brand.name`; SiteURL from `brand.landing_url` (+ EN path policy).
- **Files:** `account_i18n.go`, `account_pages.go`, link handlers/HTML, `buy_page.go` (+ template), tests.
- **Dependency:** landing URL policy decision (open Q).
- **Rollout:** binary only if landing_url already correct in prod.
- **Acceptance:** FC HTML contains «Friends Connect», not «VPN for Friends»; footer → friends-connect.club.

### 4. `feat: configure brand support and news contacts`

- **Scope:** one resolver for support URL/email used by session, payment modal, buy, premium-connect; remove hardcoded mix.
- **Files:** `support_url.go` (extend), i18n PaymentMethodSupport, buy/premium HTML or server-side inject, tests.
- **Dependency:** content contract fields.
- **Rollout:** verify telegram/email per brand.
- **Acceptance:** no `friends_connect_support` on VFF surfaces unless intentional shared support decision documented; no VFF email on FC unless intentional.

### 5. `feat: serve brand-specific favicon assets`

- **Scope:** per-brand embed or file set selected by `brand.id`; cache-bust strategy.
- **Files:** `favicon.go`, `server.go`, static assets, tests.
- **Dependency:** official FC assets (open Q).
- **Rollout:** binary + CDN/browser cache wait.
- **Acceptance:** different bytes or documented accepted shared icon.

### 6. `fix: brand operator notifications`

- **Scope:** lead text uses `brand.name`; optional brand in web-registration notify.
- **Files:** `telegram_notifier.go`, tests.
- **Dependency:** none.
- **Rollout:** binary only.
- **Acceptance:** FC lead message contains Friends Connect, not VFF.

### 7. `fix: brand premium-connect redirect and support`

- **Scope:** inject support URL + redirect base from brand public/landing; or brand-specific pages.
- **Files:** `server.go` / premium HTML templating, tests.
- **Dependency:** step 4; decision shared vs brand-specific page.
- **Acceptance:** no hardcoded `vpn-for-friends.com/redirect.html` on FC; VFF support not forced to FC username unless accepted.

### 8. `refactor: remove legacy VFF internal names`

- **Scope:** rename `VFF_ACCOUNT`/`VFF_I18N`/`vff_lang`; remove `legacyBrandDisplayName` or make it fail.
- **Files:** account static JS, i18n cookie, email sender, tests.
- **Dependency:** after identity/content fixes to reduce churn.
- **Acceptance:** grep clean for user-facing and cookie names; tests updated.

Не объединять M7 в один гигантский коммит.

---

## 15. Criteria for closing M7

Минимум:

- [ ] FC UI/Telegram/email не показывают VFF identity
- [ ] VFF UI сохраняет текущую identity
- [ ] support/news/landing ведут в активный бренд (или явно задокументированный shared contact)
- [ ] logo/favicon/title соответствуют активному бренду
- [ ] отсутствуют runtime VFF fallbacks для FC (`defaultLogoURL`, hardcoded marketing URLs, hardcoded titles)
- [ ] shared generic content явно принят и задокументирован (§13)
- [ ] config validation fail-closed для обязательных content fields
- [ ] оба runtime проходят tests, rollout и public smoke

---

## 16. Open questions

Нельзя решить только из репозитория:

1. Фактические production значения `assets.logo_url` для VFF и FC (закрывает ли уже M7-CONTENT-007 на FC?).
2. Фактические `telegram.support_chat` / `news_channel` / support mailboxes в explicit configs.
3. Нужна ли разная визуальная тема (цвета), или достаточно name/logo/favicon/links?
4. Официальные FC favicon/logo/apple-touch исходники.
5. Должен ли `/premium-connect` быть shared UX с brand injection или отдельными brand pages?
6. Допустим ли общий support team / один Telegram username на оба бренда? (если да — задокументировать как accepted shared; сейчас код всё равно непоследователен).
7. EN landing path: есть ли `https://friends-connect.club/en/` аналог VFF `/en/`?
8. Должен ли `email.from_email` отличаться по брендам в production (ожидаемо да)?

Не задаём вопросы, ответ на которые уже есть в коде (например: используется ли `defaultLogoURL`; читает ли account UI `LandingURL` — нет).

---

## Appendix A — Hotspot checklist (required confirmations)

| # | Observation | Verdict |
|---|-------------|---------|
| 1 | `defaultLogoURL` → VFF; empty `assets.logo_url` enables fallback | **Confirmed** (M7-CONTENT-007) |
| 2 | `accountMarketingSiteURL*`, PageTitle/Footer/LoginH1 VFF; PaymentMethodSupport mix; `vff_lang` legacy | **Confirmed** (001–003, 014) |
| 3 | account `index.html`: title from i18n; shared favicon; `VFF_*` globals | **Confirmed** (001, 009, 014) |
| 4 | `favicon.go` shared embeds | **Confirmed** (009) |
| 5 | premium-connect FC support; reachable on VFF | **Confirmed** active both brands (008) |
| 6 | lead notify «VPN for Friends» | **Confirmed** operator-only (010) |
| 7 | `legacyBrandDisplayName` | **Not a production leak** with valid BrandConfig; P2 (015) |
| 8 | brand profiles lack content/assets/support/email | **Confirmed observation**; not auto-error (012) |

## Appendix B — Path existence (audit-time)

Checked present in tree:

- `internal/app/bot/service.go`, `commands.go`
- `internal/app/web/account_i18n.go`, `account_pages.go`, `account_link_handlers.go`, `buy_page.go`, `payment_return.go`, `server.go`, `favicon.go`, `telegram_notifier.go`, `support_url.go`
- `internal/app/web/static/account/{index,session,link_start,link_invalid,link_standalone_conflict}.html`
- `internal/app/web/static/buy/index.html`, `payment/return.html`, `premium-connect/index.html`
- `internal/app/web/static/favicon.ico`, `favicon-32x32.png`, `apple-touch-icon.png`
- `internal/email/sender.go`
- `deploy/brands/vff.json`, `fc.json`
- `internal/config/brand.go`, `config.go`
