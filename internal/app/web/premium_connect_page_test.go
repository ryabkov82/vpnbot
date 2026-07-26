package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func clearPremiumSupportURLEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envTelegramSupportURL, "")
	t.Setenv(envTelegramSupportURLLegacy, "")
}

func TestPremiumConnect_SharedSupport_VFFAndFC(t *testing.T) {
	clearPremiumSupportURLEnv(t)
	const support = "https://t.me/shared_support_team"
	for _, name := range []string{"vff", "fc"} {
		t.Run(name, func(t *testing.T) {
			cfg := orderStartTestCfg()
			if name == "fc" {
				cfg = friendsConnectAccountTestCfg()
			}
			cfg.Telegram.SupportChat = support

			rec := httptest.NewRecorder()
			servePremiumConnect(cfg)(rec, httptest.NewRequest(http.MethodGet, "/premium-connect", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code=%d", rec.Code)
			}
			body := rec.Body.String()
			for _, needle := range []string{
				`href="` + support + `"`,
				"Написать в поддержку",
				`const redirectBase = '/redirect.html';`,
				`redirectBase + '?url=' + encodeURIComponent(link)`,
				"<title>Премиум AntiBlock VPN — Happ</title>",
				">Премиум AntiBlock VPN</h1>",
			} {
				if !strings.Contains(body, needle) {
					t.Fatalf("missing %q", needle)
				}
			}
			for _, forbid := range []string{
				"friends_connect_support",
				"mailto:",
				"support@vpn-for-friends.com",
				"vpn-for-friends.com/redirect.html",
				"friends-connect.club/redirect.html",
				"vpn-for-friends.com",
				"friends-connect.club",
			} {
				if strings.Contains(body, forbid) {
					t.Fatalf("must not contain %q", forbid)
				}
			}
			if strings.Contains(body, "const redirectBase = 'https://") ||
				strings.Contains(body, `const redirectBase = "https://`) {
				t.Fatal("redirectBase must be same-origin, not https absolute URL")
			}
		})
	}
}

func TestPremiumConnect_UsesConfiguredSupportURL(t *testing.T) {
	clearPremiumSupportURLEnv(t)
	const support = "https://t.me/another_support_team"
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = support

	body, err := renderedPremiumConnectHTML(cfg)
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

func TestPremiumConnect_MissingAndInvalidSupportOmitsBlock(t *testing.T) {
	clearPremiumSupportURLEnv(t)
	for _, tc := range []struct {
		name  string
		chat  string
		leaks []string
	}{
		{name: "empty", chat: "", leaks: []string{"javascript:", "friends_connect_support"}},
		{name: "javascript", chat: "javascript:alert(1)", leaks: []string{"javascript:", "javascript:alert(1)", "friends_connect_support"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := orderStartTestCfg()
			cfg.Telegram.SupportChat = tc.chat

			rec := httptest.NewRecorder()
			servePremiumConnect(cfg)(rec, httptest.NewRequest(http.MethodGet, "/premium-connect", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code=%d", rec.Code)
			}
			body := rec.Body.String()
			for _, needle := range []string{
				"service-id-warning",
				"access-token-warning",
				"/api/premium/service",
				"/api/premium/happ-link",
				"<title>Премиум AntiBlock VPN — Happ</title>",
				">Премиум AntiBlock VPN</h1>",
			} {
				if !strings.Contains(body, needle) {
					t.Fatalf("onboarding HTML missing %q", needle)
				}
			}
			for _, forbid := range append([]string{
				">Поддержка<",
				"Написать в поддержку",
				"mailto:",
				"t.me/",
			}, tc.leaks...) {
				if strings.Contains(body, forbid) {
					t.Fatalf("support block/fallback must be absent: %q", forbid)
				}
			}
		})
	}
}

func TestPremiumConnect_RedirectIsolation(t *testing.T) {
	clearPremiumSupportURLEnv(t)
	for _, name := range []string{"vff", "fc"} {
		t.Run(name, func(t *testing.T) {
			cfg := orderStartTestCfg()
			if name == "fc" {
				cfg = friendsConnectAccountTestCfg()
			}
			cfg.Telegram.SupportChat = "https://t.me/shared_support_team"
			body, err := renderedPremiumConnectHTML(cfg)
			if err != nil {
				t.Fatal(err)
			}
			html := string(body)
			if !strings.Contains(html, `const redirectBase = '/redirect.html';`) {
				t.Fatal("same-origin redirectBase missing")
			}
			if !strings.Contains(html, `redirectBase + '?url=' + encodeURIComponent(link)`) {
				t.Fatal("redirect URL construction missing")
			}
			for _, forbid := range []string{
				"vpn-for-friends.com/redirect.html",
				"friends-connect.club/redirect.html",
				"vpn-for-friends.com",
				"friends-connect.club",
			} {
				if strings.Contains(html, forbid) {
					t.Fatalf("foreign landing/redirect must not appear: %q", forbid)
				}
			}
		})
	}
}

func TestPremiumConnect_RoutesRegression(t *testing.T) {
	clearPremiumSupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = "https://t.me/shared_support_team"
	h := servePremiumConnect(cfg)

	needles := []string{
		"<title>Премиум AntiBlock VPN — Happ</title>",
		">Премиум AntiBlock VPN</h1>",
		`id="service-id-warning"`,
		`id="access-token-warning"`,
		"/api/premium/service",
		"/api/premium/happ-link",
		`data-platform-tab="android"`,
		`data-platform-tab="ios"`,
		`data-platform-tab="windows"`,
		"https://play.google.com/store/apps/details?id=com.happproxy",
		"https://github.com/Happ-proxy/happ-android/releases/latest/download/Happ.apk",
		"https://apps.apple.com/ru/app/happ-proxy-utility-plus/id6746188973",
		"https://apps.apple.com/us/app/happ-proxy-utility/id6504287215",
		"https://github.com/Happ-proxy/happ-desktop/releases/latest/download/setup-Happ.x64.exe",
		`id="href-add-config"`,
		`id="btn-copy-happ-link"`,
		`id="service-card"`,
		"Не передан service_id",
		"Не передан access_token",
		"Не удалось загрузить данные услуги",
		"service_id",
		"access_token",
		`const redirectBase = '/redirect.html';`,
	}

	for _, path := range []string{
		"/premium-connect",
		"/premium-connect/",
		"/premium-connect-test",
		"/premium-connect-test/",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code=%d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Fatalf("Content-Type=%q", ct)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Fatalf("Cache-Control=%q", cc)
			}
			body := rec.Body.Bytes()
			if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
				t.Fatalf("Content-Length=%q want %d", got, len(body))
			}
			html := string(body)
			for _, needle := range needles {
				if !strings.Contains(html, needle) {
					t.Fatalf("missing %q", needle)
				}
			}
		})
	}

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/premium-connect", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST code=%d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET" {
		t.Fatalf("Allow=%q", got)
	}

	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/premium-connect/extra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nested path code=%d", rec.Code)
	}
}

func TestRenderedPremiumConnectHTML_NilConfig(t *testing.T) {
	_, err := renderedPremiumConnectHTML(nil)
	if err == nil || !strings.Contains(err.Error(), "premium connect config is required") {
		t.Fatalf("err=%v", err)
	}
}

func TestServePremiumConnect_NilConfigReturns500(t *testing.T) {
	clearPremiumSupportURLEnv(t)
	rec := httptest.NewRecorder()
	servePremiumConnect(nil)(rec, httptest.NewRequest(http.MethodGet, "/premium-connect", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Internal Server Error") {
		t.Fatalf("body=%q", body)
	}
	for _, forbid := range []string{
		"Премиум AntiBlock VPN",
		"Написать в поддержку",
		"friends_connect_support",
		"service-id-warning",
	} {
		if strings.Contains(body, forbid) {
			t.Fatalf("500 must not contain premium page/fallback %q", forbid)
		}
	}
}

func TestPremiumConnectInlineJS_NodeCheck(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	clearPremiumSupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = "https://t.me/shared_support_team"
	html, err := renderedPremiumConnectHTML(cfg)
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
	path := t.TempDir() + "/premium-connect-inline.js"
	if err := os.WriteFile(path, []byte(strings.Join(parts, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--check", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
}
