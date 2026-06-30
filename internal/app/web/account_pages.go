package web

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
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

const accountSessionSupportLinkPlaceholder = "<!--ACCOUNT_SESSION_SUPPORT_LINK_BLOCK-->"

const accountTopupSubmitButtonHTML = `<button type="button" class="btn my-btn w-100" id="topup-submit">Перейти к оплате</button>`

const accountTopupPaymentMethodBlockHTML = `<div class="mb-3" id="topup-payment-methods" role="radiogroup" aria-label="Способ оплаты">
							<div class="small text-secondary mb-2">Способ оплаты</div>
							<div class="row g-2">
								<div class="col-12 col-sm-6">
									<label class="d-block h-100 rounded-3 border border-secondary p-3 bg-body">
										<input class="form-check-input me-2" type="radio" name="topup-payment-method" value="yookassa" checked>
										<span class="fw-semibold">Банковская карта</span>
										<span class="d-block small text-secondary mt-1">Оплата картой через текущий платежный шлюз</span>
									</label>
								</div>
								<div class="col-12 col-sm-6">
									<label class="d-block h-100 rounded-3 border border-secondary p-3 bg-body">
										<input class="form-check-input me-2" type="radio" name="topup-payment-method" value="cryptocloud">
										<span class="fw-semibold">Криптовалюта</span>
										<span class="d-block small text-secondary mt-1">Оплата через Trybit: USDT, TON и другие доступные валюты</span>
									</label>
								</div>
							</div>
							<div class="alert alert-warning py-2 small mt-3 mb-2">При частичной оплате доступ может не активироваться автоматически. Если платеж не зачислился, обратитесь в поддержку.</div>
							<div class="small text-secondary">Поддержка: <a href="https://t.me/friends_connect_support" target="_blank" rel="noopener noreferrer">Telegram @friends_connect_support</a> · <a href="mailto:support@vpn-for-friends.com">support@vpn-for-friends.com</a></div>
						</div>`

const accountTopupPaymentEndpointJSStub = `		function selectedTopupBalanceURL() {
			return '/api/account/balance/topup';
		}`

const accountTopupPaymentEndpointJS = `		function selectedTopupBalanceURL() {
			var picked = document.querySelector('input[name="topup-payment-method"]:checked');
			var method = picked ? String(picked.value || '').trim() : '';
			if (method === 'cryptocloud') {
				return '/api/account/balance/topup/cryptocloud';
			}
			return '/api/account/balance/topup';
		}`

func renderedAccountSessionPageHTML(cfg *config.Config) []byte {
	body := accountSessionPageWithSupportLink(cfg)
	return withAccountTopupPaymentMethods(body)
}

func accountSessionPageWithSupportLink(cfg *config.Config) []byte {
	ph := []byte(accountSessionSupportLinkPlaceholder)
	url := WebCabinetResolvedSupportURL(cfg)
	if url == "" {
		return bytes.ReplaceAll(accountSessionPageHTML, ph, nil)
	}
	block := fmt.Sprintf(`				<a class="btn btn-outline-secondary btn-sm flex-shrink-0" href="%s" target="_blank" rel="noopener noreferrer">Поддержка</a>`,
		template.HTMLEscapeString(url))
	return bytes.ReplaceAll(accountSessionPageHTML, ph, []byte(block))
}

func withAccountTopupPaymentMethods(body []byte) []byte {
	body = bytes.ReplaceAll(body, []byte(accountTopupSubmitButtonHTML), []byte(accountTopupPaymentMethodBlockHTML+"\n\t\t\t\t\t\t"+accountTopupSubmitButtonHTML))
	body = bytes.ReplaceAll(body, []byte(accountTopupPaymentEndpointJSStub), []byte(accountTopupPaymentEndpointJS))
	return body
}

//go:embed static/account/link_invalid.html
var accountLinkInvalidHTML []byte

//go:embed static/account/link_start.html
var accountLinkStartHTML []byte

//go:embed static/account/link_standalone_conflict.html
var accountLinkStandaloneConflictHTML []byte

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
		body := renderedAccountSessionPageHTML(cfg)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
