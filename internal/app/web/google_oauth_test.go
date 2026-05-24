package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func testGoogleOAuthMinimalCfg(secret string, enabled bool, id, redirect, sec string) *config.Config {
	c := &config.Config{}
	c.WebSales.OrderTokenSecret = secret
	c.WebAccount.GoogleEnabled = enabled
	c.WebAccount.GoogleClientID = id
	c.WebAccount.GoogleClientSecret = sec
	c.WebAccount.GoogleRedirectURL = redirect
	return c
}

func patchGoogleOAuthEndpoints(t *testing.T, tok, userinfo string) {
	t.Helper()
	prevTok := googleOAuthTokenURLOverride
	prevUI := googleOAuthUserinfoURLOverride
	googleOAuthTokenURLOverride = tok
	googleOAuthUserinfoURLOverride = userinfo
	t.Cleanup(func() {
		googleOAuthTokenURLOverride = prevTok
		googleOAuthUserinfoURLOverride = prevUI
	})
}

func googleOAuthTestMockHandler(userEmail string, verified bool, failToken bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		switch path {
		case "/token":
			if failToken || r.Method != http.MethodPost {
				http.Error(w, "bad", http.StatusUnauthorized)
				return
			}
			_ = r.ParseForm()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"access_token":"__test_access_fake__","token_type":"Bearer"}`)
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			b, _ := json.Marshal(map[string]any{"email": userEmail, "email_verified": verified})
			_, _ = w.Write(b)
		default:
			http.NotFound(w, r)
		}
	})
}

func findCookieValue(hdr http.Header, name string) string {
	for _, ck := range hdr.Values("Set-Cookie") {
		parts := strings.Split(ck, ";")
		if len(parts) == 0 {
			continue
		}
		nv := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
		if len(nv) == 2 && nv[0] == name {
			return nv[1]
		}
	}
	return ""
}

func TestGoogleOAuthAvailable(t *testing.T) {
	secretOK := strings.Repeat("a", 36)
	tab := []struct {
		name   string
		cfg    *config.Config
		expect bool
	}{
		{"nil cfg", nil, false},
		{"disabled", testGoogleOAuthMinimalCfg(secretOK, false, "id", "https://example.com/cb", "sec"), false},
		{"enabled_missing_id", testGoogleOAuthMinimalCfg(secretOK, true, "", "https://example.com/cb", "sec"), false},
		{"enabled_missing_secret", testGoogleOAuthMinimalCfg(secretOK, true, "id", "https://example.com/cb", ""), false},
		{"enabled_missing_redirect", testGoogleOAuthMinimalCfg(secretOK, true, "id", "", "sec"), false},
		{"enabled_all_whitespace_redirect", testGoogleOAuthMinimalCfg(secretOK, true, "id", "   ", "sec"), false},
		{"ok", testGoogleOAuthMinimalCfg(secretOK, true, "cid", "https://example.com/cb", "shh"), true},
	}
	for _, tc := range tab {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := googleOAuthAvailable(tc.cfg); got != tc.expect {
				t.Fatalf("googleOAuthAvailable = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestGETGoogleOAuthStart_POSTMethodNotAllowed(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("b", 40), true,
		"cid", "https://x/c", "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/google/start", nil)
	serveGoogleOAuthStart(cfg)(rec, req)
	if rec.Code != http.StatusMethodNotAllowed ||
		rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("code=%d allow=%s", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestGETGoogleOAuthStart_Disabled_Returns404(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("b", 40), false, "cid", "https://x/c", "secret")
	rec := httptest.NewRecorder()
	serveGoogleOAuthStart(cfg)(rec, httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGETGoogleOAuthStart_MissingSalesSecret_Returns404(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg("", true, "cid", "https://x/c", "secret")
	rec := httptest.NewRecorder()
	serveGoogleOAuthStart(cfg)(rec, httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGETGoogleOAuthStart_Enabled_RedirectSetsStateCookieAndURL(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("c", 40), true,
		"my-client-id.apps.googleusercontent.com",
		"https://connect.vpn-for-friends.com/api/account/google/callback",
		"client-secret-val")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	serveGoogleOAuthStart(cfg)(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, googleOAuthAuthURL+"?") {
		t.Fatalf("bad location prefix: %s", loc)
	}
	v, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	state := v.Query().Get("state")
	if state == "" {
		t.Fatal("missing state query")
	}
	if v.Query().Get("client_id") != cfg.WebAccount.GoogleClientID {
		t.Fatalf("client_id mismatch")
	}
	if v.Query().Get("redirect_uri") != cfg.WebAccount.GoogleRedirectURL {
		t.Fatalf("redirect_uri mismatch")
	}
	if v.Query().Get("scope") != "openid email profile" {
		t.Fatalf("scope mismatch: %q", v.Query().Get("scope"))
	}
	var oauthCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == googleOAuthCookieName {
			oauthCookie = c
			break
		}
	}
	if oauthCookie == nil {
		t.Fatal("oauth state cookie missing")
	}
	if oauthCookie.Value != state {
		t.Fatalf("cookie/state mismatch cookie=%q state=%q", oauthCookie.Value, state)
	}
	if !oauthCookie.HttpOnly || !oauthCookie.Secure || oauthCookie.MaxAge <= 0 ||
		oauthCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags %+v", oauthCookie)
	}
}

func TestGoogleOAuthCallback_MissingStateCookie(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("d", 41), true, "cid",
		"https://callback/x", "shh")
	st := stubAccountWeb{findOrCreateRet: &models.User{ID: 1, Login: "web_1"}}
	rec := httptest.NewRecorder()
	u, _ := url.Parse("/api/account/google/callback?code=ccc&state=sss")
	req := httptest.NewRequest(http.MethodGet, u.String(), nil)
	serveGoogleOAuthCallback(cfg, &st)(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_state") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGoogleOAuthCallback_WrongState(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("e", 41), true, "cid",
		"https://callback/x", "shh")
	st := stubAccountWeb{findOrCreateRet: &models.User{ID: 1, Login: "web_1"}}
	recStart := httptest.NewRecorder()
	serveGoogleOAuthStart(cfg)(recStart, httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil))
	realState := findCookieValue(recStart.Header(), googleOAuthCookieName)
	cbURL := "/api/account/google/callback?code=z&state=wrong"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, cbURL, nil)
	req.AddCookie(&http.Cookie{Name: googleOAuthCookieName, Value: realState})
	serveGoogleOAuthCallback(cfg, &st)(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_state") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGoogleOAuthCallback_QueryError(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("f", 42), true, "cid",
		"https://callback/x", "shh")
	st := stubAccountWeb{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/callback?error=access_denied", nil)
	serveGoogleOAuthCallback(cfg, &st)(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "google_auth_failed") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGoogleOAuthCallback_HappyCreatesUserAndRedirects(t *testing.T) {
	secret := strings.Repeat("g", 44)
	validTS := httptest.NewServer(googleOAuthTestMockHandler("oAuth.user@Example.com", true, false))
	t.Cleanup(validTS.Close)

	patchGoogleOAuthEndpoints(t, validTS.URL+"/token", validTS.URL+"/userinfo")

	cfg := testGoogleOAuthMinimalCfg(secret, true, "oauth-cid",
		"https://connect.vpn-for-friends.com/api/account/google/callback",
		"oauth-secret")

	normWant, err := webuser.NormalizeEmail("oAuth.user@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	wantLogin := webuser.WebLoginFromEmail(normWant)
	user := models.User{ID: 771, Login: wantLogin}

	st := stubAccountWeb{findOrCreateRet: &user, findOrCreateCreated: true}

	var notifyCalls int32
	patchAccountWebUserRegisteredTelegramNotifier(t, func(cfg *config.Config, email string, userID int, login, ip string) {
		atomic.AddInt32(&notifyCalls, 1)
	})

	recStart := httptest.NewRecorder()
	serveGoogleOAuthStart(cfg)(recStart, httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil))
	realState := findCookieValue(recStart.Header(), googleOAuthCookieName)

	rec := httptest.NewRecorder()
	cb := "/api/account/google/callback?code=test-auth-code-placeholder&state=" + url.QueryEscape(realState)
	req := httptest.NewRequest(http.MethodGet, cb, nil)
	req.AddCookie(&http.Cookie{Name: googleOAuthCookieName, Value: realState})
	serveGoogleOAuthCallback(cfg, &st)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	to, err := url.Parse(loc)
	if err != nil || to.Path != "/account/session" {
		t.Fatalf("bad redirect location %q err=%v", loc, err)
	}
	tok := strings.TrimSpace(to.Query().Get("token"))
	if tok == "" || strings.Contains(tok, "__test_access") {
		t.Fatalf("unexpected token fragment in redirect")
	}
	claims, err := ParseAndVerifyAccountToken(secret, tok)
	if err != nil || claims.Email != normWant || claims.UserID != user.ID || claims.Login != user.Login {
		t.Fatalf("token claims %+v err=%v", claims, err)
	}
	if atomic.LoadInt32(&notifyCalls) != 1 {
		t.Fatalf("want telegram notifier 1 call got %d", notifyCalls)
	}
	if st.findOrCreateCalls != 1 {
		t.Fatalf("FindOrCreateWebUser calls=%d", st.findOrCreateCalls)
	}
}

func TestGoogleOAuthCallback_ExistingUser_NoNotifier(t *testing.T) {
	secret := strings.Repeat("h", 42)
	validTS := httptest.NewServer(googleOAuthTestMockHandler("same@Example.com", true, false))
	t.Cleanup(validTS.Close)
	patchGoogleOAuthEndpoints(t, validTS.URL+"/token", validTS.URL+"/userinfo")

	cfg := testGoogleOAuthMinimalCfg(secret, true, "cid",
		"https://connect.vpn-for-friends.com/api/account/google/callback", "secret")
	norm, _ := webuser.NormalizeEmail("same@example.com")
	wantLogin := webuser.WebLoginFromEmail(norm)
	st := stubAccountWeb{findOrCreateRet: &models.User{ID: 12, Login: wantLogin}}

	var notifyCalls int32
	patchAccountWebUserRegisteredTelegramNotifier(t, func(cfg *config.Config, email string, userID int, login, ip string) {
		atomic.AddInt32(&notifyCalls, 1)
	})

	recStart := httptest.NewRecorder()
	serveGoogleOAuthStart(cfg)(recStart, httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil))
	realState := findCookieValue(recStart.Header(), googleOAuthCookieName)

	rec := httptest.NewRecorder()
	cb := "/api/account/google/callback?code=z&state=" + url.QueryEscape(realState)
	req := httptest.NewRequest(http.MethodGet, cb, nil)
	req.AddCookie(&http.Cookie{Name: googleOAuthCookieName, Value: realState})
	serveGoogleOAuthCallback(cfg, &st)(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d", rec.Code)
	}
	if atomic.LoadInt32(&notifyCalls) != 0 {
		t.Fatal("notifier must not fire for existing user")
	}
}

func TestGoogleOAuthCallback_TokenExchangeFails(t *testing.T) {
	badTok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	t.Cleanup(badTok.Close)
	patchGoogleOAuthEndpoints(t, badTok.URL+"/token", badTok.URL+"/userinfo")

	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("i", 42), true, "cid", "https://cb/x", "sec")
	recStart := httptest.NewRecorder()
	serveGoogleOAuthStart(cfg)(recStart, httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil))
	realState := findCookieValue(recStart.Header(), googleOAuthCookieName)

	rec := httptest.NewRecorder()
	st := stubAccountWeb{}
	u := "/api/account/google/callback?code=z&state=" + url.QueryEscape(realState)
	req := httptest.NewRequest(http.MethodGet, u, nil)
	req.AddCookie(&http.Cookie{Name: googleOAuthCookieName, Value: realState})
	serveGoogleOAuthCallback(cfg, &st)(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "google_auth_failed") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGoogleOAuthCallback_EmailNotVerified(t *testing.T) {
	secret := strings.Repeat("j", 43)
	validTS := httptest.NewServer(googleOAuthTestMockHandler("unverified@example.com", false, false))
	t.Cleanup(validTS.Close)
	patchGoogleOAuthEndpoints(t, validTS.URL+"/token", validTS.URL+"/userinfo")
	cfg := testGoogleOAuthMinimalCfg(secret, true, "cid", "https://cb/x", "sec")
	recStart := httptest.NewRecorder()
	serveGoogleOAuthStart(cfg)(recStart, httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil))
	realState := findCookieValue(recStart.Header(), googleOAuthCookieName)

	rec := httptest.NewRecorder()
	cb := "/api/account/google/callback?code=z&state=" + url.QueryEscape(realState)
	req := httptest.NewRequest(http.MethodGet, cb, nil)
	req.AddCookie(&http.Cookie{Name: googleOAuthCookieName, Value: realState})
	serveGoogleOAuthCallback(cfg, &stubAccountWeb{})(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "google_email_not_verified") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRenderedAccountLogin_GoogleBlockedWhenUnavailable(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("k", 40), false, "cid", "https://x/c", "")
	out := renderedAccountLoginPageHTML(cfg)
	if strings.Contains(string(out), "/api/account/google/start") ||
		strings.Contains(string(out), "Войти с Google") {
		t.Fatal("google link must be absent when oauth unavailable")
	}
	if strings.Contains(string(out), "ACCOUNT_GOOGLE_LOGIN_BLOCK") {
		t.Fatal("placeholder should be removed from rendered login page")
	}
}

func TestRenderedAccountLogin_GoogleLinkWhenConfigured(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("m", 40), true, "cid",
		"https://connect.vpn-for-friends.com/api/account/google/callback", "sekrit")
	html := string(renderedAccountLoginPageHTML(cfg))
	if !strings.Contains(html, "Войти с Google") ||
		!strings.Contains(html, `/api/account/google/start`) ||
		!strings.Contains(html, ">или</p>") {
		t.Fatal("expected google SSO block missing")
	}
	if !strings.Contains(html, "Откройте письмо и перейдите по ссылке") ||
		!strings.Contains(html, "Введите email — мы отправим ссылку для входа без пароля") {
		t.Fatal("email magic-link copy must stay intact")
	}
}
