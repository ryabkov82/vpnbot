# vpnbot

## Web: витрина и личный кабинет

- **`/buy`** — публичная страница с тарифами (данные через `GET /api/public/services`). Она не создаёт заказ и не отправляет magic-link оплаты: пользователь переходит в кабинет и оформляет услугу там.
- **`/account`** и API под префиксом **`/api/account/*`** — вход по email (magic-link), каталог, заказ услуги, пополнение баланса, подключение и отмена неактивной услуги.

Поле `web_sales.enabled` в конфиге оставлено для совместимости со старыми файлами настроек; текущее приложение на него не опирается. Для работы кабинета задаётся `web_sales.order_token_secret` и при необходимости `web_sales.public_base_url` (базовый URL для ссылок в письмах).

Альтернативный вход через Google настраивается секцией **`web_account`** (отдельно от `web_sales` / оплат):
- **`web_account.google_enabled`** — включает кнопку «Войти с Google» на `/account`.
- **`web_account.google_client_id`**, **`web_account.google_client_secret`**, **`web_account.google_redirect_url`** — учётные данные OAuth-приложения Google. Если `google_enabled=true`, но не задан хотя бы один из трёх параметров или они пустые, SSO считается выключенным. Секрет **не помещать в git** и не логировать; указывайте его только на production-хостах.
- Типичное значение `google_redirect_url` — публичный URL backend’а вида `https://<ваш домен>/api/account/google/callback` (должен совпадать с разрешённым redirect URI в Google Cloud Console).

Premium / AntiBlock в каталоге и в списке услуг отражаются полями `tier`, `connect_app`, `badges`; определение такое же, как в Telegram-боте (`premium_squad_name` из конфигурации бота и соответствующее поле в конфигурации услуги). Активный Premium подключается через `premium_connect_base_url` и подписанный токен (приложение Happ); активный обычный VPN Marzban — через `subscription_url`.

### Favicon, Nginx и Google OAuth API

Браузер запрашивает иконки с корня сайта: **`/favicon.ico`**, **`/favicon-32x32.png`**, **`/apple-touch-icon.png`**. Их отдаёт то же Go-приложение, что **`/account`** (см. mux в `internal/app/web/server.go`). За обратным прокси (Nginx) эти пути нужно проксировать на тот же upstream, что и кабинет, иначе вкладка может остаться без иконки. Файлы лежат в `internal/app/web/static/` и вшиты через `embed` в бинарник; пересобрать их можно локально из `logobot.jpg` (например, временным venv + Pillow: кроп по центру до квадрата, размеры 16×16 и 32×32 внутри ICO, PNG 32 и 180).

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
