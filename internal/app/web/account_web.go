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
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// accountWebApp — кабинет (тесты через stub).
type accountWebApp interface {
	GetUserByLogin(login string) (*models.User, error)
	FindOrCreateWebUser(email string) (*models.User, bool, error)
	GetUserServicesByUserID(userID int) ([]models.UserService, error)
	GetUserService(serviceID string) (*models.UserService, error)
	GetUserBalanceByUserID(userID int) (*models.UserBalance, error)
	GetServices() ([]models.Service, error)
	GetServiceByID(serviceID int) (*models.Service, error)
	ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error)
	DeleteUserServiceByUserID(userID int, userServiceID string) error
}

type accountLoginStartRequestJSON struct {
	Email   string `json:"email"`
	Website string `json:"website"`
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

		login := webuser.WebLoginFromEmail(normEmail)
		user, err := app.GetUserByLogin(login)
		if err != nil {
			slog.Error("account login start: GetUserByLogin", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		var magicTok string
		if user != nil {
			magicTok, err = CreateAccountToken(secret, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
		} else {
			magicTok, err = CreateAccountSignupToken(secret, normEmail, login, accountTokenTTL(cfg))
		}
		if err != nil {
			slog.Error("account login start: magic token", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		loginURL := base + "/account/session?token=" + url.QueryEscape(magicTok)

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
		wantLogin := webuser.WebLoginFromEmail(normEmail)
		if strings.TrimSpace(signup.Login) != wantLogin {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}

		existing, err := app.GetUserByLogin(wantLogin)
		if err != nil {
			slog.Error("account session start: GetUserByLogin", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		isNewUser := false
		var user *models.User
		if existing != nil {
			user = existing
		} else {
			u2, created, ferr := app.FindOrCreateWebUser(normEmail)
			if ferr != nil || u2 == nil {
				slog.Error("account session start: FindOrCreateWebUser", "err", ferr)
				writeJSONError(w, http.StatusInternalServerError, "web_user_failed")
				return
			}
			user = u2
			isNewUser = created
		}

		acTok, err := CreateAccountToken(secret, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
		if err != nil {
			slog.Error("account session start: CreateAccountToken", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
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

type accountServicesUserJSON struct {
	Email    string  `json:"email"`
	UserID   int     `json:"user_id"`
	Login    string  `json:"login"`
	Balance  float64 `json:"balance"`
	Forecast float64 `json:"forecast"`
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
}

type accountServicesOKJSON struct {
	User     accountServicesUserJSON  `json:"user"`
	Services []accountServicesRowJSON `json:"services"`
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
			out = append(out, accountServicesRowJSON{
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
			})
		}

		writeJSON(w, http.StatusOK, accountServicesOKJSON{
			User: accountServicesUserJSON{
				Email:    claims.Email,
				UserID:   claims.UserID,
				Login:    claims.Login,
				Balance:  balance,
				Forecast: forecast,
			},
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

		us, err := app.GetUserService(strconv.Itoa(userSvcID))
		if err != nil {
			slog.Error("account connect: GetUserService", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if us == nil {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
		if us.UserID != claims.UserID || us.ServiceID != userSvcID {
			writeJSONError(w, http.StatusForbidden, "forbidden")
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
	accountServiceOrderExistingUnpaidMessage = "У вас уже есть услуга, ожидающая оплаты. Новая услуга не создана. Пополните баланс — после поступления оплаты ожидающая услуга активируется автоматически."
	accountServiceOrderPendingMessage        = "Услуга ожидает оплаты. Пополните баланс — после поступления оплаты услуга активируется автоматически."
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

		out := buildPublicServiceRowsFromList(cfg, list)
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

		preview := models.BuildServicePreview(svc)
		amount := preview.Cost
		if amount <= 0 {
			amount = svc.Cost
		}

		baseURL := ""
		if cfg != nil {
			baseURL = cfg.API.BaseURL
		}
		paymentURL, err := payments.BuildYooKassaPaymentURL(baseURL, claims.UserID, amount, time.Now().Unix())
		if err != nil {
			slog.Error("account service order: BuildYooKassaPaymentURL", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "payment_url_failed")
			return
		}

		orderStatus := strings.TrimSpace(order.Status)
		existingUnpaid := strings.EqualFold(orderStatus, "NOT PAID") && order.BaseServiceID != svc.ServiceID
		msg := accountServiceOrderPendingMessage
		if existingUnpaid {
			msg = accountServiceOrderExistingUnpaidMessage
		}
		retName := strings.TrimSpace(order.Name)
		if retName == "" {
			retName = "Тариф"
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
		us, err := app.GetUserService(usKey)
		if err != nil {
			slog.Error("account service delete: GetUserService", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if us == nil {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
		if us.UserID != claims.UserID || us.ServiceID != req.UserServiceID {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}

		if strings.EqualFold(strings.TrimSpace(us.Status), "ACTIVE") {
			writeJSONError(w, http.StatusConflict, "active_service_cannot_be_deleted")
			return
		}

		if err := app.DeleteUserServiceByUserID(claims.UserID, usKey); err != nil {
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
