package web

import (
	"bytes"
	_ "embed"
	"log"
	"net/http"
	"strconv"

	"github.com/ryabkov82/vpnbot/internal/config"
)

//go:embed static/account/index.html
var accountLoginPageHTML []byte

const accountGoogleLoginPlaceholder = "<!--ACCOUNT_GOOGLE_LOGIN_BLOCK-->"

func renderedAccountLoginPageHTML(cfg *config.Config) []byte {
	ph := []byte(accountGoogleLoginPlaceholder)
	if googleOAuthAvailable(cfg) {
		block := []byte(`		<p class="text-center text-secondary small mt-4 mb-2">или</p>
		<a class="btn btn-outline-light w-100 mb-2" href="/api/account/google/start">Войти с Google</a>
`)
		return bytes.ReplaceAll(accountLoginPageHTML, ph, block)
	}
	return bytes.ReplaceAll(accountLoginPageHTML, ph, nil)
}

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

		body := renderedAccountLoginPageHTML(cfg)
		log.Printf("account/login page: %s", r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
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
