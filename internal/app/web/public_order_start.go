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

type publicOrderStartApp interface {
	GetServices() ([]models.Service, error)
	GetServiceByID(serviceID int) (*models.Service, error)
}

type publicOrderStartRequestJSON struct {
	ServiceID int    `json:"service_id"`
	Email     string `json:"email"`
	Contact   string `json:"contact"`
	Website   string `json:"website"`
}

type publicOrderStartOKJSON struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	PayURL  string `json:"pay_url,omitempty"`
}

// webSalesTokenFlowAvailable — web-sales включён и задан секрет токенов (/buy/pay, order/status).
func webSalesTokenFlowAvailable(cfg *config.Config) bool {
	if cfg == nil || !cfg.WebSales.Enabled {
		return false
	}
	return strings.TrimSpace(cfg.WebSales.OrderTokenSecret) != ""
}

func isAppEnvDevelopment(cfg *config.Config) bool {
	return cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Env), "development")
}

func servePublicOrderStart(cfg *config.Config, app publicOrderStartApp, rl *leadRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/order/start" {
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
		var req publicOrderStartRequestJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		if strings.TrimSpace(req.Website) != "" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
			return
		}

		if !email.IsConfigured(cfg) {
			writeJSONError(w, http.StatusServiceUnavailable, "email_unavailable")
			return
		}

		if req.ServiceID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid_service")
			return
		}

		normEmail, err := webuser.NormalizeEmail(req.Email)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_email")
			return
		}

		if tid := trialBaseServiceID(cfg); tid > 0 && req.ServiceID == tid {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		svc, err := resolveServiceForPublicLead(app, req.ServiceID)
		if err != nil {
			if isServiceNotFoundErr(err) {
				writeJSONError(w, http.StatusNotFound, "service_not_found")
				return
			}
			slog.Error("api/public/order/start resolve service", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "services_unavailable")
			return
		}
		if svc == nil {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		ipKey := clientIPForPublicLead(r)
		if ipKey == "" {
			ipKey = "unknown"
		}
		emailKey := strings.ToLower(strings.TrimSpace(normEmail))

		if !rl.allow(ipKey, emailKey) {
			writeJSONError(w, http.StatusTooManyRequests, "rate_limited")
			return
		}

		ttl := webSalesOrderTokenTTL(cfg)
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		startTok, err := CreateOrderStartToken(secret, normEmail, req.ServiceID, ttl)
		if err != nil {
			slog.Error("api/public/order/start token", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		base := publicOrderBaseURL(cfg, r)
		if base == "" {
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		payURL := base + "/buy/pay?token=" + url.QueryEscape(startTok)

		preview := models.BuildServicePreview(svc)
		svcName := strings.TrimSpace(preview.Title)
		if svcName == "" {
			svcName = "Тариф"
		}
		amount := preview.Cost
		if amount <= 0 {
			amount = svc.Cost
		}
		amountStr := strconv.FormatFloat(amount, 'f', -1, 64)

		if err := email.SendOrderStartEmail(cfg, normEmail, svcName, amountStr, payURL); err != nil {
			if errors.Is(err, email.ErrNotConfigured) {
				writeJSONError(w, http.StatusServiceUnavailable, "email_unavailable")
				return
			}
			slog.Error("api/public/order/start send email", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "email_send_failed")
			return
		}

		out := publicOrderStartOKJSON{
			Status:  "email_sent",
			Message: "Ссылка на оплату отправлена на email",
		}
		if isAppEnvDevelopment(cfg) {
			out.PayURL = payURL
		}
		writeJSON(w, http.StatusOK, out)
	}
}
