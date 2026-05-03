package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/remnawave"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/service"
)

func servePremiumHappLink(cfg *config.Config, app *service.Service, rw *remnawave.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/premium/happ-link" {
			http.NotFound(w, r)
			return
		}

		log.Printf("api/premium/happ-link: %s %s", r.Method, r.URL.RequestURI())

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		raw := strings.TrimSpace(r.URL.Query().Get("service_id"))
		if raw == "" {
			writeJSONError(w, http.StatusBadRequest, "invalid service_id")
			return
		}
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid service_id")
			return
		}

		us, err := app.GetUserService(strconv.Itoa(id))
		if err != nil {
			log.Printf("api/premium/happ-link GetUserService: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if us == nil {
			writeJSONError(w, http.StatusNotFound, "service not found")
			return
		}

		top, err := us.ParseTopConfig()
		if err != nil {
			log.Printf("api/premium/happ-link ParseTopConfig: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if !models.UserServiceTopConfigIsPremium(top, cfg.PremiumSquadName) {
			writeJSONError(w, http.StatusForbidden, "service is not premium")
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

		const cryptPrefix = "happ://crypt"
		prefix := enc
		if len(enc) > len(cryptPrefix)+6 {
			prefix = enc[:len(cryptPrefix)+6] + "…"
		}
		log.Printf("api/premium/happ-link ok user=%s happ_prefix=%s", username, prefix)

		writeJSON(w, http.StatusOK, map[string]any{
			"happ_link_available": true,
			"happ_link":           enc,
		})
	}
}
