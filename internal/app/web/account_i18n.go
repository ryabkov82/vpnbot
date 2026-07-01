package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

type accountLocale string

const (
	accountLocaleRU           accountLocale = "ru"
	accountLocaleEN           accountLocale = "en"
	accountLangCookieName                   = "vff_lang"
	accountLangCookieMaxAge                 = 365 * 24 * 3600
	accountMarketingSiteURLRU               = "https://vpn-for-friends.com/"
	accountMarketingSiteURLEN               = "https://vpn-for-friends.com/en/"
)

// accountI18n holds localized UI copy for the web account cabinet.
type accountI18n struct {
	HTMLLang string

	// Common / header / footer
	PageTitleLogin   string
	PageTitleSession string
	FooterBrand      string
	FooterTagline    string
	SupportBtn       string
	LogoutBtn        string
	LangSwitcherRU   string
	LangSwitcherEN   string
	CloseModal       string

	// Login page
	LoginH1           string
	LoginIntro        string
	LoginTelegramHint string
	LoginLoggedOut    string
	LoginSuccess1     string
	LoginSuccess2     string
	LoginRateLimit    string
	LoginEmailLinked  string
	LoginEmailLabel   string
	LoginSubmitBtn    string
	LoginGoogleOr     string
	LoginGoogleBtn    string
	LoginNetworkError string
	LoginGenericError string

	// Session — no token / loading / errors
	SessionNoToken           string
	SessionNoTokenLink       string
	SessionLoading           string
	SessionInvalidLink       string
	SessionInvalidLinkAction string

	// Dashboard balance
	BalanceLabel        string
	BalanceExplainer    string
	TopUpBalanceBtn     string
	TopUpResultHeading  string
	TopUpResultDetail   string
	TopUpResultFallback string
	RefreshBalanceBtn   string
	OpenPaymentBtn      string

	// Tabs
	TabServices string
	TabBuy      string
	TabPayments string
	TabHelp     string

	// Services tab (static hints in HTML where applicable)
	DashboardTitle string

	// Buy tab
	BuyNewServiceTitle   string
	BuyNewServiceDesc    string
	PostDeleteBuyHint    string
	CatalogPricingNotice string

	// Payments tab
	PaymentsHeading     string
	PaymentsRefreshBtn  string
	PaymentsPlaceholder string

	// Help tab
	HelpHeading string
	HelpStep1   string
	HelpStep2   string
	HelpStep3   string
	HelpStep4   string
	HelpFooter  string

	// Topup modal
	TopUpModalTitle        string
	QuickAmount150         string
	QuickAmount300         string
	QuickAmount450         string
	QuickAmount600         string
	TopUpCustomLabel       string
	TopUpCustomPlaceholder string
	TopUpForecastHint      string
	TopUpNoForecast        string
	TopUpSubmitBtn         string
	TopUpCurrencyNote      string

	// Payment methods (server-rendered block)
	PaymentMethodHeading    string
	PaymentMethodCard       string
	PaymentMethodCardDesc   string
	PaymentMethodCrypto     string
	PaymentMethodCryptoDesc string
	PaymentMethodTrybitWarn string
	PaymentMethodSupport    string
}

func accountMarketingSiteURL(locale accountLocale) string {
	if locale == accountLocaleEN {
		return accountMarketingSiteURLEN
	}
	return accountMarketingSiteURLRU
}

func normalizeAccountLocale(raw string) accountLocale {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "en":
		return accountLocaleEN
	default:
		return accountLocaleRU
	}
}

func resolveAccountLocale(r *http.Request) accountLocale {
	if r != nil {
		if q := strings.TrimSpace(r.URL.Query().Get("lang")); q != "" {
			return normalizeAccountLocale(q)
		}
		if c, err := r.Cookie(accountLangCookieName); err == nil {
			return normalizeAccountLocale(c.Value)
		}
	}
	return accountLocaleRU
}

func setAccountLangCookie(w http.ResponseWriter, r *http.Request, locale accountLocale) {
	if w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     accountLangCookieName,
		Value:    string(locale),
		Path:     "/",
		MaxAge:   accountLangCookieMaxAge,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   r != nil && r.TLS != nil,
	})
}

func accountLocaleHTMLLang(locale accountLocale) string {
	if locale == accountLocaleEN {
		return "en"
	}
	return "ru"
}

func accountLocaleBCP47(locale accountLocale) string {
	if locale == accountLocaleEN {
		return "en-US"
	}
	return "ru-RU"
}

func accountCurrencyDisplay(locale accountLocale) string {
	if locale == accountLocaleEN {
		return "RUB"
	}
	return "₽"
}

func accountLangQuerySuffix(locale accountLocale) string {
	if locale == accountLocaleEN {
		return "lang=en"
	}
	return "lang=ru"
}

func appendAccountLangQuery(rawURL string, locale accountLocale) string {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return u
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + accountLangQuerySuffix(locale)
}

func accountLangSwitchURLs(path string, query map[string][]string, token string) (ruURL, enURL string) {
	base := strings.TrimRight(strings.TrimSpace(path), "/")
	if base == "" {
		base = path
	}
	build := func(loc accountLocale) string {
		q := make(urlValuesCopy, len(query)+2)
		for k, vs := range query {
			if k == "lang" {
				continue
			}
			q[k] = append([]string(nil), vs...)
		}
		if tok := strings.TrimSpace(token); tok != "" {
			q["token"] = []string{tok}
		}
		q["lang"] = []string{string(loc)}
		enc := q.encode()
		if enc == "" {
			return base
		}
		return base + "?" + enc
	}
	return build(accountLocaleRU), build(accountLocaleEN)
}

type urlValuesCopy map[string][]string

func (v urlValuesCopy) encode() string {
	uv := url.Values{}
	for k, vs := range v {
		for _, val := range vs {
			uv.Add(k, val)
		}
	}
	return uv.Encode()
}

func loadAccountI18n(locale accountLocale) accountI18n {
	switch locale {
	case accountLocaleEN:
		return accountI18nEN()
	default:
		return accountI18nRU()
	}
}

func accountI18nRU() accountI18n {
	return accountI18n{
		HTMLLang: "ru",

		PageTitleLogin:   "Личный кабинет — VPN for Friends",
		PageTitleSession: "Кабинет — VPN for Friends",
		FooterBrand:      "VPN for Friends",
		FooterTagline:    "Безопасный доступ к вашим VPN-услугам",
		SupportBtn:       "Поддержка",
		LogoutBtn:        "Выйти",
		LangSwitcherRU:   "RU",
		LangSwitcherEN:   "EN",
		CloseModal:       "Закрыть",

		LoginH1:           "Личный кабинет VPN for Friends",
		LoginIntro:        "Введите email — мы отправим ссылку для входа без пароля.",
		LoginTelegramHint: "Если вы уже пользуетесь Telegram-ботом, откройте в боте команду «Личный кабинет». Так ваши текущие услуги и баланс будут доступны в web-кабинете.",
		LoginLoggedOut:    "Вы вышли из личного кабинета.",
		LoginSuccess1:     "Мы отправили ссылку для входа на указанный email. Откройте письмо и перейдите по ссылке.",
		LoginSuccess2:     "Если письма нет, проверьте папку «Спам» или попробуйте еще раз через пару минут.",
		LoginRateLimit:    "Слишком частые запросы. Попробуйте позже.",
		LoginEmailLinked:  "Этот email уже привязан к другому аккаунту. Войдите с другим email или обратитесь в поддержку.",
		LoginEmailLabel:   "Email",
		LoginSubmitBtn:    "Получить ссылку для входа",
		LoginGoogleOr:     "или",
		LoginGoogleBtn:    "Войти с Google",
		LoginNetworkError: "Сеть недоступна",
		LoginGenericError: "Ошибка",

		SessionNoToken:           "Откройте",
		SessionNoTokenLink:       "страницу входа",
		SessionLoading:           "Загрузка…",
		SessionInvalidLink:       "Ссылка недействительна или устарела.",
		SessionInvalidLinkAction: "Запросить новую ссылку для входа",

		BalanceLabel:        "Баланс:",
		BalanceExplainer:    "Баланс используется для оплаты и автоматического продления услуг.",
		TopUpBalanceBtn:     "Пополнить баланс",
		TopUpResultHeading:  "Страница оплаты открыта в новой вкладке.",
		TopUpResultDetail:   "После оплаты вернитесь в кабинет и обновите баланс. Баланс должен обновиться в течение 1–2 минут.",
		TopUpResultFallback: "Если страница оплаты не открылась автоматически, нажмите «Открыть оплату».",
		RefreshBalanceBtn:   "Обновить баланс",
		OpenPaymentBtn:      "Открыть оплату",

		TabServices: "Мои услуги",
		TabBuy:      "Купить VPN",
		TabPayments: "Платежи",
		TabHelp:     "Помощь",

		DashboardTitle: "Личный кабинет",

		BuyNewServiceTitle: "Купить новую услугу",
		BuyNewServiceDesc:  "Выберите тариф ниже. Оплату можно провести по ссылке — баланс пополнится согласно выбранной сумме, неоплаченная услуга активируется при достатке средств.",
		PostDeleteBuyHint:  "Теперь можно выбрать другой тариф.",

		PaymentsHeading:     "История платежей",
		PaymentsRefreshBtn:  "Обновить",
		PaymentsPlaceholder: "Откройте вкладку, чтобы загрузить историю платежей.",

		HelpHeading: "Как подключить VPN",
		HelpStep1:   "Перейдите во вкладку «Купить VPN» и выберите тариф.",
		HelpStep2:   "Если для активации услуги нужно пополнить баланс, кабинет предложит нужную сумму автоматически. Вы можете изменить сумму вручную.",
		HelpStep3:   "После оплаты услуга появится во вкладке «Мои услуги». Создание услуги может занять 1–2 минуты — страница обновится автоматически.",
		HelpStep4:   "Когда статус станет «Активна», нажмите «Подключить» и следуйте инструкции.",
		HelpFooter:  "Нужна помощь? Нажмите «Поддержка» вверху страницы.",

		TopUpModalTitle:        "Пополнение баланса",
		QuickAmount150:         "150 ₽",
		QuickAmount300:         "300 ₽",
		QuickAmount450:         "450 ₽",
		QuickAmount600:         "600 ₽",
		TopUpCustomLabel:       "Другая сумма: 50–10 000 ₽, до 2 знаков после запятой",
		TopUpCustomPlaceholder: "Например 250",
		TopUpForecastHint:      "Сумма рассчитана по данным биллинга для оплаты/продления услуг.",
		TopUpNoForecast:        "Не удалось рассчитать сумму оплаты. Пополните баланс вручную.",
		TopUpSubmitBtn:         "Перейти к оплате",
		TopUpCurrencyNote:      "",

		PaymentMethodHeading:    "Способ оплаты",
		PaymentMethodCard:       "Банковская карта",
		PaymentMethodCardDesc:   "Оплата картой через текущий платежный шлюз",
		PaymentMethodCrypto:     "Криптовалюта",
		PaymentMethodCryptoDesc: "Оплата через Trybit: USDT, TON и другие доступные валюты",
		PaymentMethodTrybitWarn: "При частичной оплате доступ может не активироваться автоматически. Если платеж не зачислился, обратитесь в поддержку.",
		PaymentMethodSupport:    `Поддержка: <a href="https://t.me/friends_connect_support" target="_blank" rel="noopener noreferrer">Telegram @friends_connect_support</a> · <a href="mailto:support@vpn-for-friends.com">support@vpn-for-friends.com</a>`,
	}
}

func accountI18nEN() accountI18n {
	return accountI18n{
		HTMLLang: "en",

		PageTitleLogin:   "Account — VPN for Friends",
		PageTitleSession: "Account — VPN for Friends",
		FooterBrand:      "VPN for Friends",
		FooterTagline:    "Secure access to your VPN services",
		SupportBtn:       "Support",
		LogoutBtn:        "Sign out",
		LangSwitcherRU:   "RU",
		LangSwitcherEN:   "EN",
		CloseModal:       "Close",

		LoginH1:           "VPN for Friends account",
		LoginIntro:        "Enter your email — we will send a password-free sign-in link.",
		LoginTelegramHint: "If you already use the Telegram bot, open the “Account” command in the bot. This keeps your current services and balance available in the web account.",
		LoginLoggedOut:    "You have signed out.",
		LoginSuccess1:     "We sent a sign-in link to the specified email. Open the email and follow the link.",
		LoginSuccess2:     "If you do not see the email, check Spam or try again in a few minutes.",
		LoginRateLimit:    "Too many requests. Try again later.",
		LoginEmailLinked:  "This email is already linked to another account. Sign in with another email or contact support.",
		LoginEmailLabel:   "Email",
		LoginSubmitBtn:    "Get sign-in link",
		LoginGoogleOr:     "or",
		LoginGoogleBtn:    "Sign in with Google",
		LoginNetworkError: "Network is unavailable",
		LoginGenericError: "Error",

		SessionNoToken:           "Open the",
		SessionNoTokenLink:       "sign-in page",
		SessionLoading:           "Loading…",
		SessionInvalidLink:       "This sign-in link is invalid or expired.",
		SessionInvalidLinkAction: "Request a new sign-in link",

		BalanceLabel:        "Internal balance:",
		BalanceExplainer:    "Balance is maintained in RUB and used for payments and automatic service renewal.",
		TopUpBalanceBtn:     "Top up balance",
		TopUpResultHeading:  "The payment page opened in a new tab.",
		TopUpResultDetail:   "After payment, return to your account and refresh the balance. It should update within 1–2 minutes.",
		TopUpResultFallback: "If the payment page did not open automatically, click “Open payment”.",
		RefreshBalanceBtn:   "Refresh balance",
		OpenPaymentBtn:      "Open payment",

		TabServices: "My services",
		TabBuy:      "Buy VPN",
		TabPayments: "Payments",
		TabHelp:     "Help",

		DashboardTitle: "Account",

		BuyNewServiceTitle:   "Buy a new service",
		BuyNewServiceDesc:    "Choose a VPN plan. We will create a payment link for the selected amount. The service will be activated after payment is completed.",
		PostDeleteBuyHint:    "You can now choose another plan.",
		CatalogPricingNotice: "Prices are shown in USD for convenience. Internal balance is maintained in RUB. The final crypto invoice is calculated from the internal RUB amount by the payment provider.",

		PaymentsHeading:     "Payment history",
		PaymentsRefreshBtn:  "Refresh",
		PaymentsPlaceholder: "Open this tab to load payment history.",

		HelpHeading: "How to connect VPN",
		HelpStep1:   "Open the “Buy VPN” tab and choose a plan.",
		HelpStep2:   "If the service requires a balance top-up, the account will suggest the required amount automatically. You can also enter the amount manually.",
		HelpStep3:   "After payment, the service will appear in “My services”. Service creation may take 1–2 minutes; the page updates automatically.",
		HelpStep4:   "When the status becomes “Active”, click “Connect” and follow the instructions.",
		HelpFooter:  "Need help? Click “Support” at the top of the page.",

		TopUpModalTitle:        "Top up internal balance",
		QuickAmount150:         "150 (≈ $2)",
		QuickAmount300:         "300 (≈ $4)",
		QuickAmount450:         "450 (≈ $6)",
		QuickAmount600:         "600 (≈ $8)",
		TopUpCustomLabel:       "Custom internal amount: 50–10,000",
		TopUpCustomPlaceholder: "For example 250",
		TopUpForecastHint:      "This amount is calculated by the billing system for service payment or renewal.",
		TopUpNoForecast:        "Could not calculate the payment amount. Top up your balance manually.",
		TopUpSubmitBtn:         "Go to payment",
		TopUpCurrencyNote:      "Prices are shown in USD for convenience. Your internal balance is RUB-based. The crypto invoice will show the final equivalent amount on the payment provider page.",

		PaymentMethodHeading:    "Payment method",
		PaymentMethodCard:       "Bank card",
		PaymentMethodCardDesc:   "Card payment via the current payment gateway",
		PaymentMethodCrypto:     "Cryptocurrency",
		PaymentMethodCryptoDesc: "Payment via Trybit: USDT, TON and other available currencies",
		PaymentMethodTrybitWarn: "If the payment is partial, access may not activate automatically. If the payment is not credited, contact support.",
		PaymentMethodSupport:    `Support: <a href="https://t.me/friends_connect_support" target="_blank" rel="noopener noreferrer">Telegram @friends_connect_support</a> · <a href="mailto:support@vpn-for-friends.com">support@vpn-for-friends.com</a>`,
	}
}

func (i accountI18n) jsMessages() map[string]string {
	return map[string]string{
		"buyBtn":                   pickJS(i, "Купить", "Buy"),
		"buyCreating":              pickJS(i, "Создаем...", "Creating..."),
		"buyCreatingService":       pickJS(i, "Создаем услугу…", "Creating service…"),
		"buyAwaitPayment":          pickJS(i, "Ожидает оплаты", "Waiting for payment"),
		"catalogLoadFail":          pickJS(i, "Не удалось загрузить тарифы", "Failed to load plans"),
		"catalogPlanFallback":      pickJS(i, "Тариф", "Plan"),
		"catalogMonthsSuffix":      pickJS(i, " мес.", " mo."),
		"networkError":             pickJS(i, "Сеть недоступна", "Network is unavailable. Check your connection and try again."),
		"networkErrorRetry":        pickJS(i, "Сеть недоступна. Проверьте подключение и попробуйте ещё раз.", "Network is unavailable. Check your connection and try again."),
		"genericError":             pickJS(i, "Ошибка", "Error"),
		"orderError":               pickJS(i, "Ошибка заказа", "Order failed"),
		"connectPopupBlocked":      pickJS(i, "Не удалось открыть страницу подключения. Разрешите всплывающие окна и попробуйте ещё раз.", "Could not open the connection page. Allow pop-ups and try again."),
		"connectLoading":           pickJS(i, "Открываем страницу подключения...", "Opening connection page..."),
		"connectNotReady":          pickJS(i, "Подключение пока недоступно", "Connection is not available yet"),
		"topupAmountRequired":      pickJS(i, "Укажите сумму", "Enter an amount"),
		"topupAmountInvalid":       pickJS(i, "Сумма 50–10 000 ₽, до 2 знаков после запятой", "Amount must be 50–10,000, up to 2 decimal places"),
		"trybitInvoiceFailed":      pickJS(i, "Не удалось создать счет Trybit. Попробуйте позже или обратитесь в поддержку.", "Could not create a crypto payment link. Please try again or contact support."),
		"cryptoPaymentLinkFailed":  pickJS(i, "Не удалось создать ссылку на крипто-оплату. Попробуйте позже или обратитесь в поддержку.", "Could not create a crypto payment link. Please try again or contact support."),
		"paymentInvoiceFailed":     pickJS(i, "Не удалось создать счет на оплату. Попробуйте позже или обратитесь в поддержку.", "Failed to create a payment invoice. Try again later or contact support."),
		"paymentLinkUnavailable":   pickJS(i, "Ссылка на оплату недоступна", "Payment link is not available"),
		"paymentsLoading":          pickJS(i, "Загружаем платежи…", "Loading payments…"),
		"paymentsEmpty":            pickJS(i, "Оплаченных платежей пока нет.", "No paid payments yet."),
		"paymentsLoadFailed":       pickJS(i, "Не удалось загрузить историю платежей. Попробуйте позже.", "Failed to load payment history. Try again later."),
		"signedInAs":               pickJS(i, "Вы вошли как ", "Signed in as "),
		"telegramPrefix":           pickJS(i, "Telegram: ", "Telegram: "),
		"telegramIDPrefix":         pickJS(i, "Telegram: ID ", "Telegram: ID "),
		"serviceFallback":          pickJS(i, "Услуга", "Service"),
		"statusLabel":              pickJS(i, "Статус: ", "Status: "),
		"untilLabel":               pickJS(i, "До: ", "Until: "),
		"connectBtn":               pickJS(i, "Подключить", "Connect"),
		"connectPremiumBtn":        pickJS(i, "Подключить Premium", "Connect Premium"),
		"premiumHappHint":          pickJS(i, "Для Premium используйте приложение Happ.", "For Premium, use the Happ app."),
		"premiumTariffHint":        pickJS(i, "Для сетей с блокировками. Подключение через Happ.", "Premium connection via Happ app."),
		"autorenewHint":            pickJS(i, "Для автопродления заранее пополните баланс.", "Top up your balance in advance for automatic renewal."),
		"notPaidHint1":             pickJS(i, "Пополните баланс — услуга будет активирована автоматически, когда средств будет достаточно.", "Top up your balance — the service will activate automatically when there are enough funds."),
		"notPaidHint2":             pickJS(i, "Если хотите выбрать другой тариф, сначала отмените эту услугу.", "If you want to choose another plan, cancel this service first."),
		"blockedHint":              pickJS(i, "Пополните баланс — услуга будет продлена автоматически, когда средств будет достаточно.", "Top up your balance — the service will renew automatically when there are enough funds."),
		"topUpForActivation":       pickJS(i, "Пополнить для активации", "Top up for activation"),
		"topUpForRenewal":          pickJS(i, "Пополнить для продления", "Top up for renewal"),
		"cancelService":            pickJS(i, "Отменить услугу", "Cancel service"),
		"cancelDeleting":           pickJS(i, "Удаляем...", "Deleting..."),
		"deleteConfirm":            pickJS(i, "Удалить услугу «{name}»? После удаления можно будет выбрать другой тариф.", `Delete service "{name}"? After deletion, you can choose another plan.`),
		"deleteError":              pickJS(i, "Ошибка удаления", "Failed to delete service"),
		"deleteSuccessFallback":    pickJS(i, "Услуга удалена. Теперь можно выбрать другой тариф.", "Service deleted. You can now choose another plan."),
		"progressCreating":         pickJS(i, "Услуга создаётся. Обычно это занимает до 1–2 минут.", "Service is being created. This usually takes 1–2 minutes."),
		"progressDeleting":         pickJS(i, "Услуга удаляется. Обычно это занимает до 1–2 минут.", "Service is being deleted. This usually takes 1–2 minutes."),
		"progressGeneric":          pickJS(i, "Выполняется операция с услугой. Обычно это занимает до 1–2 минут.", "Service operation in progress. This usually takes 1–2 minutes."),
		"progressAutoRefresh":      pickJS(i, "Страница обновится автоматически.", "The page updates automatically."),
		"goToMyServices":           pickJS(i, "Перейти к моим услугам", "Go to my services"),
		"goToPayment":              pickJS(i, "Перейти к оплате", "Go to payment"),
		"dupUnpaidFallback":        pickJS(i, "У вас уже есть услуга, ожидающая оплаты: {name}. Новая выбранная услуга не создана. Пополните баланс — после поступления оплаты ожидающая услуга активируется автоматически.", "You already have a service awaiting payment: {name}. The newly selected service was not created. Top up your balance — the pending service will activate automatically after payment."),
		"neutralUnpaidFallback":    pickJS(i, "Услуга ожидает оплаты. Пополните баланс — после поступления оплаты услуга активируется автоматически.", "The service is awaiting payment. Top up your balance — the service will activate automatically after payment."),
		"svcPayPageOpened":         pickJS(i, "Страница оплаты открыта в новой вкладке. После оплаты вернитесь в кабинет и обновите список услуг.", "The payment page opened in a new tab. After payment, return to your account and refresh the services list."),
		"svcPayAfterPay":           pickJS(i, "После оплаты баланс будет пополнен. Если средств достаточно, услуга активируется автоматически.", "After payment, your balance will be topped up. If funds are sufficient, the service will activate automatically."),
		"svcPayFallback":           pickJS(i, "Если страница оплаты не открылась автоматически, нажмите «Открыть оплату».", "If the payment page did not open automatically, click “Open payment”."),
		"refreshServices":          pickJS(i, "Обновить услуги", "Refresh services"),
		"openPayment":              pickJS(i, "Открыть оплату", "Open payment"),
		"catalogLoading":           pickJS(i, "Загрузка тарифов…", "Loading plans…"),
		"sessionInvalidLink":       pickJS(i, "Ссылка недействительна или устарела.", "This sign-in link is invalid or expired."),
		"sessionInvalidLinkAction": pickJS(i, "Запросить новую ссылку для входа", "Request a new sign-in link"),
		"paymentsPlaceholder":      pickJS(i, "Откройте вкладку, чтобы загрузить историю платежей.", "Open this tab to load payment history."),
		"logoutRedirect":           pickJS(i, "/account?logged_out=1", "/account?logged_out=1&lang=en"),
		"loginPagePath":            pickJS(i, "/account", "/account?lang=en"),
		"errInvalidToken":          pickJS(i, "Недействительная сессия", "Invalid session"),
		"errInvalidAmount":         pickJS(i, "Неверная сумма", "Invalid amount"),
		"errPaymentURLFailed":      pickJS(i, "Не удалось создать ссылку на оплату", "Failed to create payment link"),
		"errRateLimited":           pickJS(i, "Слишком частые запросы", "Too many requests"),
		"errInvalidEmail":          pickJS(i, "Неверный email", "Invalid email"),
		"errEmailUnavailable":      pickJS(i, "Отправка email недоступна", "Email delivery unavailable"),
		"errInternal":              pickJS(i, "Внутренняя ошибка", "Internal error"),
		"errForbidden":             pickJS(i, "Доступ запрещён", "Access denied"),
		"errActiveCannotDelete":    pickJS(i, "Активную услугу нельзя удалить", "Active service cannot be deleted"),
		"errDeleteFailed":          pickJS(i, "Не удалось удалить услугу", "Failed to delete service"),
		"errServiceNotFound":       pickJS(i, "Тариф не найден", "Plan not found"),
		"errOrderFailed":           pickJS(i, "Не удалось создать заказ", "Failed to create order"),
		"errNonJSONResponse":       pickJS(i, "Неожиданный ответ сервера", "Unexpected server response"),
	}
}

func pickJS(i accountI18n, ru, en string) string {
	if i.HTMLLang == "en" {
		return en
	}
	return ru
}

type accountJSConfig struct {
	Lang            string `json:"lang"`
	Currency        string `json:"currency"`
	Locale          string `json:"locale"`
	CurrencyDisplay string `json:"currencyDisplay"`
}

func accountJSConfigForLocale(locale accountLocale) accountJSConfig {
	return accountJSConfig{
		Lang:            string(locale),
		Currency:        "RUB",
		Locale:          accountLocaleBCP47(locale),
		CurrencyDisplay: accountCurrencyDisplay(locale),
	}
}

func marshalAccountJSConfig(locale accountLocale) template.JS {
	b, err := json.Marshal(accountJSConfigForLocale(locale))
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b)
}

func marshalAccountI18nJS(i accountI18n) template.JS {
	b, err := json.Marshal(i.jsMessages())
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b)
}

func accountLoginLoggedOutReplacePath(locale accountLocale) string {
	if locale == accountLocaleEN {
		return "/account?lang=en"
	}
	return "/account"
}

func accountNoTokenLoginURL(locale accountLocale) string {
	if locale == accountLocaleEN {
		return "/account?lang=en"
	}
	return "/account"
}

// accountLoginPageData is passed to the login page template.
type accountLoginPageData struct {
	I18n                 accountI18n
	Locale               accountLocale
	LangSwitchRU         string
	LangSwitchEN         string
	LangRUActive         bool
	LangENActive         bool
	GoogleLoginHTML      template.HTML
	AccountConfigJSON    template.JS
	I18nJSON             template.JS
	LoggedOutReplaceJSON template.JS
	ErrorReplaceJSON     template.JS
	LoginEmailLinkedJSON template.JS
	CurrentLang          string
	NoTokenLoginURL      string
	SiteURL              string
}

// accountSessionPageData is passed to the session page template.
type accountSessionPageData struct {
	I18n                    accountI18n
	Locale                  accountLocale
	LangSwitchRU            string
	LangSwitchEN            string
	LangRUActive            bool
	LangENActive            bool
	NoTokenLoginURL         string
	SupportLinkHTML         template.HTML
	TopupPaymentMethodsHTML template.HTML
	AccountConfigJSON       template.JS
	I18nJSON                template.JS
	BalanceCurrency         string
	SiteURL                 string
}

func buildAccountTopupPaymentMethodsHTML(i accountI18n, locale accountLocale) template.HTML {
	note := ""
	if strings.TrimSpace(i.TopUpCurrencyNote) != "" {
		note = fmt.Sprintf(`<div class="small text-secondary mt-2 mb-0">%s</div>`, template.HTMLEscapeString(i.TopUpCurrencyNote))
	}
	cryptoFirst := locale == accountLocaleEN
	yooChecked := " checked"
	cryptoChecked := ""
	if cryptoFirst {
		yooChecked = ""
		cryptoChecked = " checked"
	}
	cryptoBlock := fmt.Sprintf(`<div class="col-12 col-sm-6">
									<label class="d-block h-100 rounded-3 border border-secondary p-3 bg-body">
										<input class="form-check-input me-2" type="radio" name="topup-payment-method" value="cryptocloud"%s>
										<span class="fw-semibold">%s</span>
										<span class="d-block small text-secondary mt-1">%s</span>
									</label>
								</div>`,
		cryptoChecked,
		template.HTMLEscapeString(i.PaymentMethodCrypto),
		template.HTMLEscapeString(i.PaymentMethodCryptoDesc),
	)
	yooBlock := fmt.Sprintf(`<div class="col-12 col-sm-6">
									<label class="d-block h-100 rounded-3 border border-secondary p-3 bg-body">
										<input class="form-check-input me-2" type="radio" name="topup-payment-method" value="yookassa"%s>
										<span class="fw-semibold">%s</span>
										<span class="d-block small text-secondary mt-1">%s</span>
									</label>
								</div>`,
		yooChecked,
		template.HTMLEscapeString(i.PaymentMethodCard),
		template.HTMLEscapeString(i.PaymentMethodCardDesc),
	)
	methodCols := yooBlock + cryptoBlock
	if cryptoFirst {
		methodCols = cryptoBlock + yooBlock
	}
	block := fmt.Sprintf(`<div class="mb-3" id="topup-payment-methods" role="radiogroup" aria-label="%s">
							<div class="small text-secondary mb-2">%s</div>
							<div class="row g-2">
								%s
							</div>
							<div class="alert alert-warning py-2 small mt-3 mb-2">%s</div>
							<div class="small text-secondary">%s</div>
							%s
						</div>`,
		template.HTMLEscapeString(i.PaymentMethodHeading),
		template.HTMLEscapeString(i.PaymentMethodHeading),
		methodCols,
		template.HTMLEscapeString(i.PaymentMethodTrybitWarn),
		i.PaymentMethodSupport,
		note,
	)
	return template.HTML(block)
}

func buildAccountGoogleLoginHTML(cfg *config.Config, locale accountLocale) template.HTML {
	if !googleOAuthAvailable(cfg) {
		return ""
	}
	i := loadAccountI18n(locale)
	href := appendAccountLangQuery("/api/account/google/start", locale)
	block := fmt.Sprintf(`		<p class="text-center text-secondary small mt-4 mb-2">%s</p>
		<a class="btn btn-outline-light w-100 mb-2" href="%s">%s</a>
`, template.HTMLEscapeString(i.LoginGoogleOr), template.HTMLEscapeString(href), template.HTMLEscapeString(i.LoginGoogleBtn))
	return template.HTML(block)
}

func buildAccountSessionSupportLinkHTML(cfg *config.Config, i accountI18n) template.HTML {
	url := WebCabinetResolvedSupportURL(cfg)
	if url == "" {
		return ""
	}
	block := fmt.Sprintf(`				<a class="btn btn-outline-secondary btn-sm flex-shrink-0" href="%s" target="_blank" rel="noopener noreferrer">%s</a>`,
		template.HTMLEscapeString(url), template.HTMLEscapeString(i.SupportBtn))
	return template.HTML(block)
}
