package web

import (
	"bytes"
	_ "embed"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/ryabkov82/vpnbot/internal/config"
)

//go:embed static/account/index.html
var accountLoginPageTemplateSrc string

//go:embed static/account/session.html
var accountSessionPageTemplateSrc string

var (
	accountLoginPageTmplOnce sync.Once
	accountLoginPageTmpl     *template.Template
	accountLoginPageTmplErr  error

	accountSessionPageTmplOnce sync.Once
	accountSessionPageTmpl     *template.Template
	accountSessionPageTmplErr  error
)

func accountLoginPageTemplate() (*template.Template, error) {
	accountLoginPageTmplOnce.Do(func() {
		accountLoginPageTmpl, accountLoginPageTmplErr = template.New("account-login").Parse(accountLoginPageTemplateSrc)
	})
	return accountLoginPageTmpl, accountLoginPageTmplErr
}

func accountSessionPageTemplate() (*template.Template, error) {
	accountSessionPageTmplOnce.Do(func() {
		accountSessionPageTmpl, accountSessionPageTmplErr = template.New("account-session").Parse(accountSessionPageTemplateSrc)
	})
	return accountSessionPageTmpl, accountSessionPageTmplErr
}

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

func renderedAccountLoginPageHTML(cfg *config.Config, locale accountLocale) ([]byte, error) {
	tmpl, err := accountLoginPageTemplate()
	if err != nil {
		return nil, err
	}
	i18n := loadAccountI18n(locale)
	ruURL, enURL := accountLangSwitchURLs("/account", nil, "")
	data := accountLoginPageData{
		I18n:                 i18n,
		Locale:               locale,
		LangSwitchRU:         ruURL,
		LangSwitchEN:         enURL,
		LangRUActive:         locale == accountLocaleRU,
		LangENActive:         locale == accountLocaleEN,
		GoogleLoginHTML:      buildAccountGoogleLoginHTML(cfg, locale),
		AccountConfigJSON:    marshalAccountJSConfig(locale),
		I18nJSON:             marshalAccountI18nJS(i18n),
		LoggedOutReplaceJSON: template.JS(strconv.Quote(accountLoginLoggedOutReplacePath(locale))),
		ErrorReplaceJSON:     template.JS(strconv.Quote(accountLoginLoggedOutReplacePath(locale))),
		LoginEmailLinkedJSON: template.JS(strconv.Quote(i18n.LoginEmailLinked)),
		CurrentLang:          string(locale),
		SiteURL:              accountMarketingSiteURL(locale),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderedAccountSessionPageHTML(cfg *config.Config, locale accountLocale, r *http.Request) ([]byte, error) {
	tmpl, err := accountSessionPageTemplate()
	if err != nil {
		return nil, err
	}
	i18n := loadAccountI18n(locale)
	token := ""
	if r != nil {
		token = r.URL.Query().Get("token")
	}
	ruURL, enURL := accountLangSwitchURLs("/account/session", nil, token)
	data := accountSessionPageData{
		I18n:                    i18n,
		Locale:                  locale,
		LangSwitchRU:            ruURL,
		LangSwitchEN:            enURL,
		LangRUActive:            locale == accountLocaleRU,
		LangENActive:            locale == accountLocaleEN,
		NoTokenLoginURL:         accountNoTokenLoginURL(locale),
		SupportLinkHTML:         buildAccountSessionSupportLinkHTML(cfg, i18n),
		TopupPaymentMethodsHTML: buildAccountTopupPaymentMethodsHTML(i18n),
		AccountConfigJSON:       marshalAccountJSConfig(locale),
		I18nJSON:                marshalAccountI18nJS(i18n),
		BalanceCurrency:         accountCurrencyDisplay(locale),
		SiteURL:                 accountMarketingSiteURL(locale),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	body := withAccountTopupPaymentMethods(buf.Bytes())
	return body, nil
}

func withAccountTopupPaymentMethods(body []byte) []byte {
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

		locale := resolveAccountLocale(r)
		if stringsTrimLangQuery(r) {
			setAccountLangCookie(w, r, locale)
		}

		body, err := renderedAccountLoginPageHTML(cfg, locale)
		if err != nil {
			log.Printf("account/login page render: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Printf("account/login page: %s lang=%s", r.URL.Path, locale)
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

		locale := resolveAccountLocale(r)
		if stringsTrimLangQuery(r) {
			setAccountLangCookie(w, r, locale)
		}

		log.Printf("account/session page: %s lang=%s", r.URL.Path, locale)
		body, err := renderedAccountSessionPageHTML(cfg, locale, r)
		if err != nil {
			log.Printf("account/session page render: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func stringsTrimLangQuery(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.URL.Query().Get("lang")) != ""
}
