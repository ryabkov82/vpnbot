package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/email"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/payments"
	appService "github.com/ryabkov82/vpnbot/internal/service"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// cfgServiceCategory — разрешённая категория услуг текущего экземпляра приложения
// (эффективная категория активного бренда). Пустая строка = legacy-поведение без ограничения.
func cfgServiceCategory(cfg *config.Config) string {
	return cfg.ServiceCategory()
}

// accountWebApp — кабинет (тесты через stub).
type accountWebApp interface {
	GetUserByID(userID int) (*models.User, error)
	GetUserByLogin(login string) (*models.User, error)
	FindUserByWebEmail(email string) (*models.User, error)
	FindOrCreateWebUser(email string) (*models.User, bool, error)
	LinkWebEmailForTelegramUser(userID int, telegramChatID int64, email string, source string) (*models.User, error)
	GetUserServicesByUserID(userID int) ([]models.UserService, error)
	GetOwnedUserServiceByUserID(userID int, userServiceID string) (*models.UserService, error)
	GetUserBalanceByUserID(userID int) (*models.UserBalance, error)
	GetUserPaysByUserID(userID int) ([]models.UserPay, error)
	GetServices() ([]models.Service, error)
	GetServiceByID(serviceID int) (*models.Service, error)
	ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error)
	DeleteUserServiceByUserID(userID int, userServiceID string) error
}

type accountLoginStartRequestJSON struct {
	Email   string `json:"email"`
	Website string `json:"website"`
	Lang    string `json:"lang"`
}

type accountLoginStartOKJSON struct {
	Status string `json:"status"`
}

func serveAccountLoginStart(cfg *config.Config, app accountWebApp, rl *leadRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/login/start" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req accountLoginStartRequestJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		if strings.TrimSpace(req.Website) != "" {
			writeJSON(w, http.StatusOK, accountLoginStartOKJSON{Status: "email_sent"})
			return
		}

		normEmail, err := webuser.NormalizeEmail(req.Email)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_email")
			return
		}

		ipKey := ClientIPFromRequest(r)
		if ipKey == "" {
			ipKey = "unknown"
		}
		emailKey := strings.ToLower(normEmail)
		if !rl.allow(ipKey, emailKey) {
			writeJSONError(w, http.StatusTooManyRequests, "rate_limited")
			return
		}

		if !email.IsConfigured(cfg) {
			writeJSONError(w, http.StatusServiceUnavailable, "email_unavailable")
			return
		}

		base := strings.TrimRight(strings.TrimSpace(publicOrderBaseURL(cfg, r)), "/")
		if base == "" {
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)

		linkByEmail, err := app.FindUserByWebEmail(normEmail)
		if err != nil {
			slog.Error("account login start: FindUserByWebEmail", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		login := webuser.WebLoginFromEmailWithPrefix(normEmail, cfg.WebUserLoginPrefix())
		var shmUser *models.User
		if linkByEmail != nil {
			shmUser = linkByEmail
		} else {
			shmUser, err = app.GetUserByLogin(login)
			if err != nil {
				slog.Error("account login start: GetUserByLogin", "err", err)
				writeJSONError(w, http.StatusInternalServerError, "internal_error")
				return
			}
		}

		var magicTok string
		if shmUser != nil {
			magicTok, err = CreateAccountToken(secret, normEmail, shmUser.ID, shmUser.Login, accountTokenTTL(cfg))
		} else {
			magicTok, err = CreateAccountSignupToken(secret, normEmail, login, accountTokenTTL(cfg))
		}
		if err != nil {
			slog.Error("account login start: magic token", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		loginURL := base + "/account/session?token=" + url.QueryEscape(magicTok)
		locale := normalizeAccountLocale(req.Lang)
		if locale == accountLocaleEN {
			loginURL += "&lang=en"
		}

		if err := email.SendAccountLoginEmail(cfg, normEmail, loginURL); err != nil {
			if errors.Is(err, email.ErrNotConfigured) {
				writeJSONError(w, http.StatusServiceUnavailable, "email_unavailable")
				return
			}
			slog.Error("account login start: SendAccountLoginEmail", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "email_send_failed")
			return
		}

		writeJSON(w, http.StatusOK, accountLoginStartOKJSON{Status: "email_sent"})
	}
}

type accountSessionStartReqJSON struct {
	Token string `json:"token"`
}

type accountSessionStartOKJSON struct {
	Status       string `json:"status"`
	AccountToken string `json:"account_token"`
	IsNewUser    bool   `json:"is_new_user"`
}

func serveAccountSessionStart(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/session/start" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req accountSessionStartReqJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		raw := strings.TrimSpace(req.Token)
		if raw == "" {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)

		if _, err := ParseAndVerifyAccountToken(secret, raw); err == nil {
			writeJSON(w, http.StatusOK, accountSessionStartOKJSON{
				Status:       "ok",
				AccountToken: raw,
				IsNewUser:    false,
			})
			return
		}

		signup, err := ParseAndVerifyAccountSignupToken(secret, raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}

		normEmail, err := webuser.NormalizeEmail(signup.Email)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}
		wantLogin := webuser.WebLoginFromEmailWithPrefix(normEmail, cfg.WebUserLoginPrefix())
		if strings.TrimSpace(signup.Login) != wantLogin {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}

		u2, created, ferr := app.FindOrCreateWebUser(normEmail)
		if ferr != nil || u2 == nil {
			slog.Error("account session start: FindOrCreateWebUser", "err", ferr)
			writeJSONError(w, http.StatusInternalServerError, "web_user_failed")
			return
		}
		user := u2
		isNewUser := created

		acTok, err := CreateAccountToken(secret, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
		if err != nil {
			slog.Error("account session start: CreateAccountToken", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		if isNewUser {
			sendAccountUserRegisteredTelegramNotification(cfg, normEmail, user.ID, user.Login, ClientIPFromRequest(r))
		}

		writeJSON(w, http.StatusOK, accountSessionStartOKJSON{
			Status:       "ok",
			AccountToken: acTok,
			IsNewUser:    isNewUser,
		})
	}
}

// accountDashboardCanShowConnect — ACTIVE и (Premium/AntiBlock или стандартный vpn-mz-*).
func accountDashboardCanShowConnect(cfg *config.Config, us models.UserService) bool {
	st := strings.TrimSpace(us.Status)
	if !strings.EqualFold(st, "ACTIVE") {
		return false
	}
	if cfg != nil && models.IsPremiumAntiBlockUserService(&us, cfg.PremiumSquadName) {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(us.Category), "vpn-mz-")
}

// parseDashboardUserCostString читает стоимость из ответа SHM для user_service (поле cost — строка).
func parseDashboardUserCostString(costStr string) float64 {
	s := strings.TrimSpace(strings.ReplaceAll(costStr, ",", "."))
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return math.Round(f*100) / 100
}

// dashboardTariffCostForUserService — сумма для пополнения баланса под тариф: user_service.cost иначе каталог по BaseServiceID.
func dashboardTariffCostForUserService(app accountWebApp, us *models.UserService) float64 {
	if us == nil {
		return 0
	}
	c := parseDashboardUserCostString(us.Cost)
	if c > 0 {
		return c
	}
	if us.BaseServiceID <= 0 {
		return 0
	}
	svc, err := app.GetServiceByID(us.BaseServiceID)
	if err != nil || svc == nil {
		return 0
	}
	preview := models.BuildServicePreview(svc)
	v := preview.Cost
	if v <= 0 {
		v = svc.Cost
	}
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

type accountServicesUserJSON struct {
	Email            string  `json:"email"`
	UserID           int     `json:"user_id"`
	Login            string  `json:"login"`
	Balance          float64 `json:"balance"`
	Forecast         float64 `json:"forecast"`
	TelegramLinked   bool    `json:"telegram_linked"`
	TelegramUsername string  `json:"telegram_username,omitempty"`
	TelegramChatID   int64   `json:"telegram_chat_id,omitempty"`
}

type accountServicesRowJSON struct {
	UserServiceID int      `json:"user_service_id"`
	ServiceID     int      `json:"service_id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Expire        string   `json:"expire"`
	Period        string   `json:"period"`
	Category      string   `json:"category"`
	Tier          string   `json:"tier"`
	ConnectApp    string   `json:"connect_app"`
	Badges        []string `json:"badges"`
	CanConnect    bool     `json:"can_connect"`
	Cost          float64  `json:"cost,omitempty"`
}

type accountServicesOKJSON struct {
	User     accountServicesUserJSON  `json:"user"`
	Services []accountServicesRowJSON `json:"services"`
}

const accountPaymentsLimit = 20

type accountPaymentRowJSON struct {
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	AmountText  string  `json:"amount_text"`
	PaySystemID string  `json:"pay_system_id"`
}

type accountPaymentsOKJSON struct {
	Payments []accountPaymentRowJSON `json:"payments"`
}

func serveAccountPayments(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/payments" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		raw := strings.TrimSpace(r.URL.Query().Get("token"))
		if raw == "" {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		claims, err := ParseAndVerifyAccountToken(secret, raw)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}

		pays, err := app.GetUserPaysByUserID(claims.UserID)
		if err != nil {
			slog.Error("account payments: GetUserPaysByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "payments_failed")
			return
		}

		visible := models.VisibleUserPays(pays)
		if len(visible) > accountPaymentsLimit {
			visible = visible[len(visible)-accountPaymentsLimit:]
		}

		out := make([]accountPaymentRowJSON, 0, len(visible))
		for i := range visible {
			p := visible[i]
			out = append(out, accountPaymentRowJSON{
				Date:        p.Date,
				Amount:      p.Money,
				AmountText:  models.FormatRubAmount(p.Money),
				PaySystemID: p.PaySystemID,
			})
		}

		writeJSON(w, http.StatusOK, accountPaymentsOKJSON{Payments: out})
	}
}

func serveAccountServices(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/services" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		raw := strings.TrimSpace(r.URL.Query().Get("token"))
		if raw == "" {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		claims, err := ParseAndVerifyAccountToken(secret, raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}

		bal, err := app.GetUserBalanceByUserID(claims.UserID)
		if err != nil {
			slog.Error("account services: GetUserBalanceByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "balance_failed")
			return
		}
		var balance, forecast float64
		if bal != nil {
			balance = bal.Balance
			forecast = bal.Forecast
		}

		list, err := app.GetUserServicesByUserID(claims.UserID)
		if err != nil {
			slog.Error("account services: GetUserServicesByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		out := make([]accountServicesRowJSON, 0, len(list))
		for i := range list {
			us := &list[i]
			tier, conn, badges := tierConnectBadgesFromUserService(cfg, us)
			if badges == nil {
				badges = []string{}
			}
			row := accountServicesRowJSON{
				UserServiceID: us.ServiceID,
				ServiceID:     us.BaseServiceID,
				Name:          us.Name,
				Status:        us.Status,
				Expire:        us.Expire,
				Period:        us.Period,
				Category:      us.Category,
				Tier:          tier,
				ConnectApp:    conn,
				Badges:        badges,
				CanConnect:    accountDashboardCanShowConnect(cfg, *us),
			}
			if pay := dashboardTariffCostForUserService(app, us); pay > 0 {
				row.Cost = pay
			}
			out = append(out, row)
		}

		userJSON := accountServicesUserJSON{
			Email:    claims.Email,
			UserID:   claims.UserID,
			Login:    claims.Login,
			Balance:  balance,
			Forecast: forecast,
		}
		if shmUser, errGU := app.GetUserByID(claims.UserID); errGU != nil {
			slog.Error("account services: GetUserByID", "err", errGU)
		} else if shmUser != nil {
			linked, uname, chatID := telegramLinkFieldsFromUser(shmUser)
			userJSON.TelegramLinked = linked
			userJSON.TelegramUsername = uname
			userJSON.TelegramChatID = chatID
		}

		writeJSON(w, http.StatusOK, accountServicesOKJSON{
			User:     userJSON,
			Services: out,
		})
	}
}

const (
	accountConnectTitleStandard = "Открыть подключение"
	accountConnectTitlePremium  = "Открыть Premium подключение"
	accountPremiumHappMessage   = "Для Premium используйте приложение Happ."
)

type accountConnectOKJSON struct {
	Status       string `json:"status"`
	ConnectURL   string `json:"connect_url,omitempty"`
	ConnectTitle string `json:"connect_title,omitempty"`
	ConnectApp   string `json:"connect_app,omitempty"`
	Message      string `json:"message,omitempty"`
}

func serveAccountServiceConnect(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/service/connect" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		rawTok := strings.TrimSpace(r.URL.Query().Get("token"))
		if rawTok == "" {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}
		usidStr := strings.TrimSpace(r.URL.Query().Get("user_service_id"))
		if usidStr == "" {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}
		userSvcID, err := strconv.Atoi(usidStr)
		if err != nil || userSvcID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		claims, err := ParseAndVerifyAccountToken(secret, rawTok)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}

		us, err := app.GetOwnedUserServiceByUserID(claims.UserID, strconv.Itoa(userSvcID))
		if err != nil {
			if errors.Is(err, appService.ErrUserServiceUnavailable) {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
			slog.Error("account connect: GetOwnedUserServiceByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		st := strings.TrimSpace(us.Status)
		if !strings.EqualFold(st, "ACTIVE") {
			writeJSON(w, http.StatusOK, accountConnectOKJSON{
				Status:  "not_ready",
				Message: "Подключение пока недоступно",
			})
			return
		}

		if models.IsPremiumAntiBlockUserService(us, cfg.PremiumSquadName) {
			u, err := BuildPremiumConnectURLForWebAccount(cfg, claims.UserID, us.ServiceID)
			if err != nil || strings.TrimSpace(u) == "" {
				writeJSON(w, http.StatusOK, accountConnectOKJSON{
					Status:     "not_ready",
					ConnectApp: publicConnectHapp,
					Message:    "Подключение Premium пока недоступно. Попробуйте позже или напишите в поддержку.",
				})
				return
			}
			writeJSON(w, http.StatusOK, accountConnectOKJSON{
				Status:       "ok",
				ConnectURL:   u,
				ConnectTitle: accountConnectTitlePremium,
				ConnectApp:   publicConnectHapp,
				Message:      accountPremiumHappMessage,
			})
			return
		}

		cat := strings.TrimSpace(us.Category)
		sub := strings.TrimSpace(us.KeyMarzban.SubscriptionURL)
		if strings.HasPrefix(cat, "vpn-mz-") && sub != "" {
			writeJSON(w, http.StatusOK, accountConnectOKJSON{
				Status:       "ok",
				ConnectURL:   sub,
				ConnectTitle: accountConnectTitleStandard,
				ConnectApp:   publicConnectSubscription,
			})
			return
		}

		writeJSON(w, http.StatusOK, accountConnectOKJSON{
			Status:  "not_ready",
			Message: "Подключение пока недоступно",
		})
	}
}

const accountBalanceTopupMessage = "После оплаты баланс будет пополнен. Если средств достаточно, SHM автоматически активирует неоплаченные услуги или использует баланс для будущего продления."

func accountTopupAmountValid(amount float64) bool {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 50 || amount > 10000 {
		return false
	}
	norm := math.Round(amount*100) / 100
	return math.Abs(amount-norm) < 1e-9
}

// accountOrderPaymentFromSHMForecast нормализует Forecast после создания услуги:
// рекомендованная сумма пополнения (как для /balance/topup), флаг нужна ли оплата, ошибка если forecast > 0, но сумма не проходит те же ограничения 50–10 000 ₽ что и топап.
// Сама ссылка на ЮKassa для заказа услуги не строится — клиент создаёт платеж через POST /api/account/balance/topup.
func accountOrderPaymentFromSHMForecast(forecastRaw float64) (amount float64, needsTopUp bool, invalid bool) {
	f := forecastRaw
	if math.IsNaN(f) || math.IsInf(f, 0) {
		f = 0
	}
	fc := math.Round(f*100) / 100
	if fc <= 0 {
		return 0, false, false
	}
	amt := fc
	if amt < 50 {
		amt = 50
	}
	if !accountTopupAmountValid(amt) {
		return 0, false, true
	}
	return math.Round(amt*100) / 100, true, false
}

type accountBalanceTopupRequestJSON struct {
	Token  string  `json:"token"`
	Amount float64 `json:"amount"`
}

type accountBalanceTopupOKJSON struct {
	Status     string  `json:"status"`
	Amount     float64 `json:"amount"`
	PaymentURL string  `json:"payment_url"`
	Message    string  `json:"message"`
}

func serveAccountBalanceTopup(cfg *config.Config, _ accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/balance/topup" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req accountBalanceTopupRequestJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		raw := strings.TrimSpace(req.Token)
		if raw == "" {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		claims, err := ParseAndVerifyAccountToken(secret, raw)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}

		if !accountTopupAmountValid(req.Amount) {
			writeJSONError(w, http.StatusBadRequest, "invalid_amount")
			return
		}

		baseURL := ""
		if cfg != nil {
			baseURL = cfg.API.BaseURL
		}
		amountRounded := math.Round(req.Amount*100) / 100
		paymentURL, err := payments.BuildYooKassaPaymentURL(baseURL, claims.UserID, amountRounded, time.Now().Unix())
		if err != nil {
			slog.Error("account balance topup: BuildYooKassaPaymentURL", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "payment_url_failed")
			return
		}

		writeJSON(w, http.StatusOK, accountBalanceTopupOKJSON{
			Status:     "payment_required",
			Amount:     amountRounded,
			PaymentURL: paymentURL,
			Message:    accountBalanceTopupMessage,
		})
	}
}

const (
	accountServiceOrderExistingUnpaidMessage   = "У вас уже есть услуга, ожидающая оплаты. Новая услуга не создана. Пополните баланс — после поступления оплаты ожидающая услуга активируется автоматически."
	accountServiceOrderPendingMessage          = "Услуга ожидает оплаты. Пополните баланс — после поступления оплаты услуга активируется автоматически."
	accountServiceOrderCreatedNoPaymentMessage = "Услуга создана. Если средств достаточно, она будет активирована автоматически."
)

func serveAccountCatalogServices(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/catalog/services" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		raw := strings.TrimSpace(r.URL.Query().Get("token"))
		if raw == "" {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		if _, err := ParseAndVerifyAccountToken(secret, raw); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}

		list, err := app.GetServices()
		if err != nil {
			slog.Error("account catalog: GetServices", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "services_unavailable")
			return
		}

		locale := resolveAccountLocale(r)
		out := buildPublicServiceRowsFromList(cfg, list, locale)
		writeJSON(w, http.StatusOK, publicServicesListJSON{Services: out})
	}
}

type accountServiceOrderReqJSON struct {
	Token     string `json:"token"`
	ServiceID int    `json:"service_id"`
}

type accountServiceOrderOKJSON struct {
	Status              string  `json:"status"`
	ServiceID           int     `json:"service_id"`
	UserServiceID       int     `json:"user_service_id"`
	UserServiceStatus   string  `json:"user_service_status"`
	Amount              float64 `json:"amount"`
	PaymentURL          string  `json:"payment_url"`
	Message             string  `json:"message"`
	ExistingUnpaid      bool    `json:"existing_unpaid"`
	RequestedServiceID  int     `json:"requested_service_id"`
	ReturnedServiceID   int     `json:"returned_service_id"`
	ReturnedServiceName string  `json:"returned_service_name"`
}

func serveAccountServiceOrder(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/service/order" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req accountServiceOrderReqJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		rawTok := strings.TrimSpace(req.Token)
		if rawTok == "" {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		claims, err := ParseAndVerifyAccountToken(secret, rawTok)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}

		if req.ServiceID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid_service")
			return
		}

		if tid := trialBaseServiceID(cfg); tid > 0 && req.ServiceID == tid {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		svc, err := app.GetServiceByID(req.ServiceID)
		if err != nil {
			if isServiceNotFoundErr(err) {
				writeJSONError(w, http.StatusNotFound, "service_not_found")
				return
			}
			slog.Error("account service order: GetServiceByID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if svc == nil {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}
		if svc.AllowToOrder != 1 {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}
		// Защитная проверка категории перед заказом (даже если GetServiceByID уже фильтрует):
		// услуга другой категории неотличима от отсутствующей.
		if !models.ServiceCategoryAllowed(cfgServiceCategory(cfg), svc.Category) {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		order, err := app.ServiceOrderByUserID(claims.UserID, svc.ServiceID)
		if err != nil {
			slog.Error("account service order: ServiceOrderByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "order_failed")
			return
		}
		if order == nil {
			writeJSONError(w, http.StatusInternalServerError, "order_failed")
			return
		}

		bal, err := app.GetUserBalanceByUserID(claims.UserID)
		if err != nil {
			slog.Error("account service order: GetUserBalanceByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "balance_failed")
			return
		}
		var forecastRaw float64
		if bal != nil {
			forecastRaw = bal.Forecast
		}
		amount, needsTopUp, badAmt := accountOrderPaymentFromSHMForecast(forecastRaw)
		if badAmt {
			writeJSONError(w, http.StatusBadRequest, "invalid_payment_amount")
			return
		}

		orderStatus := strings.TrimSpace(order.Status)
		existingUnpaid := strings.EqualFold(orderStatus, "NOT PAID") && order.BaseServiceID != svc.ServiceID
		noPaymentNeeded := !needsTopUp
		locale := resolveAccountLocale(r)

		paymentURL := ""
		if locale == accountLocaleEN && needsTopUp && amount > 0 {
			baseURL := ""
			if cfg != nil {
				baseURL = cfg.API.BaseURL
			}
			var err error
			paymentURL, err = payments.BuildCryptoCloudPaymentURL(baseURL, claims.UserID, amount, time.Now().Unix())
			if err != nil {
				slog.Error("account service order: BuildCryptoCloudPaymentURL", "err", err, "user_id", claims.UserID, "amount", amount)
				writeJSONError(w, http.StatusInternalServerError, "crypto_payment_url_failed")
				return
			}
		}

		msg := accountServiceOrderMessage(locale, existingUnpaid, noPaymentNeeded, amount, paymentURL != "")

		retName := strings.TrimSpace(order.Name)
		if retName == "" {
			if locale == accountLocaleEN {
				retName = "Plan"
			} else {
				retName = "Тариф"
			}
		}

		writeJSON(w, http.StatusOK, accountServiceOrderOKJSON{
			Status:              "created",
			ServiceID:           svc.ServiceID,
			UserServiceID:       order.ServiceID,
			UserServiceStatus:   orderStatus,
			Amount:              amount,
			PaymentURL:          paymentURL,
			Message:             msg,
			ExistingUnpaid:      existingUnpaid,
			RequestedServiceID:  req.ServiceID,
			ReturnedServiceID:   order.BaseServiceID,
			ReturnedServiceName: retName,
		})
	}
}

const accountServiceDeletedMessage = "Услуга удалена. Теперь можно выбрать другой тариф."

type accountServiceDeleteReqJSON struct {
	Token         string `json:"token"`
	UserServiceID int    `json:"user_service_id"`
}

type accountServiceDeleteOKJSON struct {
	Status        string `json:"status"`
	UserServiceID int    `json:"user_service_id"`
	Message       string `json:"message"`
}

func serveAccountServiceDelete(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/service/delete" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req accountServiceDeleteReqJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		rawTok := strings.TrimSpace(req.Token)
		if rawTok == "" {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		claims, err := ParseAndVerifyAccountToken(secret, rawTok)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}

		if req.UserServiceID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid_service")
			return
		}

		usKey := strconv.Itoa(req.UserServiceID)
		us, err := app.GetOwnedUserServiceByUserID(claims.UserID, usKey)
		if err != nil {
			if errors.Is(err, appService.ErrUserServiceUnavailable) {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
			slog.Error("account service delete: GetOwnedUserServiceByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		if strings.EqualFold(strings.TrimSpace(us.Status), "ACTIVE") {
			writeJSONError(w, http.StatusConflict, "active_service_cannot_be_deleted")
			return
		}

		if err := app.DeleteUserServiceByUserID(claims.UserID, usKey); err != nil {
			if errors.Is(err, appService.ErrUserServiceUnavailable) {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
			slog.Error("account service delete: DeleteUserServiceByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "delete_failed")
			return
		}

		writeJSON(w, http.StatusOK, accountServiceDeleteOKJSON{
			Status:        "deleted",
			UserServiceID: req.UserServiceID,
			Message:       accountServiceDeletedMessage,
		})
	}
}
