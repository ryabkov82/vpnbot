package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/email"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// accountWebApp — кабинет (тесты через stub).
type accountWebApp interface {
	GetUserByLogin(login string) (*models.User, error)
	GetUserServicesByUserID(userID int) ([]models.UserService, error)
	GetUserService(serviceID string) (*models.UserService, error)
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

		login := webuser.WebLoginFromEmail(normEmail)
		user, err := app.GetUserByLogin(login)
		if err != nil {
			slog.Error("account login start: GetUserByLogin", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if user == nil {
			writeJSON(w, http.StatusOK, accountLoginStartOKJSON{Status: "email_sent"})
			return
		}

		if !email.IsConfigured(cfg) {
			writeJSONError(w, http.StatusServiceUnavailable, "email_unavailable")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		tok, err := CreateAccountToken(secret, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
		if err != nil {
			slog.Error("account login start: CreateAccountToken", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		base := strings.TrimRight(strings.TrimSpace(publicOrderBaseURL(cfg, r)), "/")
		if base == "" {
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		loginURL := base + "/account/session?token=" + url.QueryEscape(tok)

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

// accountDashboardCanShowConnect — ACTIVE + vpn-mz-* (без загрузки Marzban в списке).
func accountDashboardCanShowConnect(us models.UserService) bool {
	st := strings.TrimSpace(us.Status)
	if !strings.EqualFold(st, "ACTIVE") {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(us.Category), "vpn-mz-")
}

type accountServicesUserJSON struct {
	Email  string `json:"email"`
	UserID int    `json:"user_id"`
	Login  string `json:"login"`
}

type accountServicesRowJSON struct {
	UserServiceID int    `json:"user_service_id"`
	ServiceID     int    `json:"service_id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Expire        string `json:"expire"`
	Period        string `json:"period"`
	Category      string `json:"category"`
	CanConnect    bool   `json:"can_connect"`
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

		list, err := app.GetUserServicesByUserID(claims.UserID)
		if err != nil {
			slog.Error("account services: GetUserServicesByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		out := make([]accountServicesRowJSON, 0, len(list))
		for i := range list {
			us := &list[i]
			out = append(out, accountServicesRowJSON{
				UserServiceID: us.ServiceID,
				ServiceID:     us.BaseServiceID,
				Name:          us.Name,
				Status:        us.Status,
				Expire:        us.Expire,
				Period:        us.Period,
				Category:      us.Category,
				CanConnect:    accountDashboardCanShowConnect(*us),
			})
		}

		writeJSON(w, http.StatusOK, accountServicesOKJSON{
			User: accountServicesUserJSON{
				Email:  claims.Email,
				UserID: claims.UserID,
				Login:  claims.Login,
			},
			Services: out,
		})
	}
}

const accountConnectTitle = "Открыть подключение"

type accountConnectOKJSON struct {
	Status       string `json:"status"`
	ConnectURL   string `json:"connect_url,omitempty"`
	ConnectTitle string `json:"connect_title,omitempty"`
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
		cat := strings.TrimSpace(us.Category)
		sub := strings.TrimSpace(us.KeyMarzban.SubscriptionURL)

		if strings.EqualFold(st, "ACTIVE") && strings.HasPrefix(cat, "vpn-mz-") && sub != "" {
			writeJSON(w, http.StatusOK, accountConnectOKJSON{
				Status:       "ok",
				ConnectURL:   sub,
				ConnectTitle: accountConnectTitle,
			})
			return
		}

		writeJSON(w, http.StatusOK, accountConnectOKJSON{
			Status:  "not_ready",
			Message: "Подключение пока недоступно",
		})
	}
}
