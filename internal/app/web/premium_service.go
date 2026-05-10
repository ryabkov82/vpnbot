package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/remnawave"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type premiumServiceJSON struct {
	ServiceID             int    `json:"service_id"`
	Name                  string `json:"name"`
	Status                string `json:"status"`
	Expire                string `json:"expire"`
	TrafficLimitBytes     int64  `json:"traffic_limit_bytes"`
	TrafficLimitHuman     string `json:"traffic_limit_human"`
	TrafficLimitStrategy  string `json:"traffic_limit_strategy"`
	HWIDDeviceLimit       int    `json:"hwid_device_limit"`
	TrafficUsageAvailable bool   `json:"traffic_usage_available"`
	TrafficUsedBytes      int64  `json:"traffic_used_bytes"`
	TrafficUsedHuman      string `json:"traffic_used_human"`
	TrafficUsedPercent    int    `json:"traffic_used_percent"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writePremiumForbidden(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
}

// premiumAPIApp — минимальный контракт для premium HTTP handlers (в т.ч. тестовый stub).
type premiumAPIApp interface {
	GetUser(userID int64) (*models.User, error)
	GetUserService(serviceID string) (*models.UserService, error)
}

// loadPremiumUserServiceForRequest: service_id + access_token → услуга владельца токена.
// При ошибке пишет ответ в w и возвращает nil, false.
func loadPremiumUserServiceForRequest(w http.ResponseWriter, r *http.Request, cfg *config.Config, app premiumAPIApp) (*models.UserService, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("service_id"))
	if raw == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid service_id")
		return nil, false
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid service_id")
		return nil, false
	}

	tok := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if tok == "" {
		writePremiumForbidden(w)
		return nil, false
	}

	claims, err := ValidatePremiumAccessToken(cfg.PremiumLinkSigningSecret, tok, id)
	if err != nil {
		writePremiumForbidden(w)
		return nil, false
	}

	user, err := app.GetUser(claims.UserID)
	if err != nil {
		log.Printf("api/premium GetUser: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if user == nil {
		writePremiumForbidden(w)
		return nil, false
	}

	us, err := app.GetUserService(strconv.Itoa(id))
	if err != nil {
		log.Printf("api/premium GetUserService: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if us == nil {
		writeJSONError(w, http.StatusNotFound, "service not found")
		return nil, false
	}
	if us.UserID != user.ID {
		writePremiumForbidden(w)
		return nil, false
	}

	return us, true
}

func trafficUsedPercent(used, limit int64) int {
	if limit <= 0 || used < 0 {
		return 0
	}
	p := int((used * 100) / limit)
	if p > 100 {
		return 100
	}
	return p
}

func applyRemnawaveUsage(ctx context.Context, rw *remnawave.Client, us *models.UserService, out *premiumServiceJSON, limitBytes int64, trafficLimitStrategy string, now time.Time) {
	out.TrafficUsageAvailable = false
	out.TrafficUsedBytes = 0
	out.TrafficUsedHuman = ""
	out.TrafficUsedPercent = 0

	if rw == nil {
		return
	}

	strat := strings.TrimSpace(strings.ToUpper(trafficLimitStrategy))
	if strat != "MONTH" {
		if strings.TrimSpace(trafficLimitStrategy) != "" {
			log.Printf("unsupported traffic limit strategy for usage range: %s", trafficLimitStrategy)
		}
		return
	}

	start, end, ok := PremiumBandwidthQueryRange(us, trafficLimitStrategy, now)
	if !ok {
		return
	}

	username := fmt.Sprintf("us_%d", us.ServiceID)
	user, err := rw.GetUserByUsername(ctx, username)
	if err != nil {
		log.Printf("api/premium/service remnawave user %s: %v", username, err)
		return
	}

	stats, err := rw.GetUserBandwidthStats(ctx, user.UUID, start, end)
	if err != nil {
		log.Printf("api/premium/service remnawave bandwidth: %v", err)
		return
	}

	used := stats.UsedBytes
	out.TrafficUsageAvailable = true
	out.TrafficUsedBytes = used
	out.TrafficUsedHuman = BytesHumanRu(used)
	out.TrafficUsedPercent = trafficUsedPercent(used, limitBytes)
}

func servePremiumService(cfg *config.Config, app premiumAPIApp, rw *remnawave.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/premium/service" {
			http.NotFound(w, r)
			return
		}

		log.Printf("api/premium/service: %s %s", r.Method, r.URL.Path)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		us, ok := loadPremiumUserServiceForRequest(w, r, cfg, app)
		if !ok {
			return
		}

		top, err := us.ParseTopConfig()
		if err != nil {
			log.Printf("api/premium/service ParseTopConfig: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if !models.UserServiceTopConfigIsPremium(top, cfg.PremiumSquadName) {
			writePremiumForbidden(w)
			return
		}

		rwCfg := top.Remnawave
		limitBytes := rwCfg.TrafficLimitBytes

		resp := premiumServiceJSON{
			ServiceID:             us.ServiceID,
			Name:                  us.Name,
			Status:                us.Status,
			Expire:                us.Expire,
			TrafficLimitBytes:     limitBytes,
			TrafficLimitHuman:     BytesHumanRu(limitBytes),
			TrafficLimitStrategy:  strings.TrimSpace(rwCfg.TrafficLimitStrategy),
			HWIDDeviceLimit:       rwCfg.HWIDDeviceLimit,
			TrafficUsageAvailable: false,
			TrafficUsedBytes:      0,
			TrafficUsedHuman:      "",
			TrafficUsedPercent:    0,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		applyRemnawaveUsage(ctx, rw, us, &resp, limitBytes, rwCfg.TrafficLimitStrategy, time.Now())

		writeJSON(w, http.StatusOK, resp)
	}
}
