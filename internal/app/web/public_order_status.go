package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type publicOrderStatusApp interface {
	GetUserService(serviceID string) (*models.UserService, error)
}

type publicOrderStatusOKJSON struct {
	Status       string `json:"status"`
	Paid         bool   `json:"paid"`
	Message      string `json:"message"`
	ConnectURL   string `json:"connect_url,omitempty"`
	ConnectTitle string `json:"connect_title,omitempty"`
}

const (
	publicOrderStatusMsgVPNReady        = "Оплата найдена. Можно подключать VPN."
	publicOrderStatusMsgVPNProvisioning = "Оплата найдена. Подписка создается, попробуйте через минуту."
	publicOrderStatusMsgPremiumLater    = "Для Premium подключение будет добавлено отдельно"
	publicOrderStatusMsgUnpaid          = "Ожидаем оплату. После зачисления средств статус станет ACTIVE."
	publicOrderStatusConnectTitle       = "Открыть страницу подключения"
)

func servePublicOrderStatus(cfg *config.Config, app publicOrderStatusApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/order/status" {
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
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		claims, err := ParseAndVerifyOrderToken(secret, raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_token")
			return
		}

		us, err := app.GetUserService(strconv.Itoa(claims.UserServiceID))
		if err != nil {
			slog.Error("api/public/order/status GetUserService", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if us == nil {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}
		if us.UserID != claims.UserID {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
		if us.ServiceID != claims.UserServiceID || us.BaseServiceID != claims.ServiceID {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}

		st := strings.TrimSpace(us.Status)
		paid := strings.EqualFold(st, "ACTIVE")

		out := publicOrderStatusOKJSON{
			Status: st,
			Paid:   paid,
		}
		if !paid {
			out.Message = publicOrderStatusMsgUnpaid
			writeJSON(w, http.StatusOK, out)
			return
		}

		cat := strings.TrimSpace(us.Category)
		if strings.HasPrefix(cat, "vpn-mz-") {
			sub := strings.TrimSpace(us.KeyMarzban.SubscriptionURL)
			if sub != "" {
				out.Message = publicOrderStatusMsgVPNReady
				out.ConnectURL = sub
				out.ConnectTitle = publicOrderStatusConnectTitle
			} else {
				out.Message = publicOrderStatusMsgVPNProvisioning
			}
		} else {
			out.Message = publicOrderStatusMsgPremiumLater
		}

		ttl := webSalesActiveNotifyTTL(cfg)
		if webSalesOrderActiveNotified.tryMarkFirst(claims.UserServiceID, ttl) {
			tariff := strings.TrimSpace(us.Name)
			if tariff == "" {
				tariff = strconv.Itoa(claims.ServiceID)
			}
			connectTG := strings.TrimSpace(out.ConnectURL)
			sendWebOrderActiveTelegramNotification(cfg, claims.Email, tariff, claims.ServiceID, claims.UserID, claims.UserServiceID, connectTG, ClientIPFromRequest(r))
		}

		writeJSON(w, http.StatusOK, out)
	}
}
