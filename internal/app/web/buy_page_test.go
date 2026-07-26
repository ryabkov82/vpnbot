package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func clearBuySupportURLEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envTelegramSupportURL, "")
	t.Setenv(envTelegramSupportURLLegacy, "")
}

func TestBuyPageContainsAccountLink(t *testing.T) {
	clearBuySupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = "https://t.me/shared_support_team"
	req := httptest.NewRequest(http.MethodGet, "/buy", nil)
	rec := httptest.NewRecorder()
	serveBuy(cfg)(rec, req)
	body := rec.Body.Bytes()
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rec.Code)
	}
	if !bytes.Contains(body, []byte("Уже покупали VPN?")) {
		t.Fatal("missing account promo text")
	}
	if !bytes.Contains(body, []byte(`href="/account"`)) {
		t.Fatal("missing /account link")
	}
	if strings.Contains(string(body), "/api/public/order/start") {
		t.Fatal("/buy UI must not reference /api/public/order/start")
	}
	if !bytes.Contains(body, []byte("/api/public/services")) {
		t.Fatal("/buy must load tariffs from /api/public/services")
	}
	if !bytes.Contains(body, []byte("Войти и купить")) {
		t.Fatal(`missing "Войти и купить" CTA`)
	}
	if !bytes.Contains(body, []byte("личный кабинет")) {
		t.Fatal("missing cabinet copy")
	}
	if !strings.Contains(string(body), `String(s.tier || '') === 'premium'`) {
		t.Fatal("/buy UI must honour tier=premium for badges")
	}
	for _, forbid := range []string{"SHM", "Remnawave", "internal_squad_name"} {
		if strings.Contains(string(body), forbid) {
			t.Fatalf("buy UI must not contain %q", forbid)
		}
	}
}

func TestBuyPage_VFFIdentityAndSharedSupport(t *testing.T) {
	clearBuySupportURLEnv(t)
	const support = "https://t.me/shared_support_team"
	cfg := orderStartTestCfg()
	cfg.Brand.Name = "VPN for Friends"
	cfg.Telegram.SupportChat = support

	rec := httptest.NewRecorder()
	serveBuy(cfg)(rec, httptest.NewRequest(http.MethodGet, "/buy", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"<title>VPN for Friends — купить VPN</title>",
		`<h1 class="h4 fw-bold text-center mb-2">VPN for Friends</h1>`,
		`href="` + support + `"`,
		"Поддержка в Telegram",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("VFF buy missing %q", needle)
		}
	}
	for _, forbid := range []string{"Friends Connect", "friends_connect_support"} {
		if strings.Contains(body, forbid) {
			t.Fatalf("VFF buy must not contain %q", forbid)
		}
	}
}

func TestBuyPage_FriendsConnectIdentityAndSharedSupport(t *testing.T) {
	clearBuySupportURLEnv(t)
	const support = "https://t.me/shared_support_team"
	cfg := friendsConnectAccountTestCfg()
	cfg.Brand.Name = "Friends Connect"
	cfg.Telegram.SupportChat = support

	rec := httptest.NewRecorder()
	serveBuy(cfg)(rec, httptest.NewRequest(http.MethodGet, "/buy", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"<title>Friends Connect — купить VPN</title>",
		`<h1 class="h4 fw-bold text-center mb-2">Friends Connect</h1>`,
		`href="` + support + `"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("FC buy missing %q", needle)
		}
	}
	for _, forbid := range []string{"VPN for Friends", "friends_connect_support", "vpn_for_friends_support"} {
		if strings.Contains(body, forbid) {
			t.Fatalf("FC buy must not contain %q", forbid)
		}
	}
}

func TestBuyPage_UsesConfiguredSupportURL(t *testing.T) {
	clearBuySupportURLEnv(t)
	const support = "https://t.me/another_support_team"
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = support

	body, err := renderedBuyPageHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if !strings.Contains(html, `href="`+support+`"`) {
		t.Fatalf("want configured support %q", support)
	}
	if strings.Contains(html, "shared_support_team") || strings.Contains(html, "friends_connect_support") {
		t.Fatal("must not keep fixture/hardcoded support URL")
	}
}

func TestBuyPage_MissingSupportOmitsBlock(t *testing.T) {
	clearBuySupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = ""

	rec := httptest.NewRecorder()
	serveBuy(cfg)(rec, httptest.NewRequest(http.MethodGet, "/buy", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>VPN for Friends — купить VPN</title>") {
		t.Fatal("title must still render")
	}
	if !strings.Contains(body, `<h1 class="h4 fw-bold text-center mb-2">VPN for Friends</h1>`) {
		t.Fatal("H1 must still render")
	}
	for _, forbid := range []string{
		"Поддержка в Telegram",
		"t.me/",
		"friends_connect_support",
		"mailto:",
	} {
		if strings.Contains(body, forbid) {
			t.Fatalf("empty support must omit contact block %q", forbid)
		}
	}
}

func TestBuyPage_InvalidSupportOmitsBlock(t *testing.T) {
	clearBuySupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = "javascript:alert(1)"

	body, err := renderedBuyPageHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if strings.Contains(html, "javascript:") || strings.Contains(html, "Поддержка в Telegram") {
		t.Fatal("invalid support must not appear in HTML")
	}
	if strings.Contains(html, "friends_connect_support") {
		t.Fatal("invalid support must not fall back to hardcoded contact")
	}
}

func TestRenderedBuyPageHTML_FailClosed(t *testing.T) {
	if _, err := renderedBuyPageHTML(nil); err == nil || !strings.Contains(err.Error(), "buy page brand name is required") {
		t.Fatalf("nil cfg: err=%v", err)
	}
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""
	if _, err := renderedBuyPageHTML(cfg); err == nil || !strings.Contains(err.Error(), "buy page brand name is required") {
		t.Fatalf("empty name: err=%v", err)
	}
	cfg = orderStartTestCfg()
	cfg.Brand.Name = "  \t  "
	if _, err := renderedBuyPageHTML(cfg); err == nil || !strings.Contains(err.Error(), "buy page brand name is required") {
		t.Fatalf("whitespace name: err=%v", err)
	}
}

func TestServeBuy_RenderErrorReturns500WithoutFallback(t *testing.T) {
	clearBuySupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""

	rec := httptest.NewRecorder()
	serveBuy(cfg)(rec, httptest.NewRequest(http.MethodGet, "/buy", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "VPN for Friends") || strings.Contains(body, "Friends Connect") {
		t.Fatal("500 must not leak brand fallback identity")
	}
	if !strings.Contains(body, "Internal Server Error") {
		t.Fatalf("body=%q", body)
	}
}

func TestServeBuy_MethodsAndPaths(t *testing.T) {
	clearBuySupportURLEnv(t)
	cfg := orderStartTestCfg()
	h := serveBuy(cfg)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/buy/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/buy/ code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/buy", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST code=%d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET" {
		t.Fatalf("Allow=%q", got)
	}

	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/buy/extra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nested path code=%d", rec.Code)
	}
}

func TestBuyPageInlineJS_NodeCheck(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	clearBuySupportURLEnv(t)
	cfg := orderStartTestCfg()
	html, err := renderedBuyPageHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	var parts []string
	for _, m := range re.FindAllStringSubmatch(string(html), -1) {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			parts = append(parts, m[1])
		}
	}
	if len(parts) == 0 {
		t.Fatal("expected inline script")
	}
	path := t.TempDir() + "/buy-inline.js"
	if err := os.WriteFile(path, []byte(strings.Join(parts, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--check", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
}
