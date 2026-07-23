package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/payments"
)

// adminWebOrderApp — контракт для тестового admin web-order (в т.ч. stub в тестах).
type adminWebOrderApp interface {
	GetServiceByID(serviceID int) (*models.Service, error)
	FindOrCreateWebUser(email string) (*models.User, bool, error)
	ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error)
}

type adminWebOrderTestRequestJSON struct {
	Email     string `json:"email"`
	ServiceID int    `json:"service_id"`
}

type adminWebOrderTestOKJSON struct {
	Status            string  `json:"status"`
	UserID            int     `json:"user_id"`
	Login             string  `json:"login"`
	ServiceID         int     `json:"service_id"`
	UserServiceID     int     `json:"user_service_id"`
	UserServiceStatus string  `json:"user_service_status"`
	Amount            float64 `json:"amount"`
	PaymentURL        string  `json:"payment_url"`
}

func adminTrialBaseID(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	if !cfg.Features.Trial.Enabled || cfg.Features.Trial.BaseServiceID <= 0 {
		return 0
	}
	return cfg.Features.Trial.BaseServiceID
}

func adminIsServiceNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// adminTokenMatches сравнивает секреты устойчиво ко времени (через SHA-256).
func adminTokenMatches(cfgToken, hdr string) bool {
	if cfgToken == "" || hdr == "" {
		return false
	}
	a := sha256.Sum256([]byte(cfgToken))
	b := sha256.Sum256([]byte(hdr))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func serveAdminWebOrderTest(cfg *config.Config, app adminWebOrderApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/web-order/test" {
			http.NotFound(w, r)
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}

		wantTok := ""
		if cfg != nil {
			wantTok = cfg.Admin.Token
		}
		if !adminTokenMatches(wantTok, r.Header.Get("X-Admin-Token")) {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req adminWebOrderTestRequestJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		if req.ServiceID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid_service")
			return
		}

		if tid := adminTrialBaseID(cfg); tid > 0 && req.ServiceID == tid {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		svc, err := app.GetServiceByID(req.ServiceID)
		if err != nil {
			if adminIsServiceNotFound(err) {
				writeJSONError(w, http.StatusNotFound, "service_not_found")
				return
			}
			slog.Error("admin web-order test: GetServiceByID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if svc == nil {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}
		// Admin-token не даёт обойти ограничение категории текущего экземпляра приложения:
		// проверяем до создания/поиска пользователя и до заказа.
		if !models.ServiceCategoryAllowed(cfgServiceCategory(cfg), svc.Category) {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		user, _, err := app.FindOrCreateWebUser(req.Email)
		if err != nil {
			slog.Error("admin web-order test: FindOrCreateWebUser", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "web_user_failed")
			return
		}

		order, err := app.ServiceOrderByUserID(user.ID, svc.ServiceID)
		if err != nil {
			slog.Error("admin web-order test: ServiceOrderByUserID", "err", err)
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
		paySystem := ""
		brandID := ""
		if cfg != nil {
			baseURL = cfg.API.BaseURL
			paySystem = cfg.YooKassaPaySystem()
			brandID = cfg.BrandID()
		}
		payURL, err := payments.BuildYooKassaPaymentURL(baseURL, user.ID, amount, time.Now().Unix(), paySystem, brandID)
		if err != nil {
			slog.Error("admin web-order test: BuildYooKassaPaymentURL", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "payment_url_failed")
			return
		}

		writeJSON(w, http.StatusOK, adminWebOrderTestOKJSON{
			Status:            "created",
			UserID:            user.ID,
			Login:             user.Login,
			ServiceID:         svc.ServiceID,
			UserServiceID:     order.ServiceID,
			UserServiceStatus: order.Status,
			Amount:            amount,
			PaymentURL:        payURL,
		})
	}
}
