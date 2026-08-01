package web

import (
	"bytes"
	_ "embed"
	"errors"
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
			var cfg = window.VFF_ACCOUNT || {};
			if (cfg.lang === 'en') {
				return '/api/account/balance/topup/cryptocloud';
			}
			var picked = document.querySelector('input[name="topup-payment-method"]:checked');
			var method = picked ? String(picked.value || '').trim() : '';
			if (method === 'cryptocloud') {
				return '/api/account/balance/topup/cryptocloud';
			}
			return '/api/account/balance/topup';
		}`

func renderedAccountLoginPageHTML(cfg *config.Config, locale accountLocale) ([]byte, error) {
	brandName, landingURL, err := accountBrandIdentity(cfg)
	if err != nil {
		return nil, err
	}
	tmpl, err := accountLoginPageTemplate()
	if err != nil {
		return nil, err
	}
	i18n := loadAccountI18n(locale, brandName)
	ruURL, enURL := accountLangSwitchURLs("/account", nil, "")
	data := accountLoginPageData{
		I18n:                 i18n,
		Locale:               locale,
		BrandID:              cfg.BrandID(),
		LangSwitchRU:         ruURL,
		LangSwitchEN:         enURL,
		LangRUActive:         locale == accountLocaleRU,
		LangENActive:         locale == accountLocaleEN,
		GoogleLoginHTML:      buildAccountGoogleLoginHTML(cfg, locale, i18n),
		AccountConfigJSON:    marshalAccountJSConfig(locale),
		I18nJSON:             marshalAccountI18nJS(i18n),
		LoggedOutReplaceJSON: template.JS(strconv.Quote(accountLoginLoggedOutReplacePath(locale))),
		ErrorReplaceJSON:     template.JS(strconv.Quote(accountLoginLoggedOutReplacePath(locale))),
		LoginEmailLinkedJSON: template.JS(strconv.Quote(i18n.LoginEmailLinked)),
		CurrentLang:          string(locale),
		SiteURL:              landingURL,
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderedAccountSessionPageHTML(cfg *config.Config, locale accountLocale, r *http.Request) ([]byte, error) {
	brandName, landingURL, err := accountBrandIdentity(cfg)
	if err != nil {
		return nil, err
	}
	tmpl, err := accountSessionPageTemplate()
	if err != nil {
		return nil, err
	}
	i18n := loadAccountI18n(locale, brandName)
	token := ""
	if r != nil {
		token = r.URL.Query().Get("token")
	}
	ruURL, enURL := accountLangSwitchURLs("/account/session", nil, token)
	data := accountSessionPageData{
		I18n:                    i18n,
		Locale:                  locale,
		BrandID:                 cfg.BrandID(),
		LangSwitchRU:            ruURL,
		LangSwitchEN:            enURL,
		LangRUActive:            locale == accountLocaleRU,
		LangENActive:            locale == accountLocaleEN,
		NoTokenLoginURL:         accountNoTokenLoginURL(locale),
		SupportLinkHTML:         buildAccountSessionSupportLinkHTML(cfg, i18n),
		TopupPaymentMethodsHTML: buildAccountTopupPaymentMethodsHTML(cfg, i18n, locale),
		AccountConfigJSON:       marshalAccountJSConfig(locale),
		I18nJSON:                marshalAccountI18nJS(i18n),
		BalanceCurrency:         accountCurrencyDisplay(locale),
		SiteURL:                 landingURL,
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
var accountLinkInvalidTemplateSrc string

//go:embed static/account/link_start.html
var accountLinkStartTemplateSrc string

//go:embed static/account/link_standalone_conflict.html
var accountLinkStandaloneConflictTemplateSrc string

var (
	accountLinkInvalidTmplOnce sync.Once
	accountLinkInvalidTmpl     *template.Template
	accountLinkInvalidTmplErr  error

	accountLinkStartTmplOnce sync.Once
	accountLinkStartTmpl     *template.Template
	accountLinkStartTmplErr  error

	accountLinkConflictTmplOnce sync.Once
	accountLinkConflictTmpl     *template.Template
	accountLinkConflictTmplErr  error
)

type accountLinkPageData struct {
	BrandName string
}

const accountLinkStartTokenMarker = "__GO_JS_STRING__"

func accountLinkBrandName(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", errors.New("account link brand name is required")
	}
	name := strings.TrimSpace(cfg.EffectiveBrand().Name)
	if name == "" {
		return "", errors.New("account link brand name is required")
	}
	return name, nil
}

func accountLinkInvalidTemplate() (*template.Template, error) {
	accountLinkInvalidTmplOnce.Do(func() {
		accountLinkInvalidTmpl, accountLinkInvalidTmplErr = template.New("account-link-invalid").Parse(accountLinkInvalidTemplateSrc)
	})
	return accountLinkInvalidTmpl, accountLinkInvalidTmplErr
}

func accountLinkStartTemplate() (*template.Template, error) {
	accountLinkStartTmplOnce.Do(func() {
		accountLinkStartTmpl, accountLinkStartTmplErr = template.New("account-link-start").Parse(accountLinkStartTemplateSrc)
	})
	return accountLinkStartTmpl, accountLinkStartTmplErr
}

func accountLinkConflictTemplate() (*template.Template, error) {
	accountLinkConflictTmplOnce.Do(func() {
		accountLinkConflictTmpl, accountLinkConflictTmplErr = template.New("account-link-conflict").Parse(accountLinkStandaloneConflictTemplateSrc)
	})
	return accountLinkConflictTmpl, accountLinkConflictTmplErr
}

func renderAccountLinkTemplate(tmpl *template.Template, cfg *config.Config) ([]byte, error) {
	brandName, err := accountLinkBrandName(cfg)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, accountLinkPageData{BrandName: brandName}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderedAccountLinkInvalidHTML(cfg *config.Config) ([]byte, error) {
	tmpl, err := accountLinkInvalidTemplate()
	if err != nil {
		return nil, err
	}
	return renderAccountLinkTemplate(tmpl, cfg)
}

func renderedAccountLinkStandaloneConflictHTML(cfg *config.Config) ([]byte, error) {
	tmpl, err := accountLinkConflictTemplate()
	if err != nil {
		return nil, err
	}
	return renderAccountLinkTemplate(tmpl, cfg)
}

func renderedAccountLinkStartHTML(cfg *config.Config, token string) ([]byte, error) {
	tmpl, err := accountLinkStartTemplate()
	if err != nil {
		return nil, err
	}
	body, err := renderAccountLinkTemplate(tmpl, cfg)
	if err != nil {
		return nil, err
	}
	marker := []byte(accountLinkStartTokenMarker)
	if n := bytes.Count(body, marker); n != 1 {
		return nil, errors.New("account link start token marker missing or duplicated")
	}
	return bytes.Replace(body, marker, []byte(strconv.Quote(token)), 1), nil
}

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
