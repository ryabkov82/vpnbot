package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/remnawave"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func servePremiumHappLink(cfg *config.Config, app premiumAPIApp, rw *remnawave.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/premium/happ-link" {
			http.NotFound(w, r)
			return
		}

		log.Printf("api/premium/happ-link: %s %s", r.Method, r.URL.Path)

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
			log.Printf("api/premium/happ-link ParseTopConfig: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if !models.UserServiceTopConfigIsPremium(top, cfg.PremiumSquadName) {
			writePremiumForbidden(w)
			return
		}

		if rw == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "remnawave unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		username := fmt.Sprintf("us_%d", us.ServiceID)
		sub, err := rw.GetSubscriptionByUsername(ctx, username)
		if err != nil {
			log.Printf("api/premium/happ-link remnawave subscription user=%s: %v", username, err)
			writeJSONError(w, http.StatusBadGateway, "failed to build happ link")
			return
		}

		enc, err := rw.EncryptHappLink(ctx, sub.SubscriptionURL)
		if err != nil {
			log.Printf("api/premium/happ-link remnawave encrypt user=%s: %v", username, err)
			writeJSONError(w, http.StatusBadGateway, "failed to build happ link")
			return
		}

		log.Printf("api/premium/happ-link ok user=%s", username)

		writeJSON(w, http.StatusOK, map[string]any{
			"happ_link_available": true,
			"happ_link":           enc,
		})
	}
}
