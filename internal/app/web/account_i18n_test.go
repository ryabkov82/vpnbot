package web

import (
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"os"
	"os/exec"
	"regexp"
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

func TestAccountMarketingSiteURL(t *testing.T) {
	if got := accountMarketingSiteURL(accountLocaleRU); got != accountMarketingSiteURLRU {
		t.Fatalf("ru: got %q", got)
	}
	if got := accountMarketingSiteURL(accountLocaleEN); got != accountMarketingSiteURLEN {
		t.Fatalf("en: got %q", got)
	}
}

func TestRenderedAccountMarketingSiteLink(t *testing.T) {
	cfg := orderStartTestCfg()
	ruLogin := mustRenderAccountLoginHTML(t, cfg, accountLocaleRU)
	if !strings.Contains(ruLogin, `href="https://vpn-for-friends.com/"`) ||
		!strings.Contains(ruLogin, `>VPN for Friends</a>`) {
		t.Fatal("RU login footer brand must link to marketing site")
	}
	if strings.Contains(ruLogin, ">На сайт</a>") || strings.Contains(ruLogin, ">Website</a>") {
		t.Fatal("marketing link must use footer brand, not header site link")
	}

	enLogin := mustRenderAccountLoginHTML(t, cfg, accountLocaleEN)
	if !strings.Contains(enLogin, `href="https://vpn-for-friends.com/en/"`) ||
		!strings.Contains(enLogin, `>VPN for Friends</a>`) {
		t.Fatal("EN login footer brand must link to marketing site")
	}
	if !strings.Contains(enLogin, `/account?lang=en`) {
		t.Fatal("EN login must keep lang switcher")
	}

	ruSession := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	if !strings.Contains(ruSession, `href="https://vpn-for-friends.com/"`) ||
		!strings.Contains(ruSession, `account-footer`) {
		t.Fatal("RU session footer brand must link to marketing site")
	}

	enSession := mustRenderAccountSessionHTML(t, cfg, accountLocaleEN)
	if !strings.Contains(enSession, `href="https://vpn-for-friends.com/en/"`) {
		t.Fatal("EN session footer brand must link to marketing site")
	}
	if !strings.Contains(enSession, `/account/session?lang=en`) {
		t.Fatal("EN session must keep lang switcher with lang=en")
	}
}

func TestRenderedAccountSessionInvalidLinkI18n(t *testing.T) {
	cfg := orderStartTestCfg()
	ru := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	en := mustRenderAccountSessionHTML(t, cfg, accountLocaleEN)
	for _, html := range []string{ru, en} {
		if strings.Contains(html, `t('sessionInvalidLinkA')`) || strings.Contains(html, `"sessionInvalidLinkA"`) {
			t.Fatal("rendered session must not expose raw key sessionInvalidLinkA")
		}
		if !strings.Contains(html, "sessionInvalidLinkAction") {
			t.Fatal("rendered session must reference sessionInvalidLinkAction")
		}
	}
	if !strings.Contains(ru, `"sessionInvalidLink":"Ссылка недействительна или устарела."`) ||
		!strings.Contains(ru, `"sessionInvalidLinkAction":"Запросить новую ссылку для входа"`) ||
		!strings.Contains(ru, `t('sessionInvalidLinkAction')`) {
		t.Fatal("RU invalid-session i18n missing")
	}
	if !strings.Contains(en, `"sessionInvalidLink":"This sign-in link is invalid or expired."`) ||
		!strings.Contains(en, `"sessionInvalidLinkAction":"Request a new sign-in link"`) ||
		!strings.Contains(en, `"/account?lang=en"`) {
		t.Fatal("EN invalid-session i18n or login path missing")
	}
}

func TestRenderedAccountSessionInlineJS_NodeCheck(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	cfg := orderStartTestCfg()
	re := regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	for _, tc := range []struct {
		name   string
		locale accountLocale
	}{
		{"RU", accountLocaleRU},
		{"EN", accountLocaleEN},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := mustRenderAccountSessionHTML(t, cfg, tc.locale)
			var parts []string
			for _, m := range re.FindAllStringSubmatch(html, -1) {
				if len(m) > 1 {
					parts = append(parts, m[1])
				}
			}
			path := t.TempDir() + "/session-inline.js"
			if err := os.WriteFile(path, []byte(strings.Join(parts, "\n")), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "--check", path)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("node --check: %v\n%s", err, out)
			}
		})
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
		"Internal balance:",
		"Balance is maintained in RUB",
		"Prices are shown in USD. Internal balance is maintained in RUB.",
		"Choose a VPN plan. We will create a payment link for the selected amount. The service will be activated after payment is completed.",
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
