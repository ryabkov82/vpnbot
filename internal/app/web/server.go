package web

import (
	_ "embed"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/remnawave"
	"github.com/ryabkov82/vpnbot/internal/service"
)

//go:embed static/premium-connect-test/index.html
var premiumConnectHTML []byte

func servePremiumConnectTest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path != "/premium-connect-test" && path != "/premium-connect-test/" {
		http.NotFound(w, r)
		return
	}

	log.Printf("premium-connect-test: %s %s", r.Method, r.URL.RequestURI())

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
	h := servePremiumConnectTest
	mux.HandleFunc("/premium-connect-test", h)
	mux.HandleFunc("/premium-connect-test/", h)
	mux.HandleFunc("/api/premium/service", servePremiumService(cfg, app, rw))
	mux.HandleFunc("/api/premium/happ-link", servePremiumHappLink(cfg, app, rw))

	port := strings.TrimSpace(cfg.WebPort)
	if port == "" {
		port = "8080"
	}
	addr := port
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}

	go func() {
		log.Printf("HTTP server (premium-connect-test) listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}
