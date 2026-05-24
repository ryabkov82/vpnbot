# vpnbot

## Web: витрина и личный кабинет

- **`/buy`** — публичная страница с тарифами (данные через `GET /api/public/services`). Она не создаёт заказ и не отправляет magic-link оплаты: пользователь переходит в кабинет и оформляет услугу там.
- **`/account`** и API под префиксом **`/api/account/*`** — вход по email (magic-link), каталог, заказ услуги, пополнение баланса, **история платежей (`GET /api/account/payments`)**, подключение и отмена неактивной услуги.

Поле `web_sales.enabled` в конфиге оставлено для совместимости со старыми файлами настроек; текущее приложение на него не опирается. Для работы кабинета задаётся `web_sales.order_token_secret` и при необходимости `web_sales.public_base_url` (базовый URL для ссылок в письмах).

Альтернативный вход через Google настраивается секцией **`web_account`** (отдельно от `web_sales` / оплат):
- **`web_account.google_enabled`** — включает кнопку «Войти с Google» на `/account`.
- **`web_account.google_client_id`**, **`web_account.google_client_secret`**, **`web_account.google_redirect_url`** — учётные данные OAuth-приложения Google. Если `google_enabled=true`, но не задан хотя бы один из трёх параметров или они пустые, SSO считается выключенным. Секрет **не помещать в git** и не логировать; указывайте его только на production-хостах.
- Типичное значение `google_redirect_url` — публичный URL backend’а вида `https://<ваш домен>/api/account/google/callback` (должен совпадать с разрешённым redirect URI в Google Cloud Console).

Premium / AntiBlock в каталоге и в списке услуг отражаются полями `tier`, `connect_app`, `badges`; определение такое же, как в Telegram-боте (`premium_squad_name` из конфигурации бота и соответствующее поле в конфигурации услуги). Активный Premium подключается через `premium_connect_base_url` и подписанный токен (приложение Happ); активный обычный VPN Marzban — через `subscription_url`.

### Favicon, Nginx и Google OAuth API

Браузер запрашивает иконки с корня сайта: **`/favicon.ico`**, **`/favicon-32x32.png`**, **`/apple-touch-icon.png`**. Их отдаёт то же Go-приложение, что **`/account`** (см. mux в `internal/app/web/server.go`). За обратным прокси (Nginx) эти пути нужно проксировать на тот же upstream, что и кабинет, иначе вкладка может остаться без иконки. Файлы лежат в `internal/app/web/static/` и вшиты через `embed` в бинарник; пересобрать их можно локально из `logobot.jpg` (например, временным venv + Pillow: кроп по центру до квадрата, размеры 16×16 и 32×32 внутри ICO, PNG 32 и 180).

За reverse proxy нужно пробрасывать те же префиксы, что использует приложение (**`/buy`**, **`/account`**, в том числе **`/account/link`** и **`/account/link/confirm`**, **`/api/public/*`**, **`/api/account/*`**, включая **`/api/account/payments`** и **`/api/account/link/login/start`**), чтобы кабинет и оплата работали через один домен с HTTPS.

### Связка Telegram-бота и web-кабинета

Зарегистрированный пользователь может открыть **«Личный кабинет»** в главном меню бота (если заданы `web_sales.public_base_url` и `web_sales.order_token_secret`). Бот подписывает одноразовую ссылку `GET /account/link?token=...` (TTL по умолчанию **30 минут**, настройка **`web_sales.telegram_link_token_ttl_minutes`**). После подтверждения того же email через Google или письмо (`account_link_email`, TTL **`web_sales.link_confirm_email_ttl_minutes`**, по умолчанию 60 минут) на учётную запись Telegram-пользователя в SHM добавляются **`login2 = web_<hash(email)>`** (тот же стабильный логин, что у чистых web-пользователей как **`login`**) и блок **`settings.web`** (email и источник) через **`POST /shm/v1/admin/user`** с телом минимум `user_id`, при необходимости **`login2`**, **`settings`**; вложенный фильтр по `settings.web.email` для поиска на стороне SHM **не используется** — поиск аккаунта по email выполняется запросами `GET /shm/v1/admin/user?filter={"login":"web_…"}` и при отсутствии — **`{"login2":"web_…"}`**. Подписанные одноразовые токены не логировать и не кешировать поисковиками; не передавайте содержимое ссылок в открытые редиректы. Если этот **`web_`**-код уже занят другим пользователем (по `login` или `login2`), автоматической перепривязки нет — пользователю показывается сообщение обращения в поддержку.

Для production также проксируйте на тот же backend маршруты **`/api/account/google/start`** и **`/api/account/google/callback`** (если включён вход через Google), например:

```nginx
location = /api/account/google/start {
    proxy_pass http://127.0.0.1:8081;
    proxy_http_version 1.1;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 10s;
}

location = /api/account/google/callback {
    proxy_pass http://127.0.0.1:8081;
    proxy_http_version 1.1;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 10s;
}
```
