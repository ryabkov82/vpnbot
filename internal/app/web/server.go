package web

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/remnawave"
	"github.com/ryabkov82/vpnbot/internal/service"
)

// Start runs a minimal HTTP server for static premium onboarding (does not block).
func Start(cfg *config.Config, app *service.Service, rw *remnawave.Client) {
	mux := http.NewServeMux()
	premiumH := servePremiumConnect(cfg)
	mux.HandleFunc("/premium-connect", premiumH)
	mux.HandleFunc("/premium-connect/", premiumH)
	mux.HandleFunc("/premium-connect-test", premiumH)
	mux.HandleFunc("/premium-connect-test/", premiumH)
	mux.HandleFunc("/redirect.html", serveHappRedirect)
	buyH := serveBuy(cfg)
	mux.HandleFunc("/buy", buyH)
	mux.HandleFunc("/buy/", buyH)
	mux.HandleFunc("/favicon.ico", serveEmbeddedAsset("image/x-icon", faviconICO))
	mux.HandleFunc("/favicon-32x32.png", serveEmbeddedAsset("image/png", favicon32PNG))
	mux.HandleFunc("/apple-touch-icon.png", serveEmbeddedAsset("image/png", appleTouchIconPNG))
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
	payReturnH := servePaymentReturn(cfg)
	mux.HandleFunc("/payment/return", payReturnH)
	mux.HandleFunc("/payment/return/", payReturnH)
	linkH := serveAccountLink(cfg, app)
	mux.HandleFunc("/account/link", linkH)
	mux.HandleFunc("/account/link/", linkH)
	mux.HandleFunc("/account/link/confirm", serveAccountLinkConfirm(cfg, app))
	mux.HandleFunc("/account/link/confirm/", serveAccountLinkConfirm(cfg, app))
	mux.HandleFunc("/account/session", serveAccountSession(cfg))
	mux.HandleFunc("/account/session/", serveAccountSession(cfg))
	mux.HandleFunc("/api/account/login/start", serveAccountLoginStart(cfg, app, accountLoginRL))
	mux.HandleFunc("/api/account/link/login/start", serveAccountLinkLoginStart(cfg, app, accountLoginRL))
	msStart := serveGoogleOAuthStart(cfg)
	mux.HandleFunc("/api/account/google/start", msStart)
	mux.HandleFunc("/api/account/google/start/", msStart)
	cb := serveGoogleOAuthCallback(cfg, app)
	mux.HandleFunc("/api/account/google/callback", cb)
	mux.HandleFunc("/api/account/google/callback/", cb)
	mux.HandleFunc("/api/account/session/start", serveAccountSessionStart(cfg, app))
	mux.HandleFunc("/api/account/services", serveAccountServices(cfg, app))
	mux.HandleFunc("/api/account/catalog/services", serveAccountCatalogServices(cfg, app))
	mux.HandleFunc("/api/account/payments", serveAccountPayments(cfg, app))
	mux.HandleFunc("/api/account/service/connect", serveAccountServiceConnect(cfg, app))
	mux.HandleFunc("/api/account/service/order", serveAccountServiceOrder(cfg, app))
	mux.HandleFunc("/api/account/service/delete", serveAccountServiceDelete(cfg, app))
	mux.HandleFunc("/api/account/balance/topup", serveAccountBalanceTopup(cfg, app))
	mux.HandleFunc("/api/account/balance/topup/cryptocloud", serveAccountBalanceTopupCrypto(cfg, app))

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
