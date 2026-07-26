package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestRenderedAccountLinkTitles_VFF(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.Name = "VPN for Friends"

	start, err := renderedAccountLinkStartHTML(cfg, "tok-vff")
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := renderedAccountLinkInvalidHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := renderedAccountLinkStandaloneConflictHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		html string
		want string
	}{
		{"start", string(start), "<title>Привязка личного кабинета — VPN for Friends</title>"},
		{"invalid", string(invalid), "<title>Ссылка устарела — VPN for Friends</title>"},
		{"conflict", string(conflict), "<title>Привязка — VPN for Friends</title>"},
	} {
		if !strings.Contains(tc.html, tc.want) {
			t.Fatalf("%s missing %q", tc.name, tc.want)
		}
		if strings.Contains(tc.html, "— Friends Connect") {
			t.Fatalf("%s must not contain Friends Connect title suffix", tc.name)
		}
	}
}

func TestRenderedAccountLinkTitles_FriendsConnect(t *testing.T) {
	cfg := friendsConnectAccountTestCfg()
	cfg.Brand.Name = "Friends Connect"

	start, err := renderedAccountLinkStartHTML(cfg, "tok-fc")
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := renderedAccountLinkInvalidHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := renderedAccountLinkStandaloneConflictHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		html string
		want string
	}{
		{"start", string(start), "<title>Привязка личного кабинета — Friends Connect</title>"},
		{"invalid", string(invalid), "<title>Ссылка устарела — Friends Connect</title>"},
		{"conflict", string(conflict), "<title>Привязка — Friends Connect</title>"},
	} {
		if !strings.Contains(tc.html, tc.want) {
			t.Fatalf("%s missing %q", tc.name, tc.want)
		}
		if strings.Contains(tc.html, "— VPN for Friends") {
			t.Fatalf("%s must not contain VFF title suffix", tc.name)
		}
	}
}

func TestRenderedAccountLinkStartHTML_TokenInjection(t *testing.T) {
	cfg := orderStartTestCfg()
	const token = `abc"def\nghi`
	body, err := renderedAccountLinkStartHTML(cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if strings.Contains(html, accountLinkStartTokenMarker) {
		t.Fatal("marker must be removed after injection")
	}
	quoted := strconv.Quote(token)
	if !strings.Contains(html, "var tok = "+quoted+";") {
		t.Fatalf("token must be injected as quoted JS string; want %q", quoted)
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	re := regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	var parts []string
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			parts = append(parts, m[1])
		}
	}
	if len(parts) == 0 {
		t.Fatal("expected inline script")
	}
	path := t.TempDir() + "/link-start-inline.js"
	if err := os.WriteFile(path, []byte(strings.Join(parts, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--check", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
}

func TestAccountLinkBrandName_FailClosed(t *testing.T) {
	if _, err := accountLinkBrandName(nil); err == nil || !strings.Contains(err.Error(), "account link brand name is required") {
		t.Fatalf("nil: %v", err)
	}
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""
	if _, err := accountLinkBrandName(cfg); err == nil {
		t.Fatal("empty name must fail")
	}
	cfg.Brand.Name = " \t "
	if _, err := accountLinkBrandName(cfg); err == nil {
		t.Fatal("whitespace name must fail")
	}
}

func TestRenderedAccountLinkPages_FailClosed(t *testing.T) {
	for _, fn := range []struct {
		name string
		call func() error
	}{
		{"invalid_nil", func() error { _, err := renderedAccountLinkInvalidHTML(nil); return err }},
		{"conflict_nil", func() error { _, err := renderedAccountLinkStandaloneConflictHTML(nil); return err }},
		{"start_nil", func() error { _, err := renderedAccountLinkStartHTML(nil, "t"); return err }},
		{"invalid_empty", func() error {
			cfg := orderStartTestCfg()
			cfg.Brand.Name = ""
			_, err := renderedAccountLinkInvalidHTML(cfg)
			return err
		}},
		{"start_whitespace", func() error {
			cfg := orderStartTestCfg()
			cfg.Brand.Name = "   "
			_, err := renderedAccountLinkStartHTML(cfg, "t")
			return err
		}},
	} {
		if err := fn.call(); err == nil || !strings.Contains(err.Error(), "account link brand name is required") {
			t.Fatalf("%s: err=%v", fn.name, err)
		}
	}
}

func TestRenderedAccountLinkTitles_EscapesBrandName(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.Name = "Friends <Connect>"
	body, err := renderedAccountLinkInvalidHTML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if !strings.Contains(html, "<title>Ссылка устарела — Friends &lt;Connect&gt;</title>") {
		t.Fatalf("escaped title missing: %s", html[:min(300, len(html))])
	}
	if strings.Contains(html, "<title>Ссылка устарела — Friends <Connect></title>") {
		t.Fatal("raw brand angle brackets must not appear in title")
	}
}

func TestServeAccountLink_StartPageBrandMatrix(t *testing.T) {
	cases := []struct {
		name      string
		brandID   string
		brandName string
		title     string
		forbid    string
	}{
		{"vff", "vff", "VPN for Friends", "<title>Привязка личного кабинета — VPN for Friends</title>", "— Friends Connect"},
		{"fc", "fc", "Friends Connect", "<title>Привязка личного кабинета — Friends Connect</title>", "— VPN for Friends"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const chatID int64 = 110022
			sec := strings.Repeat("y", 40)
			cfg := orderStartTestCfg()
			if tc.brandID == "fc" {
				cfg = friendsConnectAccountTestCfg()
			}
			cfg.Brand.ID = tc.brandID
			cfg.Brand.Name = tc.brandName
			cfg.WebSales.OrderTokenSecret = sec
			linkTok, err := CreateAccountTelegramLinkToken(sec, tc.brandID, 5, chatID, cfg)
			if err != nil {
				t.Fatal(err)
			}
			u := models.User{
				ID:       5,
				Login:    "@tg5",
				Login2:   "",
				Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
			}
			rec := httptest.NewRecorder()
			serveAccountLink(cfg, &stubAccountWeb{getUserByIDRet: &u}).ServeHTTP(rec,
				httptest.NewRequest(http.MethodGet, "/account/link?token="+url.QueryEscape(linkTok), nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code=%d", rec.Code)
			}
			body := rec.Body.String()
			for _, needle := range []string{
				tc.title,
				"Привязка личного кабинета",
				"/api/account/google/start",
				"/api/account/link/login/start",
				"var tok = " + strconv.Quote(linkTok) + ";",
			} {
				if !strings.Contains(body, needle) {
					t.Fatalf("missing %q", needle)
				}
			}
			if strings.Contains(body, accountLinkStartTokenMarker) {
				t.Fatal("marker must not remain")
			}
			if strings.Contains(body, tc.forbid) {
				t.Fatalf("must not contain %q", tc.forbid)
			}
		})
	}
}

func TestServeAccountLink_InvalidPageBrandMatrix(t *testing.T) {
	cases := []struct {
		name      string
		brandName string
		title     string
		forbid    string
	}{
		{"vff", "VPN for Friends", "<title>Ссылка устарела — VPN for Friends</title>", "— Friends Connect"},
		{"fc", "Friends Connect", "<title>Ссылка устарела — Friends Connect</title>", "— VPN for Friends"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := orderStartTestCfg()
			if tc.name == "fc" {
				cfg = friendsConnectAccountTestCfg()
			}
			cfg.Brand.Name = tc.brandName
			rec := httptest.NewRecorder()
			serveAccountLink(cfg, &stubAccountWeb{}).ServeHTTP(rec,
				httptest.NewRequest(http.MethodGet, "/account/link?token=not-a-valid-token", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code=%d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.title) {
				t.Fatalf("missing title %q", tc.title)
			}
			if !strings.Contains(body, "Ссылка для привязки устарела") {
				t.Fatal("invalid copy missing")
			}
			if !strings.Contains(body, `href="/account"`) {
				t.Fatal("missing /account link")
			}
			if strings.Contains(body, tc.forbid) {
				t.Fatalf("must not contain %q", tc.forbid)
			}
		})
	}
}

func TestServeAccountLink_ConflictPageBrandMatrix(t *testing.T) {
	cases := []struct {
		name      string
		brandName string
		title     string
		forbid    string
	}{
		{"vff", "VPN for Friends", "<title>Привязка — VPN for Friends</title>", "— Friends Connect"},
		{"fc", "Friends Connect", "<title>Привязка — Friends Connect</title>", "— VPN for Friends"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := orderStartTestCfg()
			if tc.name == "fc" {
				cfg = friendsConnectAccountTestCfg()
			}
			cfg.Brand.Name = tc.brandName
			rec := httptest.NewRecorder()
			serveAccountLink(cfg, &stubAccountWeb{}).ServeHTTP(rec,
				httptest.NewRequest(http.MethodGet, "/account/link?err=google_email_conflict", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code=%d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.title) {
				t.Fatalf("missing title %q", tc.title)
			}
			if !strings.Contains(body, "Этот email уже привязан к другому аккаунту") {
				t.Fatal("conflict copy missing")
			}
			if !strings.Contains(body, `href="/account"`) {
				t.Fatal("missing /account link")
			}
			if strings.Contains(body, tc.forbid) {
				t.Fatalf("must not contain %q", tc.forbid)
			}
		})
	}

	cfg := orderStartTestCfg()
	rec := httptest.NewRecorder()
	serveAccountLink(cfg, &stubAccountWeb{}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/account/link?err=email_used_by_other", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("email_used_by_other code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>Привязка — VPN for Friends</title>") {
		t.Fatal("email_used_by_other must use conflict renderer")
	}
}

func TestServeAccountLink_RenderErrorReturns500(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""
	rec := httptest.NewRecorder()
	serveAccountLink(cfg, &stubAccountWeb{}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/account/link", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, forbid := range []string{"VPN for Friends", "Friends Connect", "Привязка личного кабинета", "Ссылка устарела"} {
		if strings.Contains(body, forbid) {
			t.Fatalf("500 must not contain %q", forbid)
		}
	}
	if !strings.Contains(body, "Internal Server Error") {
		t.Fatalf("body=%q", body)
	}
}
