package web

import (
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func mustRenderAccountLoginHTML(t *testing.T, cfg *config.Config, locale accountLocale) string {
	t.Helper()
	b, err := renderedAccountLoginPageHTML(cfg, locale)
	if err != nil {
		t.Fatalf("render login: %v", err)
	}
	return string(b)
}

func mustRenderAccountSessionHTML(t *testing.T, cfg *config.Config, locale accountLocale) string {
	t.Helper()
	b, err := renderedAccountSessionPageHTML(cfg, locale, nil)
	if err != nil {
		t.Fatalf("render session: %v", err)
	}
	return string(b)
}

func TestResolveAccountLocale(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/account?lang=en", nil)
	if got := resolveAccountLocale(req); got != accountLocaleEN {
		t.Fatalf("query en: got %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/account?lang=ru", nil)
	if got := resolveAccountLocale(req); got != accountLocaleRU {
		t.Fatalf("query ru: got %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/account?lang=de", nil)
	if got := resolveAccountLocale(req); got != accountLocaleRU {
		t.Fatalf("unknown lang fallback: got %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: accountLangCookieName, Value: "en"})
	if got := resolveAccountLocale(req); got != accountLocaleEN {
		t.Fatalf("cookie en: got %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/account", nil)
	if got := resolveAccountLocale(req); got != accountLocaleRU {
		t.Fatalf("default ru: got %q", got)
	}
}

func TestRenderedAccountLogin_RU(t *testing.T) {
	cfg := orderStartTestCfg()
	html := mustRenderAccountLoginHTML(t, cfg, accountLocaleRU)
	for _, needle := range []string{
		`<html lang="ru"`,
		"Личный кабинет VPN for Friends",
		"Получить ссылку для входа",
		"Введите email — мы отправим ссылку для входа без пароля.",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("RU login missing %q", needle)
		}
	}
}

func TestRenderedAccountLogin_EN(t *testing.T) {
	cfg := orderStartTestCfg()
	html := mustRenderAccountLoginHTML(t, cfg, accountLocaleEN)
	for _, needle := range []string{
		`<html lang="en"`,
		"VPN for Friends account",
		"Get sign-in link",
		"Enter your email — we will send a password-free sign-in link.",
		`/account?lang=en`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("EN login missing %q", needle)
		}
	}
}

func TestRenderedAccountSession_RU(t *testing.T) {
	cfg := orderStartTestCfg()
	html := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	for _, needle := range []string{
		`<html lang="ru"`,
		">Мои услуги</button>",
		">Купить VPN</button>",
		">Платежи</button>",
		"Банковская карта",
		"Криптовалюта",
		"150 ₽",
		"50–10 000 ₽, до 2 знаков после запятой",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("RU session missing %q", needle)
		}
	}
}

func TestRenderedAccountSession_EN(t *testing.T) {
	cfg := orderStartTestCfg()
	html := mustRenderAccountSessionHTML(t, cfg, accountLocaleEN)
	for _, needle := range []string{
		`<html lang="en"`,
		">My services</button>",
		">Buy VPN</button>",
		">Payments</button>",
		">Help</button>",
		"Bank card",
		"Cryptocurrency",
		"150 RUB",
		"Custom amount: 50–10,000 RUB, up to 2 decimal places",
		"Balance is shown in RUB. Crypto payment options may display available currencies on the payment provider page.",
		`"currencyDisplay":"RUB"`,
		"Premium connection via Happ app.",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("EN session missing %q", needle)
		}
	}
	for _, forbid := range []string{
		"bypass", "unblock", "no limits", "unrestricted",
		"invisible", "hide everything", "restricted networks",
	} {
		if strings.Contains(strings.ToLower(html), forbid) {
			t.Fatalf("EN session contains risky word %q", forbid)
		}
	}
	if riskyUserFacingSubstring(html, "anonymous") {
		t.Fatal("EN session contains risky word \"anonymous\" in user-facing copy")
	}
	if !strings.Contains(html, "RUB") && !strings.Contains(html, `"currency":"RUB"`) {
		t.Fatal("EN session must show RUB as account currency")
	}
}

func TestServeAccountLoginStart_LangENMagicLink(t *testing.T) {
	var gotMail []byte
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotMail = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	cfg.WebSales.PublicBaseURL = "https://shop.example"
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := serveAccountLoginStart(cfg, &stubAccountWeb{}, rl)
	body := `{"email":"u@test.com","website":"","lang":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	raw := string(gotMail)
	if !strings.Contains(raw, "/account/session?token=") || !strings.Contains(raw, "lang=en") {
		t.Fatalf("magic link must include lang=en: %q", raw[:min(500, len(raw))])
	}
}

func TestServeAccountLoginStart_UnknownLangFallbackRU(t *testing.T) {
	var gotMail []byte
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotMail = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	cfg.WebSales.PublicBaseURL = "https://shop.example"
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := serveAccountLoginStart(cfg, &stubAccountWeb{}, rl)
	body := `{"email":"u@test.com","website":"","lang":"fr"}`
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	raw := string(gotMail)
	if strings.Contains(raw, "lang=en") || strings.Contains(raw, "lang=fr") {
		t.Fatalf("unknown lang must not append lang query: %q", raw[:min(500, len(raw))])
	}
}

func TestServeAccount_SetsLangCookie(t *testing.T) {
	cfg := orderStartTestCfg()
	h := serveAccount(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account?lang=en", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	ck := rec.Result().Cookies()
	var langCk *http.Cookie
	for _, c := range ck {
		if c.Name == accountLangCookieName {
			langCk = c
			break
		}
	}
	if langCk == nil || langCk.Value != "en" {
		t.Fatalf("expected vff_lang=en cookie, got %#v", langCk)
	}
	if !strings.Contains(rec.Body.String(), `<html lang="en"`) {
		t.Fatal("expected EN login page")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func riskyUserFacingSubstring(html, word string) bool {
	lower := strings.ToLower(html)
	word = strings.ToLower(word)
	for idx := 0; idx >= 0; {
		pos := strings.Index(lower[idx:], word)
		if pos < 0 {
			return false
		}
		pos += idx
		start := pos
		for start > 0 && lower[start-1] != '>' && lower[start-1] != '\n' {
			start--
		}
		chunk := lower[start:pos]
		if strings.Contains(chunk, "crossorigin=") || strings.Contains(chunk, "rel=") {
			idx = pos + len(word)
			continue
		}
		return true
	}
	return false
}
