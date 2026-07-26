package web

import (
	"encoding/json"
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
	"github.com/ryabkov82/vpnbot/internal/models"
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

func TestAccountBrandIdentity_FailClosed(t *testing.T) {
	if _, _, err := accountBrandIdentity(nil); err == nil || !strings.Contains(err.Error(), "account brand name is required") {
		t.Fatalf("nil cfg: err=%v", err)
	}
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""
	if _, _, err := accountBrandIdentity(cfg); err == nil || !strings.Contains(err.Error(), "account brand name is required") {
		t.Fatalf("empty name: err=%v", err)
	}
	cfg = orderStartTestCfg()
	cfg.Brand.LandingURL = ""
	if _, _, err := accountBrandIdentity(cfg); err == nil || !strings.Contains(err.Error(), "account brand landing URL is required") {
		t.Fatalf("empty landing: err=%v", err)
	}
}

func TestRenderedAccountLoginSession_FailClosed(t *testing.T) {
	if _, err := renderedAccountLoginPageHTML(nil, accountLocaleRU); err == nil {
		t.Fatal("nil cfg login must fail")
	}
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""
	if _, err := renderedAccountLoginPageHTML(cfg, accountLocaleRU); err == nil {
		t.Fatal("empty name login must fail")
	}
	cfg = orderStartTestCfg()
	cfg.Brand.LandingURL = ""
	if _, err := renderedAccountSessionPageHTML(cfg, accountLocaleEN, nil); err == nil {
		t.Fatal("empty landing session must fail")
	}
}

func TestRenderedAccountIdentity_VFF(t *testing.T) {
	cfg := orderStartTestCfg()
	const landing = "https://vpn-for-friends.com"

	ruLogin := mustRenderAccountLoginHTML(t, cfg, accountLocaleRU)
	for _, needle := range []string{
		"<title>Личный кабинет — VPN for Friends</title>",
		"<h1 class=\"h4 fw-bold mb-3\">Личный кабинет VPN for Friends</h1>",
		`href="` + landing + `"`,
		`>VPN for Friends</a>`,
		"Введите email — мы отправим ссылку для входа без пароля.",
	} {
		if !strings.Contains(ruLogin, needle) {
			t.Fatalf("VFF RU login missing %q", needle)
		}
	}
	if strings.Contains(ruLogin, ">На сайт</a>") || strings.Contains(ruLogin, ">Website</a>") {
		t.Fatal("marketing link must use footer brand, not header site link")
	}

	enLogin := mustRenderAccountLoginHTML(t, cfg, accountLocaleEN)
	for _, needle := range []string{
		"<title>Account — VPN for Friends</title>",
		"<h1 class=\"h4 fw-bold mb-3\">VPN for Friends account</h1>",
		`href="` + landing + `"`,
		`>VPN for Friends</a>`,
		`/account?lang=en`,
	} {
		if !strings.Contains(enLogin, needle) {
			t.Fatalf("VFF EN login missing %q", needle)
		}
	}
	if strings.Contains(enLogin, "vpn-for-friends.com/en/") {
		t.Fatal("EN footer must use canonical landing_url without /en/")
	}

	ruSession := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	for _, needle := range []string{
		"<title>Кабинет — VPN for Friends</title>",
		`href="` + landing + `"`,
		`>VPN for Friends</a>`,
		`account-footer`,
	} {
		if !strings.Contains(ruSession, needle) {
			t.Fatalf("VFF RU session missing %q", needle)
		}
	}

	enSession := mustRenderAccountSessionHTML(t, cfg, accountLocaleEN)
	for _, needle := range []string{
		"<title>Account — VPN for Friends</title>",
		`href="` + landing + `"`,
		`/account/session?lang=en`,
	} {
		if !strings.Contains(enSession, needle) {
			t.Fatalf("VFF EN session missing %q", needle)
		}
	}
	if strings.Contains(enSession, "vpn-for-friends.com/en/") {
		t.Fatal("EN session footer must use canonical landing_url without /en/")
	}
}

func TestRenderedAccountIdentity_FriendsConnect(t *testing.T) {
	cfg := friendsConnectAccountTestCfg()
	const landing = "https://friends-connect.club"

	assertIdentityNoVFF := func(t *testing.T, html string) {
		t.Helper()
		// Identity surfaces only (title/H1/footer). PaymentMethodSupport email is out of this commit.
		if strings.Contains(html, "VPN for Friends") {
			t.Fatal("FC identity must not show VPN for Friends")
		}
		if strings.Contains(html, `href="https://vpn-for-friends.com`) {
			t.Fatal("FC footer must not link to vpn-for-friends.com landing")
		}
	}

	ruLogin := mustRenderAccountLoginHTML(t, cfg, accountLocaleRU)
	for _, needle := range []string{
		"<title>Личный кабинет — Friends Connect</title>",
		"<h1 class=\"h4 fw-bold mb-3\">Личный кабинет Friends Connect</h1>",
		`href="` + landing + `"`,
		`>Friends Connect</a>`,
		"Введите email — мы отправим ссылку для входа без пароля.",
	} {
		if !strings.Contains(ruLogin, needle) {
			t.Fatalf("FC RU login missing %q", needle)
		}
	}
	assertIdentityNoVFF(t, ruLogin)

	enLogin := mustRenderAccountLoginHTML(t, cfg, accountLocaleEN)
	for _, needle := range []string{
		"<title>Account — Friends Connect</title>",
		"<h1 class=\"h4 fw-bold mb-3\">Friends Connect account</h1>",
		`href="` + landing + `"`,
		`>Friends Connect</a>`,
	} {
		if !strings.Contains(enLogin, needle) {
			t.Fatalf("FC EN login missing %q", needle)
		}
	}
	assertIdentityNoVFF(t, enLogin)

	ruSession := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	for _, needle := range []string{
		"<title>Кабинет — Friends Connect</title>",
		`href="` + landing + `"`,
		`>Friends Connect</a>`,
	} {
		if !strings.Contains(ruSession, needle) {
			t.Fatalf("FC RU session missing %q", needle)
		}
	}
	assertIdentityNoVFF(t, ruSession)

	enSession := mustRenderAccountSessionHTML(t, cfg, accountLocaleEN)
	for _, needle := range []string{
		"<title>Account — Friends Connect</title>",
		`href="` + landing + `"`,
		`>Friends Connect</a>`,
	} {
		if !strings.Contains(enSession, needle) {
			t.Fatalf("FC EN session missing %q", needle)
		}
	}
	assertIdentityNoVFF(t, enSession)
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
		"Cryptocurrency",
		"/api/account/balance/topup/cryptocloud",
		"Top up internal balance",
		"150 (≈ $2)",
		"300 (≈ $4)",
		"Custom internal amount: 50–10,000",
		"This amount is calculated by the billing system for service payment or renewal.",
		"Internal balance:",
		"Balance is maintained in RUB",
		"Prices are shown in USD for convenience. Internal balance is maintained in RUB. The final crypto invoice is calculated from the internal RUB amount by the payment provider.",
		"Prices are shown in USD for convenience. Your internal balance is RUB-based. The crypto invoice will show the final equivalent amount on the payment provider page.",
		"Choose a VPN plan. We will create a payment link for the selected amount. The service will be activated after payment is completed.",
		`"currencyDisplay":"RUB"`,
		"Premium connection via Happ app.",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("EN session missing %q", needle)
		}
	}
	for _, forbid := range []string{
		"150 RUB",
		"300 RUB",
		"Bank card",
		"Card payment via the current payment gateway",
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

func TestRenderedAccountSession_RU_TopupModalUnchanged(t *testing.T) {
	html := mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU)
	for _, needle := range []string{
		"Пополнение баланса",
		"150 ₽",
		"300 ₽",
		"50–10 000 ₽",
		`name="topup-payment-method" value="yookassa" checked`,
		`name="topup-payment-method" value="cryptocloud"`,
		"Банковская карта",
		"Криптовалюта",
		`id="topup-custom" min="50" max="10000" step="0.01"`,
		`type="number"`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("RU session missing %q", needle)
		}
	}
}

func TestRenderedAccountSession_EN_TopupModal(t *testing.T) {
	html := mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleEN)
	for _, needle := range []string{
		"Top up internal balance",
		`id="topup-custom"`,
		`type="text"`,
		`inputmode="decimal"`,
		`lang="en"`,
		`data-amt="150">150 (≈ $2)<`,
		`data-amt="300">300 (≈ $4)<`,
		"Custom internal amount: 50–10,000",
		"function formatTopupAmountInput",
		"function setTopupCustomAmount",
		"function parseTopupAmountInput",
		"setTopupCustomAmount(amt)",
		"setTopupCustomAmount(presetAmt)",
		"setTopupCustomAmount(Math.round(amtOrder * 100) / 100)",
		"parseTopupAmountInput(customIn.value)",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("EN top-up modal missing %q", needle)
		}
	}
	for _, forbid := range []string{
		"150 RUB",
		"300 RUB",
		"450 RUB",
		"600 RUB",
		`id="topup-custom" min="50"`,
		`customIn.value = b.getAttribute('data-amt')`,
		`customIn.value = String(`,
		`ciAmt.value = formatTopupAmountInput`,
		`customIn.value = fmtMoney`,
	} {
		if strings.Contains(html, forbid) {
			t.Fatalf("EN top-up modal must not contain %q", forbid)
		}
	}
}

func TestRenderedAccountSession_EN_CatalogCardNoPeriodMeta(t *testing.T) {
	html := mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleEN)
	if !strings.Contains(html, "catalogPeriodMetaHtml") ||
		!strings.Contains(html, "acfg.lang === 'en'") {
		t.Fatal("EN catalog must skip period meta row when lang is en")
	}
	iCat := strings.Index(html, "function loadAccountCatalog")
	if iCat < 0 {
		t.Fatal("loadAccountCatalog missing")
	}
	catBlock := html[iCat:]
	if iEnd := strings.Index(catBlock, "function attachConnect"); iEnd > 0 {
		catBlock = catBlock[:iEnd]
	}
	if !strings.Contains(catBlock, "catalogPeriodMetaHtml +") {
		t.Fatal("EN catalog card layout must use catalogPeriodMetaHtml")
	}
	for _, needle := range []string{
		`"catalogMonthsSuffix":" mo."`,
		"display_amount_text",
		"display_monthly_text",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("EN session missing %q", needle)
		}
	}
	for _, forbid := range []string{"1 mo.", "3 mo.", "6 mo.", "12 mo."} {
		if strings.Contains(html, forbid) {
			t.Fatalf("EN session must not contain static period meta %q", forbid)
		}
	}
}

func TestRenderedAccountSession_EN_CatalogAPIStillShowsTitlesAndUSD(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "u@test.com", 1, "lg", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{
			serviceWithPricing(3, "1 месяц", 150, 1, 200),
			serviceWithPricing(4, "3 месяца", 450, 3, 500),
		},
	}
	rec := httptest.NewRecorder()
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok+"&lang=en", nil))
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	want := map[int]struct{ name, price, monthly string }{
		3: {"1 month", "$2", "$2/mo"},
		4: {"3 months", "$5", "$1.67/mo"},
	}
	for _, svc := range out.Services {
		w, ok := want[svc.ServiceID]
		if !ok {
			continue
		}
		if svc.Name != w.name || svc.DisplayAmountText != w.price || svc.DisplayMonthlyText != w.monthly {
			t.Fatalf("service %d: %#v want %#v", svc.ServiceID, svc, w)
		}
	}
}

func TestRenderedAccountSession_RU_CatalogCardKeepsPeriodMeta(t *testing.T) {
	html := mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU)
	if !strings.Contains(html, "catalogPeriodMetaHtml") ||
		!strings.Contains(html, "catalogMonthsLabel(Number(s.period))") {
		t.Fatal("RU catalog must keep period meta rendering")
	}
	if !strings.Contains(html, `"catalogMonthsSuffix":" мес."`) {
		t.Fatal("RU catalog months suffix missing")
	}
}

func TestServeAccountLoginStart_LangENMagicLink(t *testing.T) {
	var gotMail []byte
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotMail = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	cfg.Brand.PublicBaseURL = "https://shop.example"
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
	cfg.Brand.PublicBaseURL = "https://shop.example"
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
