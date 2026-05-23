package web

import (
	_ "embed"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/remnawave"
	"github.com/ryabkov82/vpnbot/internal/service"
)

//go:embed static/premium-connect/index.html
var premiumConnectHTML []byte

func servePremiumConnect(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch path {
	case "/premium-connect", "/premium-connect/",
		"/premium-connect-test", "/premium-connect-test/":
	default:
		http.NotFound(w, r)
		return
	}

	log.Printf("premium-connect: %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(premiumConnectHTML)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(premiumConnectHTML)
}

// Start runs a minimal HTTP server for static premium onboarding (does not block).
func Start(cfg *config.Config, app *service.Service, rw *remnawave.Client) {
	mux := http.NewServeMux()
	h := servePremiumConnect
	mux.HandleFunc("/premium-connect", h)
	mux.HandleFunc("/premium-connect/", h)
	mux.HandleFunc("/premium-connect-test", h)
	mux.HandleFunc("/premium-connect-test/", h)
	buyH := serveBuy
	mux.HandleFunc("/buy", buyH)
	mux.HandleFunc("/buy/", buyH)
	mux.HandleFunc("/api/premium/service", servePremiumService(cfg, app, rw))
	mux.HandleFunc("/api/premium/happ-link", servePremiumHappLink(cfg, app, rw))
	mux.HandleFunc("/api/public/services", servePublicServices(cfg, app))
	sharedLeadRL := newLeadRateLimiter(5, 15*time.Minute, 3, time.Hour)
	accountLoginRL := newLeadRateLimiter(5, 15*time.Minute, 3, time.Hour)
	mux.HandleFunc("/api/public/lead", servePublicLeadWithLimiter(cfg, app, sharedLeadRL))
	mux.HandleFunc("/api/admin/web-order/test", serveAdminWebOrderTest(cfg, app))
	mux.HandleFunc("/api/admin/account/test", serveAdminAccountTest(cfg, app))

	mux.HandleFunc("/account", serveAccount(cfg))
	mux.HandleFunc("/account/", serveAccount(cfg))
	mux.HandleFunc("/account/session", serveAccountSession(cfg))
	mux.HandleFunc("/account/session/", serveAccountSession(cfg))
	mux.HandleFunc("/api/account/login/start", serveAccountLoginStart(cfg, app, accountLoginRL))
	mux.HandleFunc("/api/account/services", serveAccountServices(cfg, app))
	mux.HandleFunc("/api/account/catalog/services", serveAccountCatalogServices(cfg, app))
	mux.HandleFunc("/api/account/service/connect", serveAccountServiceConnect(cfg, app))
	mux.HandleFunc("/api/account/service/order", serveAccountServiceOrder(cfg, app))
	mux.HandleFunc("/api/account/service/delete", serveAccountServiceDelete(cfg, app))
	mux.HandleFunc("/api/account/balance/topup", serveAccountBalanceTopup(cfg, app))

	port := strings.TrimSpace(cfg.WebPort)
	if port == "" {
		port = "8080"
	}
	addr := port
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}

	go func() {
		log.Printf("HTTP server (premium-connect) listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}
