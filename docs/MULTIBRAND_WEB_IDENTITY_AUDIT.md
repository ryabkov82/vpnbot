# M5 — Аудит lifecycle web identity (VFF / FC)

Технический документ этапа анализа M5. Дополняет [`docs/MULTIBRAND_ROADMAP.md`](MULTIBRAND_ROADMAP.md) (§5 / M5). Roadmap на этом этапе **не изменяется**.

**Статус документа:** read-only анализ исходного кода. Runtime-код, тесты, конфигурация и production API на этапе аудита не изменялись и не вызывались.

---

## 0. Легенда статусов выводов

| Метка | Значение |
|-------|----------|
| **Подтверждено кодом** | Поведение восстановлено по исходникам; указаны путь и функция/тип |
| **Условный риск** | Риск реализуется при определённых условиях (общий secret, наличие production данных и т.п.), не доказанных в репозитории |
| **Требует production data audit** | Нельзя закрыть без read-only аудита SHM |
| **Рекомендация** | Предложение аудита; не принято владельцем |
| **Принятое ранее решение** | Уже зафиксировано в roadmap / Telegram identity (M4) |

---

## 1. Цель и ограничения этапа

### Цель

Восстановить фактический lifecycle web-пользователя в мультибрендовом runtime, выявить cross-brand риски и предложить целевую архитектуру с планом реализации.

### Ограничения этапа (соблюдены)

- не изменять runtime-код / тесты / конфигурацию;
- не обращаться к production API;
- не выполнять миграцию;
- не принимать необратимых решений без обоснования;
- не читать и не выводить production secrets.

### Изученный охват (минимум)

Конфиг и deploy:

- `internal/config/brand.go`, `internal/config/config.go`
- `deploy/brands/vff.json`, `deploy/brands/fc.json`
- `scripts/render-brand-config.sh`, `scripts/lib/brand_profile.sh`

Модели / identity helpers:

- `internal/webuser/webuser.go`
- `internal/models/models.go`

Service:

- `internal/service/service.go`
- `internal/service/web_user.go`
- `internal/service/link_web_email.go`
- `internal/service/brand_user.go`
- `internal/service/owned_user_service.go`

Web:

- `internal/app/web/account_web.go`
- `internal/app/web/account_token.go`
- `internal/app/web/account_link_handlers.go`
- `internal/app/web/google_oauth.go`
- `internal/app/web/public_buy_urls.go`
- `internal/app/web/server.go`
- `internal/app/web/admin_account.go`
- `internal/app/web/admin_web_order.go`
- `internal/app/web/account_balance_crypto.go`

API:

- `internal/infrastructure/api/client.go`

Связанные тесты (поиск по именам / символам): `web_user`, `web_identity`, `link_web_email`, `account_web`, `account_token`, `account_link`, `google_oauth`, `account_brand`, `account_service_category`, `owned_user_service`, `admin_account`, `admin_web_order`.

Дополнительно: `internal/app/bot/service.go` (создание Telegram link URL), `docs/SHM_USER_AUDIT.md` (паттерн read-only audit).

---

## 2. Фактические сценарии lifecycle

### 2.1 Email magic link — существующий пользователь

```
POST /api/account/login/start
  → NormalizeEmail
  → WebLoginFromEmailWithPrefix(email, brand.web_user_login_prefix)
  → FindUserByWebEmail  (login, затем login2)
  → иначе GetUserByLogin(computed login)   // дублирующий путь
  → CreateAccountToken | CreateAccountSignupToken
  → email с URL /account/session?token=...
  → клиент POST /api/account/session/start
  → ParseAndVerifyAccountToken → возврат того же account token
  → последующие /api/account/* с ?token= / body.token
```

**Файлы:** `serveAccountLoginStart`, `serveAccountSessionStart` (`account_web.go`); `FindUserByWebEmail`, `findUserByWebLoginKeys` (`web_user.go`); `CreateAccountToken` / `ParseAndVerifyAccountToken` (`account_token.go`).

| Вопрос | Факт |
|--------|------|
| Ключи поиска | Только `login` / `login2` = `<prefix>` + first16hex(SHA256(normalized_email)). Nested filter по `settings.web.email` **не используется** (комментарий в `FindUserByWebEmail`: SHM даёт ISE). |
| Что попадает в token | `typ=account`, `email`, `user_id=shmUser.ID`, `login=shmUser.Login` (для Telegram+linked это Telegram login, не web hash), `exp`. |
| Brand membership validation | **Нет.** Найденная запись возвращается как есть. |
| Запись другого бренда | Если у другого бренда уже занят тот же `login`/`login2` (при одинаковом prefix — неизбежно для одного email), процесс выдаст magic link на **чужую** SHM-запись. |

**Подтверждено кодом.**

### 2.2 Email magic link — новый пользователь

```
POST /api/account/login/start (user not found)
  → CreateAccountSignupToken(email, computed_login)
  → /account/session?token=signup
  → POST /api/account/session/start
  → ParseAndVerifyAccountSignupToken
  → проверка signup.Login == WebLoginFromEmailWithPrefix(active prefix)
  → FindOrCreateWebUser
      → findUserByWebLoginKeys
      → RegisterUser через apiClient (не Service.RegisterUser)
      → settings.web.{email,source}; brand_id НЕ пишется
  → CreateAccountToken
```

| Вопрос | Факт |
|--------|------|
| Формула login | `<prefix>` + first16hex(SHA256(lower(trim(email)))) — `WebLoginFromEmailWithPrefix` |
| settings | `settings.web.email`, `settings.web.source` = `brand.web_user_source` |
| `brand_id` | **Не записывается** в `findOrCreateWebUser` (`UserRegistrationRequest.Settings` без `BrandID`) |
| Один email в VFF и FC | При текущих profile values (`web_` / `web_`) — **один и тот же login**. Второй бренд найдёт существующую запись и не создаст независимую identity. |

**Подтверждено кодом.** Важно: `Service.RegisterUser` (Telegram) всегда пишет `settings.brand_id`, но web-путь вызывает `apiClient.RegisterUser` напрямую и обходит этот контракт. Кроме того, `Service.RegisterUser` требует `telegram.chat_id > 0` и не может обслуживать web-only регистрацию в текущем виде.

### 2.3 Google OAuth — обычный login/signup

```
GET /api/account/google/start
  → newGoogleOAuthState → cookie vff_google_oauth_state (Path=/, HttpOnly, SameSite=Lax)
  → optional link_token → cookie vff_google_oauth_link_token
  → redirect Google (client_id, redirect_uri=cfg.WebAccount.GoogleRedirectURL, state)

GET /api/account/google/callback
  → state == cookie; clear cookies
  → exchange code → userinfo (email_verified обязателен)
  → если link cookie: Telegram↔web linking (см. 2.5)
  → иначе FindOrCreateWebUser → CreateAccountToken → redirect /account/session?token=...
```

| Проверка | Факт |
|----------|------|
| OAuth state | Случайные 32 байта, сравнение cookie ↔ query |
| Cookie scope | `Path=/`, без Domain → host-only cookie текущего хоста |
| Redirect URL | Полностью из `web_account.google_redirect_url` (per-process config) |
| Поиск / регистрация | Тот же `FindOrCreateWebUser` без brand membership |
| Brand binding | Не записывается |

**Подтверждено кодом** (`google_oauth.go`).

### 2.4 Telegram → web linking через email

```
Telegram bot: GetUser(chatID) [brand-aware Telegram identity]
  → CreateAccountTelegramLinkToken(shm_user_id, telegram_chat_id)
  → URL brand.public_base_url + /account/link?token=...

GET /account/link
  → VerifyAccountTelegramLinkToken
  → GetUserByID(shm_user_id)  // БЕЗ brand validation
  → проверка Settings.Telegram.ChatID == claims
  → если уже linked (login2 prefix "web_" + settings.web.email): CreateAccountToken → session
  → иначе UI ввода email

POST /api/account/link/login/start
  → VerifyAccountTelegramLinkToken
  → FindUserByWebEmail (conflict если другой user_id)
  → CreateAccountLinkEmailToken → письмо /account/link/confirm?token=...

GET /account/link/confirm
  → VerifyAccountLinkEmailToken
  → LinkWebEmailForTelegramUser(shm_user_id, chat_id, email, "telegram_link")
  → login2 = web login; settings.web.{email,source}; raw settings merge
  → CreateAccountToken → /account/session
```

**Подтверждено кодом** (`bot/service.go`, `account_link_handlers.go`, `link_web_email.go`).

Замечания:

- `isWebLinkedTelegramUser` жёстко проверяет `strings.HasPrefix(login2, "web_")` — не использует active brand prefix.
- `LinkWebEmailForTelegramUser` не проверяет `settings.brand_id` и канонический Telegram login.
- Conflict detection (`GetUserByLogin` / `GetUserByLogin2` на web login) при общем prefix блокирует linking, если email уже занят **в любом бренде**.

### 2.5 Telegram → web linking через Google OAuth

```
GET /api/account/google/start?link_token=<telegram_link>
  → VerifyAccountTelegramLinkToken
  → cookie vff_google_oauth_link_token = link_token
  → OAuth flow

callback с link cookie:
  → VerifyAccountTelegramLinkToken(link cookie)
  → FindUserByWebEmail (conflict)
  → LinkWebEmailForTelegramUser(..., source="telegram_link_google")
  → CreateAccountToken → /account/session
```

Та же модель рисков, что в §2.4, плюс зависимость от корректного `GoogleRedirectURL` активного процесса.

### 2.6 Авторизованные операции кабинета

После session все операции принимают `account` token (`OrderTokenSecret` + HMAC).

| Операция | Handler | После verify | Brand membership | Category |
|----------|---------|--------------|------------------|----------|
| Список услуг + баланс | `serveAccountServices` | `GetUserBalanceByUserID(claims.UserID)`, `GetUserServicesByUserID(claims.UserID)`; опционально `GetUserByID` для telegram fields | нет | список услуг: API filter + local reject out-of-category rows |
| Платежи | `serveAccountPayments` | `GetUserPaysByUserID(claims.UserID)` | нет | нет |
| Каталог | `serveAccountCatalogServices` | только verify token; `GetServices()` | нет (user не читается) | API filter по category |
| Заказ услуги | `serveAccountServiceOrder` | `GetServiceByID` + local `ServiceCategoryAllowed`; `ServiceOrderByUserID(claims.UserID, ...)` | нет | handler + GetServiceByID |
| Удаление услуги | `serveAccountServiceDelete` | `GetOwnedUserServiceByUserID` → `DeleteUserServiceByUserID` | нет | owned + category |
| Connect | `serveAccountServiceConnect` | `GetOwnedUserServiceByUserID` | нет | owned + category |
| Topup YooKassa | `serveAccountBalanceTopup` | `BuildYooKassaPaymentURL(..., claims.UserID, ...)` | нет | нет |
| Topup CryptoCloud | `serveAccountBalanceTopupCrypto` | `BuildCryptoCloudPaymentURL(..., claims.UserID, ...)` | нет | нет |
| Admin account test | `serveAdminAccountTest` | admin token; `GetUserByLogin(web login)` | нет | услуги через GetUserServicesByUserID |
| Admin web-order test | `serveAdminWebOrderTest` | admin; `FindOrCreateWebUser` + order | нет | category check на service |

Где используется только `claims.UserID` (без повторной brand membership validation):

- payments, balance, topup (оба), service order (user), delete/connect (user id + owned service).

Где повторно читается SHM user:

- `serveAccountServices` — `GetUserByID` только для telegram UI fields;
- link flows — `GetUserByID` / `LinkWebEmailForTelegramUser`.

Где проверяется `claims.Login`:

- при verify: непустое поле обязательно;
- при signup session: `signup.Login` сверяется с computed login активного prefix;
- **account** handlers после verify **не** перепроверяют login против SHM.

Где проверяется `settings.brand_id`:

- **нигде** в web identity / account token path. Только Telegram path (`GetUser` / `userBelongsToBrand`).

Какие операции защищены только подписью token + TTL:

- balance / payments / topup URL generation — при валидной подписи достаточно `user_id` из claims.

---

## 3. Матрица текущих identity-ключей

| Поле | Назначение сейчас | Используется при поиске | Используется при проверке бренда | Примечание |
|------|-------------------|-------------------------|----------------------------------|----------|
| `user.login` | Primary SHM login: web hash **или** Telegram `@...` | да (`GetUserByLogin`) | только Telegram (`GetUser` / canonical login) | Web lookup не проверяет brand |
| `user.login2` | Secondary: web hash на Telegram-аккаунте | да (`GetUserByLogin2`) | нет | Ключ cross-brand collision при общем prefix |
| `settings.brand_id` | Brand membership (Telegram M4) | нет для web | Telegram: да; web: **нет** | Web registration не пишет |
| `settings.web.email` | Метаданные email | нет (нельзя фильтровать nested) | нет | Attribution/display; не boundary |
| `settings.web.source` | Канал регистрации / linking | нет | нет | Перезаписывается при link (`telegram_link*`) |
| `settings.telegram.chat_id` | Telegram binding | link flows | Telegram GetUser + link chat match | Не заменяет brand_id |
| account token `user_id` | SHM user_id сессии | операции кабинета | нет | Доверие после HMAC |
| account token `login` | Snapshot login при выдаче | signup verify; не для account ops | нет | Может быть Telegram login |
| token signing secret | `web_sales.order_token_secret` | create/verify всех 4 typ | нет brand claim | Per-process config; равенство VFF/FC **не подтверждено** |

### Текущая формула web login

```
<prefix> + first16hex(SHA256(normalized_email))
normalized_email = strings.ToLower(strings.TrimSpace(email))
```

Источник: `internal/webuser/webuser.go` — `WebLoginFromEmailWithPrefix`.

`WebLoginFromEmail` — явная VFF-compatibility обёртка с жёстким prefix `web_` (не fallback parameterized API).

### Текущие profile values (deploy)

| Brand | `web_user_login_prefix` / `web_login_prefix` | `web_user_source` |
|-------|-----------------------------------------------|-------------------|
| VFF (`deploy/brands/vff.json`) | `web_` | `vpn-for-friends.com` |
| FC (`deploy/brands/fc.json`) | `web_` | `vpn-for-friends.com` |

Renderer: `scripts/render-brand-config.sh` копирует profile → `brand.web_user_login_prefix` / `brand.web_user_source`.

**Подтверждено кодом / deploy profiles.** Фактические значения на production hosts требуют ops verification, но репозиторные profiles одинаковы.

---

## 4. Матрица типов пользователей

Для Telegram + web: `login` = Telegram identity, `login2` = web identity.

| Тип | VFF login | FC login | login2 | brand_id | Основной риск |
|-----|-----------|----------|--------|----------|---------------|
| Web-only | `web_<hash>` | сейчас тоже `web_<hash>` | пусто | пусто (web reg) | Collision login; захват сессии другим брендом |
| Telegram-only | `@<chat_id>` | `@fc_<chat_id>` | пусто | vff / legacy empty / fc | Изолировано M4; web link может пересечь |
| Telegram + linked email | `@<chat_id>` | `@fc_<chat_id>` | `web_<hash>` (оба) | как у Telegram | login2 collision; link захватит/заблокирует чужой email |
| Legacy VFF web user | `web_<hash>` | — | пусто | обычно empty | Нужен legacy-контракт при введении brand checks |
| Один email в двух брендах | один `web_<hash>` | тот же | — | — | **Невозможно** независимо при текущем prefix |
| Один Telegram ID в двух брендах | `@<id>` | `@fc_<id>` | каждый может иметь свой login2 | разные | Если оба link один email → conflict на общем web login |

---

## 5. Findings

### WEB-ID-01 — одинаковый web prefix

| | |
|--|--|
| **Severity** | **Critical** |
| **Файлы / функции** | `deploy/brands/vff.json`, `deploy/brands/fc.json` (`web_login_prefix`); `WebLoginFromEmailWithPrefix`; `findOrCreateWebUser` |
| **Текущее поведение** | VFF и FC используют prefix `web_`. Один normalized email → один SHM login. |
| **Cross-brand сценарий** | Пользователь регистрируется в VFF; тот же email входит на FC → FindOrCreate находит VFF-запись. |
| **Последствия** | Нет независимых web identities; возможна выдача session чужого бренда; блокировка независимой регистрации. |
| **Необходимое исправление** | Развести prefix (кандидат: FC `web_fc_`) **и** brand membership; смена prefix только после data audit / миграции. |

**Подтверждено кодом.**

### WEB-ID-02 — web registration не пишет brand_id

| | |
|--|--|
| **Severity** | **High** |
| **Файлы / функции** | `findOrCreateWebUser` (`web_user.go`); `models.UserRegistrationRequest`; контраст с `Service.RegisterUser` (`service.go`) |
| **Текущее поведение** | Регистрация пишет только `settings.web.{email,source}`. `BrandID` не задаётся. Вызов идёт в `apiClient.RegisterUser`, не в brand-aware `Service.RegisterUser`. |
| **Cross-brand сценарий** | Даже после разведения prefix нельзя надёжно отличить «свой» user без brand_id / membership helper. |
| **Последствия** | Невозможность строгой membership validation; legacy ambiguity. |
| **Необходимое исправление** | Писать `settings.brand_id = activeBrandID` при web registration; отдельный registrar path (не требовать telegram.chat_id). |

**Подтверждено кодом.**

### WEB-ID-03 — web lookup не валидирует brand membership

| | |
|--|--|
| **Severity** | **Critical** |
| **Файлы / функции** | `findUserByWebLoginKeys`, `FindUserByWebEmail`, `FindOrCreateWebUser`; callers в `account_web.go`, `google_oauth.go`, link handlers |
| **Текущее поведение** | Любая запись с matching login/login2 возвращается. Нет `userBelongsToBrand` / `ErrUserIdentityMismatch`. |
| **Cross-brand сценарий** | Найден user с «чужим» brand_id (или legacy) → выдаётся account token. |
| **Последствия** | Session hijack across brands; при трактовке mismatch как not found — риск повторной регистрации в занятый login (roadmap принцип §2.4). |
| **Необходимое исправление** | После lookup: membership check; mismatch → `ErrUserIdentityMismatch` (не not found). |

**Подтверждено кодом.**

### WEB-ID-04 — LinkWebEmailForTelegramUser не проверяет активный бренд

| | |
|--|--|
| **Severity** | **High** |
| **Файлы / функции** | `LinkWebEmailForTelegramUser` (`link_web_email.go`); `GetUserByID` (`service.go` → прямой API); `serveAccountLink`, confirm/OAuth link |
| **Текущее поведение** | `GetUserByID` без brand validation; проверяется только `telegram.chat_id`; login2/web settings пишутся с active prefix; raw settings merge сохраняется. Канонический Telegram login и `settings.brand_id` **не** сверяются. |
| **Cross-brand сценарий** | Telegram link token другого бренда (при общем secret) или ошибочный shm_user_id с совпавшим chat_id → link на чужую запись; conflict на общем web login2 блокирует/связывает не тот бренд. |
| **Последствия** | Захват login2; порча settings.web; невозможность независимого linking. |
| **Необходимое исправление** | Validate brand membership + canonical telegram login; conflict checks brand-scoped. |

**Подтверждено кодом** (отсутствие brand checks). Условный риск усиления — общий `OrderTokenSecret` (см. WEB-ID-05).

### WEB-ID-05 — токены не содержат brand identity

| | |
|--|--|
| **Severity** | **High** (Critical **условно**, если secrets совпадают) |
| **Файлы / функции** | `AccountTokenClaims`, `AccountSignupTokenClaims`, `AccountTelegramLinkClaims`, `AccountLinkEmailClaims`; create/verify в `account_token.go` |
| **Текущее поведение** | Claims: typ, email/user ids, login, exp. Поля `brand_id` нет. Verify проверяет secret + typ + exp + обязательные поля. |
| **Cross-brand сценарий** | **Условный:** если VFF и FC используют одинаковый `web_sales.order_token_secret`, подписанный token одного бренда принимается другим процессом (hosts разные, но secret общий → HMAC валиден). |
| **Последствия** | Session / signup / link / link-email переносимы между процессами при общем secret. |
| **Необходимое исправление** | Добавить `brand_id` в claims; verify с expected brand; рассмотреть разные secrets как defense-in-depth (не замена claim). |

**Подтверждено кодом:** отсутствие brand в claims.  
**Не подтверждено:** равенство production secrets (репозиторий не содержит production config secrets).  
**Условный риск:** при одинаковом secret — да, token одного бренда принимается другим.

TTL по умолчанию: account/signup ≈ `order_token_ttl_hours` (default 24h); telegram link 30m; link email 60m — принудительная инвалидация старых форматов относительно безопасна в горизонте TTL.

### WEB-ID-06 — account handlers доверяют user_id из token

| | |
|--|--|
| **Severity** | **High** |
| **Файлы / функции** | `serveAccountPayments`, `serveAccountServices`, `serveAccountBalanceTopup`, `serveAccountBalanceTopupCrypto`, `serveAccountServiceOrder`, delete/connect |
| **Текущее поведение** | После HMAC: операции по `claims.UserID` без повторной brand membership validation и без сверки login/email с SHM. |
| **Cross-brand сценарий** | Валидный token «чужого» бренда (WEB-ID-05) → баланс, платежи, topup URL, заказ в рамках category изоляции активного процесса. |
| **Последствия** | Чтение баланса/платежей чужого user_id; создание платежных URL; заказ услуг **своей** category на чужой user (деньги/услуги на чужом аккаунте). |
| **Необходимое исправление** | Brand-bound tokens + optional re-load/membership check на чувствительных операциях. |

**Подтверждено кодом.**

### WEB-ID-07 — web_user_source не является identity boundary

| | |
|--|--|
| **Severity** | **Medium** (design debt / false safety) |
| **Файлы / функции** | `BrandConfig.WebUserSource`; `findOrCreateWebUser`; `LinkWebEmailForTelegramUser` (source overwrite) |
| **Текущее поведение** | Source пишется при регистрации; при linking становится `telegram_link` / `telegram_link_google`. VFF и FC profiles сейчас одинаковы (`vpn-for-friends.com`). |
| **Cross-brand сценарий** | Попытка использовать source как brand discriminator ложна: одинаковые значения; source мутабелен. |
| **Последствия** | Ошибочная миграция/классификация, если опираться на source. |
| **Необходимое исправление** | Identity = prefix + brand_id (+ tokens). Source оставить attribution-only. |

**Подтверждено кодом.**

### WEB-ID-08 — backward compatibility

| | |
|--|--|
| **Severity** | **High** (operational) |
| **Файлы / функции** | все web lookup/registration; deploy FC prefix; token verify |
| **Текущее поведение** | Legacy VFF web users: `login=web_<hash>`, `brand_id` обычно empty. Telegram+login2 уже используют `web_<hash>`. FC web users (если есть) неотличимы по login от VFF. |
| **Cross-brand сценарий** | Смена FC prefix до миграции → существующие FC web (если есть) «пропадают». Строгий brand_id без legacy → VFF web login ломается. Invalidation tokens → logout в пределах TTL. |
| **Последствия** | Потеря доступа; дубликаты; необходимость allowlist migration. |
| **Необходимое исправление** | Production data audit → утвердить legacy-контракт VFF → затем code → затем prefix/migration. |

**Требует production data audit** для объёма FC web / linked login2 / empty brand_id.

### WEB-ID-09 — cookie и OAuth naming/config

| | |
|--|--|
| **Severity** | **Low** (tech debt); повышается при shared host/cookie domain |
| **Файлы / функции** | `googleOAuthCookieName = "vff_google_oauth_state"`; `googleOAuthCookieLinkToken = "vff_google_oauth_link_token"`; `GoogleRedirectURL`; cookie Path=/ без Domain |
| **Текущее поведение** | VFF-oriented имена cookie; redirect URL из config процесса; host-only cookies. |
| **Cross-brand сценарий** | На разных hosts (`connect...` vs `connect-fc...`) host-only cookies не шарятся. Имя само по себе не security bug. Риск — путаница ops / будущий общий parent domain. |
| **Последствия** | Техдолг; потенциальный cookie clash при общем Domain. |
| **Необходимое исправление** | Brand-neutral или brand-prefixed cookie names; явный Domain policy. |

**Подтверждено кодом** как tech debt, не как Critical security bug.

### Дополнительный finding: WEB-ID-10 — hardcoded `web_` в link detection

| | |
|--|--|
| **Severity** | **Medium** |
| **Файлы / функции** | `isWebLinkedTelegramUser` (`account_link_handlers.go`) |
| **Текущее поведение** | `HasPrefix(login2, "web_")` — не active prefix. |
| **Cross-brand сценарий** | После введения `web_fc_` детектор «уже привязан» может давать ложные срабатывания/пропуски в зависимости от строки. |
| **Последствия** | Некорректный short-circuit на `/account/link`. |
| **Необходимое исправление** | Проверять canonical web login / prefix активного бренда + settings.web.email. |

**Подтверждено кодом.**

---

## 6. Service isolation (связь с web identity)

Не опираемся на roadmap без проверки кода.

| Метод | Фильтр category в API | Local defense-in-depth | Риск при чужом account token |
|-------|----------------------|------------------------|------------------------------|
| `GetUserServicesByUserID` → `APIClient.GetUserServices` | да (`filter.category`) + drop out-of-category rows | да в API client | Список услуг **своей** category чужого user; баланс/платежи — отдельно |
| `GetOwnedUserServiceByUserID` | API filter + `ownedUserServiceMatches` | да | Чужая category → `ErrUserServiceUnavailable`; своя category того же user_id — доступна |
| `ServiceOrderByUserID` → `APIClient.ServiceOrder` | **нет** в API order body | web handler: `GetServiceByID` + `ServiceCategoryAllowed` | Заказ только если service_id виден в category активного бренда; user_id из token |
| `DeleteUserServiceByUserID` | через owned check | да | Удаление только owned+category |
| `GetServices` | да | — | Каталог бренда; token только auth gate |
| `GetServiceByID` | да + local allow | да | Чужая category → not found |

Отличие защиты списка и конкретной операции:

- **Список услуг:** category filter на чтении.
- **Конкретная операция (connect/delete):** owned + category.
- **Баланс / платежи / topup:** **нет** category/brand; только `user_id` из token.
- **Order:** category на выборе service; user_id из token без brand membership.

Полный аудит платежей M6 не выполняется. Зависимости от web identity: topup/payment URL строятся с `claims.UserID`; изоляция payment profile — отдельный M6.

---

## 7. Варианты целевой архитектуры

### Вариант A — только разные prefix

- VFF: `web_`
- FC: `web_fc_`

**Почему недостаточно как единственная защита:**

- не закрывает token portability при общем secret;
- не закрывает handlers, доверяющие `user_id`;
- не закрывает linking по shm_user_id без brand check;
- legacy / ошибки prefix / будущий бренд без дисциплины;
- lookup mismatch vs not found остаётся опасным без membership errors.

### Вариант B — только settings.brand_id

**Проблемы:**

- collision login при одинаковом prefix остаётся (SHM uniqueness на login);
- поиск по login происходит **до** проверки brand → легко спутать not found и mismatch;
- login2 на Telegram-аккаунтах всё ещё глобален по значению;
- legacy empty brand_id требует отдельного контракта;
- tokens без brand claim всё ещё переносимы.

### Вариант C — prefix + brand_id + brand-bound tokens (**основной кандидат**)

- VFF login prefix: `web_`
- FC login prefix: `web_fc_`
- будущий бренд: `web_<brand_id>_`
- `settings.brand_id = active brand` на web registration и учитывается при lookup/linking
- `brand_id` внутри всех account/link/signup tokens; verify с expected brand

| Слой | Роль |
|------|------|
| Prefix | Физическое разделение SHM login / login2 |
| brand_id | Membership + legacy policy |
| Token brand claim | Session/link не переносятся между процессами |
| Разные secrets | Defense-in-depth, не замена claim |

### Разделение статусов решения

| Утверждение | Статус |
|-------------|--------|
| Telegram identity уже brand-aware (M4) | **Принятое ранее решение** |
| Web identities должны быть независимы (roadmap §5) | **Принятое ранее решение** (цель) |
| FC prefix = `web_fc_`, шаблон `web_<brand_id>_` | **Рекомендация** аудита; **не утверждено** |
| Variant C как целевая архитектура | **Рекомендация** |
| Есть ли production FC web users / нужна ли миграция | **Требует production data audit** |

---

## 8. Предлагаемые правила web membership

Аналог Telegram `userBelongsToBrand`, но для web login / login2.

### Canonical web login

```
canonicalWebLogin = WebLoginFromEmailWithPrefix(normalizedEmail, activeBrand.WebUserLoginPrefix)
```

Запись «является web identity ключа» если `login == canonical` **или** `login2 == canonical`.

### VFF (предложение; финал после data audit)

Принадлежит VFF, если:

1. `settings.brand_id == "vff"`, **либо**
2. пустой `brand_id` **только** при каноническом VFF web login/login2 (`web_<hash>` с prefix `web_`), **и**
3. запись не имеет признаков другого бренда (например `brand_id` иного значения; login вида `web_<other>_...` — отвергать).

Mismatch → `ErrUserIdentityMismatch`, не not found.

### FC и будущие бренды

- только точное `settings.brand_id == active`;
- только канонический brand-aware login/login2;
- **никакого** fallback на VFF `web_<hash>`;
- mismatch → отдельная identity error.

Окончательный legacy-контракт VFF зависит от production data audit (доля empty brand_id, коллизии, FC registrations).

---

## 9. Target token contract

Минимальный целевой payload (все четыре typ):

```json
{
  "typ": "account|account_signup|account_telegram_link|account_link_email",
  "brand_id": "fc",
  "email": "...",
  "user_id": 123,
  "login": "...",
  "exp": 1234567890
}
```

Для telegram_link / link_email: `shm_user_id`, `telegram_chat_id`, `email` (link_email) + обязательный `brand_id`.

| Вопрос | Рекомендация |
|--------|--------------|
| Что подписывается | Весь JSON payload (как сейчас HMAC-SHA256) |
| Expected brand при verify | `cfg.EffectiveBrand().ID` процесса |
| Mismatch | reject (invalid token / identity mismatch), не silent ignore |
| iss / aud | необязательны, если есть `brand_id` + per-process secret; можно добавить позже |
| Разные secrets по брендам | **желательно** как defense-in-depth; **не замена** brand claim |
| Старый формат после deploy | отклонять (нет `brand_id`) **или** короткий dual-read только для VFF в миграционном окне |
| Принудительная инвалидация | допустима: TTL account/signup ≤ 24h (default), link tokens короче |

---

## 10. Production data audit — спецификация (без выполнения)

Production API на этом этапе **не вызывается**.

### Scope выборки

Пользователи, у которых выполняется хотя бы одно:

- `login` starts with `web_`
- `login2` starts with `web_`
- `settings.web.email` non-empty

### Безопасные поля отчёта

- `user_id`, `login`, `login2`
- `settings.brand_id`, `settings.web.email`, `settings.web.source`
- наличие `telegram.chat_id` (bool / chat_id при необходимости классификации)
- категории и статусы услуг
- количество платежей — только если нужно для классификации (без comment/secrets)

Не включать: password, secrets, полный raw settings dump, payment comments.

### Классификации

| Класс | Смысл |
|-------|--------|
| `vff_only` | Evidence только VFF (category / brand_id / login shape) |
| `fc_only` | Evidence только FC |
| `shared` | Evidence обоих брендов на одной записи |
| `empty` | Нет услуг/платежей/баланса — низкая активность |
| `ambiguous` | Неоднозначность |
| `duplicate_email` | Один normalized email → несколько user rows |
| `login_collision` | Один web login/login2 претендует на два бренда |
| `already_brand_aware` | Корректный brand_id + ожидаемый prefix |

### Правила классификации (черновик)

- По услугам: category ∈ {vff category, fc category} как evidence (аналог `SHM_USER_AUDIT`).
- Telegram: login `@<id>` vs `@fc_<id>` + brand_id.
- Duplicate email: нормализовать `settings.web.email` и/или обратный расчёт невозможен из hash — группировать по одинаковому `login`/`login2` web-ключу и по email metadata.
- Кандидаты миграции: FC evidence + login ещё `web_<hash>` + свободный целевой `web_fc_<hash>` (если email известен) **или** allowlist по user_id.
- Manual review: shared, ambiguous, duplicate_email, login_collision.
- CLI: расширить `cmd/shm-user-audit` **или** отдельный read-only `web-identity-audit` — **открытый вопрос**. На этом этапе CLI не реализуется.

---

## 11. Рекомендуемый implementation plan

Порядок учитывает backward compatibility: **нельзя** менять FC prefix до понимания production данных.

1. **tests:** зафиксировать cross-brand web identity contract (failing tests сначала допустимы в отдельном PR после утверждения).
2. **service:** web brand membership helpers + `ErrUserIdentityMismatch` для web path.
3. **service:** писать `brand_id` при web registration (отдельный путь без telegram.chat_id).
4. **service:** валидировать brand при login/login2 lookup.
5. **service:** защитить Telegram ↔ web linking (membership + canonical telegram login).
6. **web:** `brand_id` во все token claims.
7. **web:** expected brand при token verify; политика old tokens.
8. **audit:** read-only production web-identity audit (CLI).
9. **migration (если нужно):** allowlist, dry-run-first; **затем** смена FC prefix в deploy profile на `web_fc_`.
10. **deploy:** назначить FC prefix только после миграции/подтверждения отсутствия FC web rows на старом login.
11. **rollout:** VFF/FC deployment + e2e smoke.
12. **cleanup:** удалить одноразовую migration CLI.

Критическое предупреждение: шаг «deploy FC prefix» **перед** миграцией существующих FC web users сделает их недоступными.

---

## 12. Test matrix (обязательная для будущей реализации)

| # | Сценарий |
|---|----------|
| 1 | Одинаковый email создаёт разные VFF/FC users |
| 2 | VFF runtime не принимает FC web user |
| 3 | FC runtime не принимает VFF web user |
| 4 | wrong brand_id → identity mismatch (не not found) |
| 5 | FC не делает fallback на `web_<hash>` |
| 6 | Legacy VFF user без brand_id работает по утверждённому контракту |
| 7 | account token VFF отклоняется FC |
| 8 | signup token VFF отклоняется FC |
| 9 | Telegram link token VFF отклоняется FC |
| 10 | email-link token VFF отклоняется FC |
| 11 | Одинаковый Telegram ID и email независимо существуют в VFF и FC |
| 12 | Linking не захватывает login2 другого бренда |
| 13 | OAuth callback не возвращает user другого бренда |
| 14 | Сервис другого бренда недоступен |
| 15 | Raw settings не теряются при update (link) |
| 16 | Повторная операция linking идемпотентна |
| 17 | `isWebLinkedTelegramUser` / equivalent работает с brand-aware prefix |
| 18 | Balance/payments с token чужого бренда отклоняются на verify |

---

## 13. Открытые вопросы (решения владельца)

1. Утверждаем ли FC prefix `web_fc_`?
2. Используем ли общий шаблон `web_<brand_id>_` для будущих брендов?
3. Разрешаем ли VFF legacy users без `brand_id` (и при каких условиях)?
4. Инвалидируем ли токены старого формата при deployment (без dual-read)?
5. Должны ли signing secrets быть разными по брендам?
6. Меняем ли FC `web_user_source` на `friends-connect.club` (attribution only)?
7. Расширяем существующий `shm-user-audit` или создаём отдельный read-only audit?
8. Есть ли сейчас production web-регистрации FC, требующие миграции?
9. Нужен ли dual-read window для old tokens на VFF?
10. Требуется ли re-load SHM user + membership на каждом account API вызове или достаточно brand-bound token?

---

## 14. Сводка рисков

### Critical (подтверждено кодом)

- WEB-ID-01 одинаковый prefix → нет независимых web identities.
- WEB-ID-03 lookup без brand membership.

### High

- WEB-ID-02 registration без brand_id.
- WEB-ID-04 linking без brand checks.
- WEB-ID-05 tokens без brand claim (**Critical условно** при общем secret).
- WEB-ID-06 handlers доверяют `user_id`.
- WEB-ID-08 совместимость / порядок миграции.

### Medium / Low

- WEB-ID-07 source ≠ identity boundary.
- WEB-ID-09 / WEB-ID-10 naming и hardcoded prefix.

### Требует production data audit

- наличие FC web-only / linked users;
- объём legacy VFF web без brand_id;
- duplicate email / login_collision;
- необходимость allowlist migration;
- фактические значения `order_token_secret` на VFF vs FC (только факт «равны/нет», без публикации секретов).

---

## 15. Рекомендуемая целевая архитектура (итог)

**Рекомендация аудита:** Вариант C — **разные web login prefix + запись/проверка `settings.brand_id` + brand-bound tokens**, с разными secrets как defense-in-depth.

Это согласуется с уже принятой моделью Telegram identity (M4) и принципами roadmap (§2), но конкретные значения prefix/legacy policy **не утверждаются** до production data audit и решения владельца по §13.

---

## 16. Карта соответствия roadmap

| Roadmap требование (§5 / M5) | Статус по коду сейчас |
|------------------------------|------------------------|
| Один email независимо в VFF и FC | ❌ невозможно при общем prefix |
| Web registration пишет brand_id | ❌ |
| Поиск проверяет бренд | ❌ |
| Magic link ограничен брендом | ❌ (нет brand в token) |
| Session не переносится | ❌ условно при общем secret; hosts разные |
| OAuth не связывает бренды | ❌ через общий FindOrCreate |
| Telegram↔web не захватывает чужой бренд | ❌ недостаточно проверок |
| Услуги другого бренда недоступны | ✅ category isolation (M3) на service ops; баланс/pay — вне category |
| Предварительный audit существующих web users | ⬜ спецификация в §10; не выполнен |

---

*Конец документа M5 web identity audit.*
