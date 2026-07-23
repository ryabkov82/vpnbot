# M6 — Payment end-to-end audit (VFF / Friends Connect)

## Статус документа

Это **read-only статический аудит** исходного кода ветки `main`.

Ограничения аудита:

- production API и payment providers **не вызывались**;
- реальные платежи и callback **не выполнялись**;
- SSH / deployment / production config и secrets **не читались**;
- runtime-код, тесты и deployment profiles **не изменялись**.

Выводы разделены на:

- **code-confirmed** — подтверждено текущим кодом репозитория;
- **config-dependent** — зависит от SHM / provider / production config вне репозитория;
- **manual verification** — требует sandbox/e2e или ручной проверки.

Источник истины — текущий код `main`. Исторические документы (`docs/MULTIBRAND_ROADMAP.md`, `docs/MULTIBRAND_WEB_IDENTITY_AUDIT.md`) использованы только как контекст.

Roadmap (`docs/MULTIBRAND_ROADMAP.md`) этим аудитом **не обновляется**: статус M6 остаётся незакрытым до анализа и последующих исправлений.

---

## 1. Цель и рамка брендов

Цель: восстановить фактический lifecycle платежей и оценить brand isolation для:

| | VPN for Friends | Friends Connect |
|--|-----------------|-----------------|
| `brand_id` | `vff` | `fc` |
| service category | `vpn-mz-test` | `vpn-mz-fc` |
| payment profile | `telegram_bot` | `telegram_friends_connect_bot` |
| public base URL | `https://connect.vpn-for-friends.com` | `https://connect.friends-connect.club` |

Значения profiles: `deploy/brands/vff.json`, `deploy/brands/fc.json`.

Проверяемые свойства процесса:

1. использует `payment_profile` активного бренда;
2. относится к пользователю активного бренда;
3. относится к услуге category активного бренда;
4. не активирует / не продлевает услугу другого бренда;
5. возвращает пользователя на домен активного бренда;
6. формирует правильные branded email/тексты;
7. безопасно обрабатывает повторные callback/webhook.

---

## 2. Архитектурный вывод (кратко)

Платёжный контур в `vpnbot` — это **инициатор**, а не полный payment processor.

Фактически:

1. **Заказ услуги** создаётся через SHM Admin API (`PUT /shm/v1/admin/service/order`) и часто оставляет `user_service` в статусе `NOT PAID` / `BLOCK`.
2. **Оплата** — это пополнение внутреннего баланса SHM-пользователя:
   - Telegram: SHM WebApp `tg_payments_webapp` с query `profile=<brand.payment_profile>`;
   - Web: ссылки на SHM CGI `yookassa.cgi` / `cryptocloud.cgi` с `user_id` + `amount` (+ `ps`, `ts`, `action=create`).
3. **Callback / webhook / success/fail URL / idempotency / merchant credentials** в этом репозитории **отсутствуют**. Они живут в SHM / платёжных провайдерах.
4. После оплаты `vpnbot` **не** получает событие и **не** заказывает услугу повторно: UI явно ожидает, что SHM сам зачислит баланс и активирует/продлит неоплаченные услуги.

Следствие для M6: часть brand isolation подтверждается кодом `vpnbot` (identity + category на этапе order/top-up init). Критичный хвост — callback/activation/idempotency/merchant profile — **config/manual dependent** и не может быть закрыт только статическим аудитом этого репозитория.

---

## 3. Где живёт платёжный код

| Область | Путь | Роль |
|---------|------|------|
| Brand config | `internal/config/brand.go` | `BrandConfig.PaymentProfile`, `Config.PaymentProfile()`, обязательная валидация |
| Deployment profiles | `deploy/brands/vff.json`, `deploy/brands/fc.json` | разные `payment_profile` / `service_category` / `public_base_url` |
| URL builders | `internal/payments/shm.go`, `yookassa.go`, `cryptocloud.go` | сборка SHM CGI create-URL |
| Telegram balance | `internal/app/bot/service.go` (`handleBalance`) | единственный runtime-потребитель `PaymentProfile()` |
| Telegram order | `internal/app/bot/service.go` (`handleServiceOrder`, `handleServiceBuy`) | заказ + category gate; без payment URL |
| Web top-up | `internal/app/web/account_web.go` (`serveAccountBalanceTopup`) | YooKassa CGI |
| Web crypto top-up | `internal/app/web/account_balance_crypto.go` | CryptoCloud CGI |
| Web order | `internal/app/web/account_web.go` (`serveAccountServiceOrder`) | order + category; CryptoCloud URL только для EN + needsTopUp |
| Admin test order | `internal/app/web/admin_web_order.go` | admin-token + category + YooKassa URL |
| SHM API | `internal/infrastructure/api/client.go` | balance, services, order, pays history |
| Ownership | `internal/service/owned_user_service.go` | user_service принадлежит user + category |
| Web auth | `internal/app/web/account_auth.go` | brand-bound token + `ValidateWebAccountUser` |
| Email | `internal/email/sender.go` | только login/link; **нет** post-payment email |
| HTTP routes | `internal/app/web/server.go` | topup/order; **нет** payment webhook routes |

Других платёжных систем в runtime-коде (кроме YooKassa, CryptoCloud и SHM Telegram WebApp profile) не найдено.

---

## 4. Сценарии lifecycle

### 4.1 Telegram — покупка новой услуги

```
Telegram callback service_buy / order
→ bot.Service.handleServiceBuy / handleServiceOrder
→ GetServiceByID + orderServiceCategoryAllowed
→ service.ServiceOrder(chatID, serviceID)
→ GetUser (brand membership) → APIClient.ServiceOrder
→ PUT /shm/v1/admin/service/order {user_id, service_id, check_exists_unpaid:1}
→ UI списка услуг (часто NOT PAID)
```

Payment URL на этом шаге **не создаётся**.

Оплата отдельно: кнопка «Оплатить» → `/balance` → сценарий 4.2.

### 4.2 Telegram — продление / оплата существующей услуги

Отдельного renew-handler **нет**.

```
Карточка услуги NOT PAID | BLOCK → кнопка «Оплатить» → /balance
→ bot.Service.handleBalance
→ service.GetUserBalance (через GetUser / brand membership)
→ WebApp URL:
   {API.BaseURL}/shm/v1/public/tg_payments_webapp
     ?format=html&user_id=<shm_user_id>&profile=<PaymentProfile()>
```

Дальше оплата и callback — **вне** `vpnbot` (SHM WebApp / provider).

После возврата в бот отдельного post-payment handler нет; пользователь смотрит баланс/услуги.

### 4.3 Web — покупка из каталога / кабинета

```
POST /api/account/service/order {token, service_id}
→ authenticateWebAccount (brand token + ValidateWebAccountUser)
→ GetServiceByID + ServiceCategoryAllowed + AllowToOrder
→ ServiceOrderByUserID
→ GetUserBalanceByUserID → forecast → amount / needsTopUp
→ response JSON (status created, optional payment_url)
```

Payment URL:

- **EN** + `needsTopUp`: `BuildCryptoCloudPaymentURL(API.BaseURL, userID, amount, ts)`;
- **RU**: `payment_url` пустой; UI ведёт на отдельный balance top-up (YooKassa/CryptoCloud).

`payment_profile` в web-order **не используется**.

### 4.4 Web — пополнение баланса

YooKassa:

```
POST /api/account/balance/topup {token, amount}
→ authenticateWebAccount
→ amount validation (50..10000)
→ BuildYooKassaPaymentURL(API.BaseURL, userID, amount, ts)
→ {payment_url, message}
```

CryptoCloud:

```
POST /api/account/balance/topup/cryptocloud
→ аналогично → BuildCryptoCloudPaymentURL(...)
```

Query CGI (code-confirmed, `internal/payments/shm.go`):

- `action=create`
- `user_id`
- `ts`
- `ps=yookassa|cryptocloud`
- `amount`

Нет: `profile`, `brand_id`, `service_id`, `user_service_id`, `success_url`, `fail_url`, comment/metadata.

### 4.5 YooKassa payment flow

В `vpnbot`:

1. построение URL на `{API.BaseURL}/shm/pay_systems/yookassa.cgi?...`;
2. клиент открывает URL;
3. дальнейший provider/SHM flow **отсутствует в репозитории**.

### 4.6 CryptoCloud payment flow

Аналогично YooKassa, path `/shm/pay_systems/cryptocloud.cgi`, `ps=cryptocloud`.

Дополнительно: EN catalog order может сразу вернуть CryptoCloud `payment_url`.

### 4.7 Успешный callback

**Отсутствует в сценарии `vpnbot`.**

В репозитории нет HTTP handler для YooKassa/CryptoCloud/SHM payment notification.

Ожидаемое поведение (из UI-копирайта, не из callback-кода): SHM зачисляет баланс и активирует неоплаченные услуги. Это **manual/config verification**.

### 4.8 Неуспешный / отменённый платёж

В `vpnbot` нет fail-callback handler.

История платежей читается через `GetUserPays` / `serveAccountPayments`; отменённые записи с `PaySystemID` вроде `yookassa-canceled` и нулевой суммой скрываются (`models.VisibleUserPays`). Это отображение, не обработка callback.

### 4.9 Повторный callback одного платежа

**Отсутствует в сценарии `vpnbot`.** Идемпотентность должна обеспечиваться SHM/provider — **manual/config verification**.

### 4.10 Callback с неверными / подменёнными идентификаторами

**Отсутствует в сценарии `vpnbot`.** Подмена `user_id`/`service_id` на этапе create-URL ограничена тем, что URL строится сервером после auth; сам CGI/callback не валидируется этим кодом.

### 4.11 Callback, относящийся к услуге другой category

**Отсутствует в сценарии `vpnbot`.** На этапе create web-платёж вообще не привязан к `service_id`/`category` — только к `user_id` и сумме. Активация чужой category после top-up — вопрос поведения SHM на балансе пользователя (**conditional risk**).

### 4.12 Возврат по success/fail URL

**Отсутствует в сценарии `vpnbot`.**

`buildSHMPaymentURL` не добавляет success/fail/return URL.
Google OAuth callback (`/api/account/google/callback`) к платежам не относится.

Пользовательский UX: сообщение «вернитесь в кабинет и обновите баланс» (`accountBalanceTopupMessage` / crypto message).

### 4.13 Письмо / Telegram после оплаты

**Post-payment email отсутствует** (`internal/email/sender.go` содержит только login/link).

**Post-payment Telegram notify отсутствует** (lead/registration notifiers не относятся к оплате).

Есть только pre-payment тексты после создания payment URL.

---

## 5. Матрица brand isolation

| Контрольная точка | VFF | FC | Где задаётся | Где проверяется | Статус |
|-------------------|-----|----|--------------|-----------------|--------|
| active `brand_id` | `vff` | `fc` | explicit `brand` config / profiles | `Config.Normalize`, web tokens, `GetUser` / web membership | подтверждено кодом |
| `payment_profile` | `telegram_bot` | `telegram_friends_connect_bot` | `brand.payment_profile` / deploy profiles | обязателен в `validateExplicitBrand`; runtime read в `handleBalance` | подтверждено конфигурацией + кодом (Telegram only) |
| provider credentials / profile selection | SHM profile `telegram_bot` | SHM profile `telegram_friends_connect_bot` | SHM (вне репо) | Telegram WebApp `profile=`; web CGI **не передаёт** profile | требует production config verification |
| service category | `vpn-mz-test` | `vpn-mz-fc` | `brand.service_category` | `GetServiceByID`, `ServiceCategoryAllowed`, owned user service | подтверждено кодом |
| `service_id` при order | catalog brand category | catalog brand category | request body | `GetServiceByID` + category gate до order | подтверждено кодом |
| `user_service_id` | owned + category | owned + category | request / callback UI | `GetOwnedUserServiceByUserID` / Telegram wrapper | подтверждено кодом (для manage/delete/connect; не для payment CGI) |
| `user_id` в payment URL | SHM id из account/Telegram user | аналогично | server-built after auth | auth + brand membership до build URL | подтверждено кодом (init); callback — вне репо |
| web account brand membership | brand-bound token + validate | аналогично | M5 identity | `authenticateWebAccount` | подтверждено кодом |
| Telegram user brand membership | `@<chat_id>` + brand | `@fc_<chat_id>` + brand | brand_user rules | `GetUser` before balance/order | подтверждено кодом |
| invoice / order metadata | нет brand metadata в CGI | нет | — | — | подтверждённый разрыв (web metadata) / отсутствует в сценарии callback |
| payment comment | не формируется vpnbot | не формируется | — | — | отсутствует в сценарии |
| callback routing | нет handler | нет handler | SHM/provider | вне репо | требует manual/e2e verification |
| callback signature/authenticity | нет | нет | SHM/provider | вне репо | требует production config verification |
| callback → payment profile | n/a in vpnbot | n/a | SHM | вне репо | требует production config verification |
| callback → active brand | n/a | n/a | — | vpnbot не участвует | требует manual/e2e verification |
| callback → service category | n/a | n/a | SHM activation | вне репо | условный риск |
| callback → user identity | n/a | n/a | SHM `user_id` | вне репо | требует manual/e2e verification |
| success URL | не строится | не строится | возможно SHM profile | вне репо | требует production config verification |
| fail URL | не строится | не строится | возможно SHM profile | вне репо | требует production config verification |
| public base URL | connect.vpn-for-friends.com | connect.friends-connect.club | brand config | host allowlist / pages; **не** payment CGI base | подтверждено кодом (pages); payment base = `API.BaseURL` |
| landing URL | vpn-for-friends.com | friends-connect.club | brand config | не используется payment flow | отсутствует в сценарии payment |
| email sender/name после оплаты | n/a | n/a | — | post-payment email нет | отсутствует в сценарии |
| email content после оплаты | n/a | n/a | — | нет | отсутствует в сценарии |
| Telegram content после оплаты | n/a | n/a | — | нет post-pay notify | отсутствует в сценарии |
| повторная обработка callback | n/a in vpnbot | n/a | SHM/provider | вне репо | требует manual/e2e verification |
| невозможность cross-brand активации | order/list ограничены category | аналогично | brand category | order gates + owned service; activation после pay — SHM | условный риск (activation path) |

---

## 6. Payment profile — детальный разбор

### 6.1 Где читается `brand.payment_profile`

- accessor: `Config.PaymentProfile()` → `EffectiveBrand().PaymentProfile` (`internal/config/brand.go`);
- валидация: пустой profile → ошибка `Normalize` / `validateExplicitBrand`;
- startup summary: `internal/config/brand_summary.go`;
- **единственный payment runtime consumer**: `bot.Service.handleBalance`.

Legacy `config.Payments.Profile` **не** читается `PaymentProfile()` (explicit brand only) — подтверждено `internal/config/brand_test.go`.

### 6.2 Используется ли при каждом создании счёта?

**Нет.**

| Путь | Использует profile? |
|------|---------------------|
| Telegram WebApp top-up | да (`profile=` query) |
| Web YooKassa top-up | нет |
| Web CryptoCloud top-up | нет |
| Web EN service order crypto URL | нет |
| Admin web-order test YooKassa | нет |

### 6.3 Передаётся ли в SHM / provider?

- Telegram: да, как query `profile` в `tg_payments_webapp`.
- Web CGI: нет (только `ps=yookassa|cryptocloud`).
- Admin service order API body: нет `payment_profile`.

### 6.4 Можно ли обойти active brand через request?

Web request JSON содержит `token` + `amount` / `service_id`. Поля profile/brand для оплаты клиент передать не может — URL строится на сервере.

Telegram WebApp URL тоже строится на сервере из `PaymentProfile()`.

### 6.5 Default / fallback profile

В `handleBalance`:

```go
paymentProfile := s.config.PaymentProfile()
if paymentProfile == "" {
    paymentProfile = "telegram_bot"
}
```

Для production runtime с `Normalize()` fallback **недостижим** (пустой profile запрещён).
Тем не менее hardcoded fallback на VFF profile — **code smell / latent FC risk**, если config когда-либо загрузится в обход строгой валидации.

### 6.6 Hardcoded `telegram_bot` / `telegram_friends_connect_bot`

| Место | Тип |
|-------|-----|
| `deploy/brands/*.json` | допустимые profiles |
| `internal/app/bot/service.go` fallback `"telegram_bot"` | runtime hardcode вне profiles |
| tests / docs | допустимо |
| `models.TelegramInfo.Profile` json tag `telegram_bot` | **не** payment profile (поле settings) |

`telegram_friends_connect_bot` вне profiles/tests/docs в production Go-коде **не** захардкожен.

### 6.7 Пустой / неизвестный profile

- пустой: процесс не стартует (`brand.payment_profile is required`);
- неизвестный SHM profile: поведение WebApp — **config-dependent** (ошибка SHM / пустые методы и т.п.); в `vpnbot` проверки существования profile нет.

### 6.8 Связка active brand ↔ payment profile ↔ service category

В коде **нет** таблицы соответствия `vff→telegram_bot→vpn-mz-test` / `fc→telegram_friends_connect_bot→vpn-mz-fc`.

Есть только: «все три поля обязательны в explicit brand» и раздельное использование category (order) / profile (Telegram pay).
Согласованность значений — ответственность конфигурации (**config-dependent**).

---

## 7. Service category и владение услугой

### 7.1 Проверка category до создания платежа / order

| Место | Функция | Условие |
|-------|---------|---------|
| Bot order | `orderServiceCategoryAllowed` → `models.ServiceCategoryAllowed` | до `ServiceOrder` |
| Web order | `ServiceCategoryAllowed(cfgServiceCategory(cfg), svc.Category)` | до `ServiceOrderByUserID` |
| Admin test order | то же | до FindOrCreate/order |
| API GetServiceByID | filter + локальный `ServiceCategoryAllowed` | not found при чужой category |
| API GetUserServices | filter + локальный drop чужой category | list isolation |
| Owned user service | `ownedUserServiceMatches` | user_id + user_service_id + category |

Web **top-up** category не проверяет: платёж не привязан к услуге.

### 7.2 Проверка category при callback

В `vpnbot` **нет** callback → повторной проверки category нет.

### 7.3 Invoice для `service_id` чужой category

Через bot/web/admin handlers: **отклоняется** как `service_not_found` / «Услуга не найдена» до order.

Прямой вызов `APIClient.ServiceOrder` без предварительного `GetServiceByID` category gate теоретически возможен (защита в `ServiceOrder` закомментирована) — см. PAY-MB-03.

### 7.4 `user_service_id` чужого бренда

Manage/delete/connect пути идут через `GetOwnedUserServiceByUserID` → `ErrUserServiceUnavailable` при чужой category / чужом user.

Payment CGI `user_service_id` не принимает.

### 7.5 Принадлежность user_service → user_id

Проверяется в `ownedUserServiceMatches` (`us.UserID == userID && us.ServiceID == userServiceID`).

### 7.6 Принадлежность user → active brand

- Telegram: `GetUser` / brand membership до balance/order;
- Web: `authenticateWebAccount` (token brand + `ValidateWebAccountUser`).

`GetUserBalanceByUserID` / `ServiceOrderByUserID` сами brand не проверяют — полагаются на caller auth.

### 7.7 Может ли callback напрямую заказать/продлить без проверок vpnbot?

Да, архитектурно: activation после оплаты выполняется **вне** `vpnbot`. Этот репозиторий не участвует в повторной валидации на callback.

### 7.8 Полагается ли callback только на metadata create-счёта?

Для web CGI metadata минимальны (`user_id`, `amount`, `ps`, `ts`). Brand/category/service в create-URL отсутствуют — **code-confirmed gap relative to strong brand-bound intents**.

### 7.9 Подмена идентификаторов

На init:

- клиент не задаёт `user_id` в payment URL (берётся из claims/Telegram user);
- `service_id` проверяется category gate;
- чужой account token другого бренда отклоняется.

На callback: вне репозитория.

---

## 8. Callback / webhook / идемпотентность

Для YooKassa, CryptoCloud и SHM Telegram WebApp в `vpnbot`:

| Вопрос | Результат |
|--------|-----------|
| endpoint | отсутствует |
| HTTP method | отсутствует |
| проверка подписи/секрета/IP | отсутствует |
| payload / обязательные поля | отсутствует |
| поиск исходного платежа | отсутствует |
| сумма/валюта | отсутствует |
| payment status | отсутствует |
| user_id / service / category / profile / brand | отсутствует |
| изменение SHM из vpnbot по callback | отсутствует |
| ответ provider | отсутствует |
| повторный callback | отсутствует |
| transaction / idempotency key | отсутствует |
| защита от двойного начисления/заказа в vpnbot | отсутствует (ожидается от SHM) |

Единственный «callback» в HTTP server — Google OAuth, не платёжный.

**Вывод:** идемпотентность и anti-fraud callback — целиком **SHM/provider concern**. M6 не может быть закрыт без отдельной verification вне этого репозитория (или без появления локальных payment intents + webhook в будущем — это открытый архитектурный вопрос, не решение аудита).

---

## 9. URL и брендированный контент

### 9.1 Происхождение URL

| URL | Источник в vpnbot |
|-----|-------------------|
| success URL | не строится |
| fail URL | не строится |
| callback URL | не строится |
| return URL | не строится |
| payment create URL | `cfg.API.BaseURL` + `/shm/pay_systems/...` |
| Telegram pay WebApp | `cfg.API.BaseURL` + `/shm/v1/public/tg_payments_webapp?...` |
| account URL | `brand.public_base_url` (кабинет/link flows; не payment CGI) |
| landing URL | `brand.landing_url` (не payment flow) |

### 9.2 Находки по URL

- Payment create URL строится от **`API.BaseURL` (SHM)**, не от `brand.public_base_url`.
- Если VFF и FC используют общий SHM `API.BaseURL` (типичная multi-process / shared backend модель), web CGI endpoint общий; brand boundary для Telegram — параметр `profile`, для web CGI — фактически отсутствует.
- VFF-oriented hardcoded domains в payment path напрямую не вшиты в URL builders; есть смежный content debt (например `defaultLogoURL` в bot, support copy в session UI) — относится скорее к M7, но влияет на «оплатный» UX.
- Provider profile success/fail URL **нельзя определить по коду репозитория**.

### 9.3 Email после оплаты

Отсутствует. Login/link письма brand-aware, но к payment lifecycle не подключены.

---

## 10. Token/session и cross-brand сценарии

| Сценарий | Блокируется? | Уровень |
|----------|--------------|---------|
| VFF account token → FC payment API | да | `ParseAndVerifyAccountToken` требует `cfgBrandID`; FC runtime отклонит VFF token |
| FC account token → VFF payment API | да | симметрично |
| Telegram user бренда A заказывает category бренда B | да | `GetServiceByID` + `orderServiceCategoryAllowed` / web category gate |
| Существующий payment URL открыт «из другого brand runtime» | частично | URL указывает на SHM CGI + конкретный `user_id`; оплата кредитует этого SHM user независимо от того, какой brand UI «рядом». Это не cross-user hijack через token, но и не brand-bound intent |
| Callback бренда A принят handler бренда B | n/a in vpnbot | общего payment webhook в vpnbot нет; оба процесса не принимают payment callbacks |
| Один payment profile активирует услугу другой category | условно | vpnbot не активирует; SHM auto-activation по балансу пользователя — **manual/config** |
| Callback только по `user_id` без brand/category revalidation | архитектурно да вне vpnbot | web create-URL уже только `user_id`; vpnbot revalidation на callback нет |

Итог: **инициация** платежа/заказа в `vpnbot` brand-isolated через identity + category. **Завершение** платежа и активация — не контролируются этим кодом.

---

## 11. Тестовое покрытие

### 11.1 Существующие tests

| Сценарий | Test/file | Что подтверждает | Чего не подтверждает |
|----------|-----------|------------------|----------------------|
| Сборка YooKassa/CryptoCloud URL | `internal/payments/yookassa_test.go` | query fields, validation amount/user/base | profile/brand/category/callback |
| Web crypto top-up URL | `internal/app/web/account_crypto_payment_test.go` | auth+URL contains cryptocloud/user/amount | payment_profile; FC brand; callback |
| Web order category gate | `internal/app/web/account_brand_test.go` | чужая category → 404 | payment profile; callback activation |
| Bot category gate | `internal/app/bot/service_category_test.go` | deny other category | payment WebApp profile wiring |
| Owned user service | `internal/service/owned_user_service_test.go` | user/category ownership | payment CGI |
| Brand PaymentProfile accessor/validation | `internal/config/brand_test.go`, `vff_explicit_parity_test.go` | required field; no legacy pick | runtime Telegram/web usage matrix |
| Wrong-brand token на payments/topup | `internal/app/web/account_auth_test.go` | 401 before pays/topup | SHM callback |
| Payments history filter | `internal/app/web/account_web_test.go` | hide canceled; no comment leak | provider callback authenticity |
| Catalog/order UX | `account_catalog_order*_test.go`, session page tests | topup endpoints wiring; EN crypto-only UI | e2e money movement |
| API category helper | `internal/infrastructure/api/client_brand_category_test.go` | `expectedServiceCategory` from brand | ServiceOrder body category |

### 11.2 Отсутствующие обязательные tests для M6

На этапе аудита тесты **не добавлялись**. Пробелы:

- VFF bot top-up WebApp использует только `telegram_bot`;
- FC bot top-up WebApp использует только `telegram_friends_connect_bot`;
- web top-up **не** принимает/не подменяет profile из request;
- `service_id` другой category отклоняется до invoice/order (частично есть; нужна явная матрица VFF/FC ids в интеграционных тестах при наличии fixtures);
- callback чужой category отклоняется — **нельзя unit-тестировать в vpnbot без появления handler**;
- callback чужого brand/profile отклоняется — аналогично;
- VFF token не инициирует FC payment (частично есть auth tests; стоит явно для topup/order на FC cfg);
- FC token не инициирует VFF payment;
- success/fail URL относятся к active brand — сейчас не применимо / нужен SHM contract test;
- повторный callback идемпотентен — вне репо;
- сумма/валюта на callback проверяются — вне репо;
- подмена user_id/service_id/user_service_id на payment complete отклоняется — вне репо / частично на init;
- post-payment email использует active brand — email отсутствует;
- fail-closed поведение при пустом `PaymentProfile` в `handleBalance` (вместо fallback на `telegram_bot`);
- `APIClient.ServiceOrder` не заказывает чужую category даже при прямом вызове.

---

## 12. Находки

### PAY-MB-01 — Web payment path игнорирует `brand.payment_profile`

- **Severity:** High
- **Описание:** YooKassa/CryptoCloud create-URL и admin test order не передают payment profile бренда; profile используется только Telegram WebApp.
- **Подтверждение:** code-confirmed (`account_web.go`, `account_balance_crypto.go`, `admin_web_order.go`, `payments/shm.go` vs `bot/service.go`).
- **Сценарии:** 4.3–4.6.
- **Cross-brand следствие:** merchant/credentials/receipt/return URL для web могут быть общими на уровне SHM CGI, даже если Telegram profiles разделены.
- **Кодовое исправление:** вероятно да (если SHM CGI поддерживает profile/аналог).
- **Config verification:** да — как сейчас разделены web pay systems для VFF/FC в SHM.
- **E2E:** да.
- **Направление:** определить SHM-контракт для web create с profile/brand; передавать profile активного бренда либо отдельные CGI/credentials per brand.

### PAY-MB-02 — Hardcoded fallback `telegram_bot` в Telegram balance

- **Severity:** Medium
- **Описание:** при пустом `PaymentProfile()` bot подставляет `telegram_bot`.
- **Подтверждение:** `internal/app/bot/service.go` `handleBalance`.
- **Сценарий:** 4.2.
- **Cross-brand следствие:** FC процесс без валидного profile мог бы открыть VFF payment profile.
- **Кодовое исправление:** да (fail-closed; убрать fallback).
- **Config verification:** нет для production Normalize path.
- **E2E:** желательно после фикса.
- **Направление:** считать пустой profile ошибкой конфигурации/рантайма, не дефолтить на VFF.

### PAY-MB-03 — `APIClient.ServiceOrder` без повторной category-проверки

- **Severity:** Medium
- **Описание:** локальный `GetServiceByID` перед order в API client закомментирован; защита держится на callers.
- **Подтверждение:** `internal/infrastructure/api/client.go` `ServiceOrder`.
- **Сценарий:** 4.1, 4.3.
- **Cross-brand следствие:** будущий caller без gate может заказать услугу чужой category в общий SHM.
- **Кодовое исправление:** да (defense-in-depth).
- **Config/E2E:** unit достаточно для фикса.
- **Направление:** перед PUT выполнять `GetServiceByID` / `ServiceCategoryAllowed` и отказывать единообразно.

### PAY-MB-04 — Web payment intent без brand/service metadata

- **Severity:** Medium
- **Описание:** create-URL содержит только `user_id`/`amount`/`ps`/`ts`; нет `brand_id`, `service_id`, `user_service_id`, comment.
- **Подтверждение:** `internal/payments/shm.go`.
- **Сценарии:** 4.4–4.6, 4.11.
- **Cross-brand следствие:** невозможно в vpnbot доказать привязку оплаты к category/brand на стороне callback; остаётся trust к SHM user balance model.
- **Кодовое исправление:** возможно, если SHM поддерживает metadata; иначе — documented reliance.
- **Config/E2E:** да.
- **Направление:** либо brand-bound payment intent в SHM/metadata, либо явный архитектурный accept «balance top-up is brand-agnostic per SHM user».

### PAY-MB-05 — Payment callbacks отсутствуют в vpnbot

- **Severity:** Informational (scope) / High для закрытия M6 без внешней verification
- **Описание:** нет webhook endpoints, signature checks, idempotency, amount checks, brand/category revalidation.
- **Подтверждение:** routes в `server.go`; отсутствие handlers по поиску.
- **Сценарии:** 4.7–4.11, 4.9.
- **Cross-brand следствие:** безопасность завершения платежа не доказывается кодом vpnbot.
- **Кодовое исправление:** не обязательно, если граница trust — SHM; иначе нужен новый контур.
- **Config/E2E:** обязательно для закрытия M6.
- **Направление:** checklist verification SHM profiles/webhooks; не объявлять M6 done только по unit-тестам vpnbot.

### PAY-MB-06 — Success/fail/return URL не контролируются vpnbot

- **Severity:** Medium (UX/brand) / config-dependent (security)
- **Описание:** код не задаёт return на `brand.public_base_url`.
- **Подтверждение:** отсутствие success/fail в payments package и handlers.
- **Сценарий:** 4.12.
- **Cross-brand следствие:** пользователь может вернуться на URL из SHM profile (возможен VFF domain для FC или наоборот) — **config-dependent**.
- **Кодовое исправление:** только если SHM API позволяет передать return URL.
- **Config verification:** да (settings profiles `telegram_bot` / `telegram_friends_connect_bot` и web pay systems).
- **E2E:** да.

### PAY-MB-07 — Post-payment email/Telegram отсутствуют

- **Severity:** Low / Informational
- **Описание:** нет branded post-payment коммуникаций в коде.
- **Подтверждение:** `internal/email/sender.go`; отсутствие payment notifiers.
- **Сценарий:** 4.13.
- **Cross-brand следствие:** нет утечки чужого бренда через post-pay email (его нет); есть gap относительно DoD «письма после оплаты», если он трактуется как требование vpnbot.
- **Кодовое исправление:** только если продукт требует письма из vpnbot.
- **Config/E2E:** проверить, не шлёт ли SHM свои письма с чужим брендом.

### PAY-MB-08 — Нет кодовой связки brand ↔ profile ↔ category

- **Severity:** Low
- **Описание:** валидируется наличие полей, но не допустимые комбинации.
- **Подтверждение:** `validateExplicitBrand`.
- **Следствие:** ошибочный FC config с `payment_profile=telegram_bot` пройдёт Normalize.
- **Кодовое исправление:** опционально (allowlist per brand id) или ops checklist.
- **Config verification:** да.

### PAY-MB-09 — Payment CGI base = shared `API.BaseURL`

- **Severity:** Informational / conditional risk
- **Описание:** create URL всегда на SHM base, не на brand public host.
- **Подтверждение:** topup handlers используют `cfg.API.BaseURL`.
- **Следствие:** ожидаемо при shared SHM; brand isolation web payments не обеспечивается отдельным hostname vpnbot.
- **Исправление:** обычно не требуется в vpnbot; требуется корректная SHM multi-profile/pay-system конфигурация.

---

## 13. Решения и открытые вопросы

### Подтверждено безопасным

- разные `payment_profile` и `service_category` заданы в deployment profiles VFF/FC;
- `brand.payment_profile` обязателен при старте процесса;
- Telegram balance WebApp передаёт profile активного бренда (при непустом config);
- web/Telegram order отклоняют `service_id` чужой category;
- web top-up/order требуют brand-bound account token + повторную web identity validation;
- Telegram order/balance идут через `GetUser` с brand membership;
- ownership user_service проверяет user_id + category;
- клиент не может передать произвольный payment profile в web API body;
- post-payment email в vpnbot отсутствует → нет code-level brand mix в таких письмах.

### Подтверждённые разрывы

- web payment create **не** использует `payment_profile` (PAY-MB-01);
- payment create metadata без brand/service (PAY-MB-04);
- hardcoded Telegram fallback на `telegram_bot` (PAY-MB-02);
- `ServiceOrder` API без внутренней category revalidation (PAY-MB-03).

### Условные риски

- общие SHM CGI credentials / return URL для web YooKassa/CryptoCloud при shared backend (PAY-MB-01/06/09);
- SHM auto-activation неоплаченной услуги «не той» category на том же `user_id` после top-up (PAY-MB-05/04);
- ошибочно сконфигурированная связка brand/profile/category (PAY-MB-08);
- письма/чеки, которые может слать SHM/provider, с чужим brand name.

### Требует manual/e2e verification

- фактические merchant/credentials для `telegram_bot` vs `telegram_friends_connect_bot`;
- web pay systems YooKassa/CryptoCloud: отдельные ли настройки per brand или общие;
- success/fail URL после оплаты VFF и FC;
- успешный/неуспешный/повторный callback в SHM;
- зачисление баланса только целевому `user_id`;
- активация только услуг category активного бренда;
- sandbox оплата VFF не затрагивает FC identity/services и наоборот;
- EN crypto order + RU card top-up на обоих брендах.

### Открытые архитектурные вопросы

Не решены аудитом (нужно явное продуктовое/архитектурное решение):

1. Должен ли callback / payment intent содержать обязательный `brand_id`?
2. Нужен ли `brand_id` / `payment_profile` в metadata web CGI create?
3. Должен ли vpnbot получать payment webhook и заново проверять category, или граница trust — SHM?
4. Достаточно ли `payment_profile` как boundary для Telegram, если web CGI его не использует?
5. Нужны ли отдельные callback URL на каждый бренд, если callbacks принимает SHM?
6. Нужна ли собственная таблица payment intents / idempotency в vpnbot?
7. Как связывать оплату с конкретной invoice/order при модели «top-up balance → SHM activates unpaid services» и общем SHM backend?

---

## 14. Implementation plan (после аудита)

План опирается на найденные разрывы; лишние компоненты не предлагаются.

1. **Tests на подтверждённые разрывы/ожидания**
   - bot `handleBalance` URL содержит profile из brand config для VFF и FC fixtures;
   - fail-closed без fallback `telegram_bot`;
   - web topup/order не читают profile из request;
   - wrong-brand token матрица для topup/order на cfg `vff`/`fc`;
   - (при фиксе) `APIClient.ServiceOrder` отклоняет чужую category.

2. **Минимальные service/payment guards**
   - убрать/заменить fallback в `handleBalance`;
   - вернуть category check внутрь `APIClient.ServiceOrder` (или service-layer wrapper).

3. **Brand binding payment intent/metadata**
   - после выяснения SHM-контракта: передавать `profile` (или эквивалент) в web create path;
   - если SHM не поддерживает — зафиксировать accepted model «balance top-up per SHM user» и checklist для ops.

4. **Callback validation**
   - не изобретать webhook в vpnbot без решения по вопросу §13;
   - вместо этого: документированный SHM verification checklist (signature, amount, idempotency, user_id, side effects).

5. **Idempotency**
   - подтвердить на стороне SHM; добавлять локальную таблицу intents только если webhook переносится в vpnbot.

6. **Brand-aware URLs/content**
   - проверить/настроить success/fail в SHM profiles на `public_base_url` бренда;
   - content debt support/logo вынести в M7, если не блокирует оплату.

7. **Config verification VFF/FC**
   - сверить production explicit configs: `payment_profile`, `service_category`, `API.BaseURL`, pay systems;
   - не читать secrets в git; проверка только на сервере ops-процессом.

8. **Manual sandbox/e2e**
   - Telegram top-up VFF/FC;
   - Web YooKassa (RU) и CryptoCloud (EN) на обоих брендах;
   - order → top-up → activation only own category;
   - повторный callback / canceled payment;
   - cross-token negative cases.

9. **Production rollout**
   - только после sandbox и необходимых code/config фиксаций; отдельным согласованием.

10. **Roadmap update / закрытие M6**
    - обновить `docs/MULTIBRAND_ROADMAP.md` только когда code gaps закрыты или accepted, и e2e checklist выполнен.

---

## 15. Рекомендуемый следующий шаг

1. Зафиксировать архитектурный accept/reject по вопросу:
   **web CGI должен передавать `payment_profile` (или иной brand marker), или brand boundary для web — только SHM user identity + category на order.**
2. Параллельно добавить unit-тесты на Telegram profile wiring + fail-closed fallback (PAY-MB-02) — минимальный безопасный code follow-up.
3. Собрать ops checklist для SHM payment profiles / webhooks / return URLs без чтения secrets в репозиторий.

Без пунктов 1–3 закрывать M6 нельзя: статически подтверждена изоляция **инициации** заказа/top-up, но не полный end-to-end payment completion path.
