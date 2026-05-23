package web

import (
	_ "embed"
	"log"
	"net/http"
	"strconv"

	"github.com/ryabkov82/vpnbot/internal/config"
)

//go:embed static/account/index.html
var accountLoginPageHTML []byte

//go:embed static/account/session.html
var accountSessionPageHTML []byte

func serveAccount(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account", "/account/":
		default:
			http.NotFound(w, r)
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		log.Printf("account/login page: %s", r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(accountLoginPageHTML)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(accountLoginPageHTML)
	}
}

func serveAccountSession(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/session", "/account/session/":
		default:
			http.NotFound(w, r)
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		log.Printf("account/session page: %s", r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(accountSessionPageHTML)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(accountSessionPageHTML)
	}
}
