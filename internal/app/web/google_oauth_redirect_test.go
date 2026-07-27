package web

import (
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

func multiHostVFFGoogleCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("r", 40), true,
		"cid.apps.googleusercontent.com",
		"https://connect.vpn-for-friends.com/api/account/google/callback",
		"client-secret")
	cfg.Brand.AllowedHosts = []string{
		"connect.vpn-for-friends.com",
		"vff.portalbase.link",
	}
	return cfg
}

func TestResolveGoogleOAuthRedirectURL_CanonicalHost(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "connect.vpn-for-friends.com"
	got, err := resolveGoogleOAuthRedirectURL(cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://connect.vpn-for-friends.com/api/account/google/callback"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveGoogleOAuthRedirectURL_PortalBaseHost(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "vff.portalbase.link"
	got, err := resolveGoogleOAuthRedirectURL(cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://vff.portalbase.link/api/account/google/callback"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveGoogleOAuthRedirectURL_MixedCaseAndPort(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "VFF.PortalBase.Link:443"
	got, err := resolveGoogleOAuthRedirectURL(cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://vff.portalbase.link/api/account/google/callback" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveGoogleOAuthRedirectURL_TrailingDNSDot(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "connect.vpn-for-friends.com."
	got, err := resolveGoogleOAuthRedirectURL(cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://connect.vpn-for-friends.com/api/account/google/callback" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveGoogleOAuthRedirectURL_UnknownHostRejected(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "evil.example"
	_, err := resolveGoogleOAuthRedirectURL(cfg, req)
	if _, ok := err.(errGoogleOAuthInvalidHost); !ok {
		t.Fatalf("want invalid host error, got %v", err)
	}
}

func TestResolveGoogleOAuthRedirectURL_XForwardedHostIgnored(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "vff.portalbase.link"
	req.Header.Set("X-Forwarded-Host", "connect.vpn-for-friends.com")
	got, err := resolveGoogleOAuthRedirectURL(cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://vff.portalbase.link/api/account/google/callback" {
		t.Fatalf("X-Forwarded-Host must not win: got %q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req2.Host = "evil.example"
	req2.Header.Set("X-Forwarded-Host", "vff.portalbase.link")
	_, err = resolveGoogleOAuthRedirectURL(cfg, req2)
	if _, ok := err.(errGoogleOAuthInvalidHost); !ok {
		t.Fatalf("spoofed X-Forwarded-Host must not authorize host: %v", err)
	}
}

func TestGoogleOAuthStart_PortalBaseRedirectURI(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "vff.portalbase.link"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "connect.vpn-for-friends.com")
	serveGoogleOAuthStart(cfg)(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	want := "https://vff.portalbase.link/api/account/google/callback"
	if loc.Query().Get("redirect_uri") != want {
		t.Fatalf("redirect_uri=%q want %q", loc.Query().Get("redirect_uri"), want)
	}
	var oauthCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == googleOAuthCookieName {
			oauthCookie = c
			break
		}
	}
	if oauthCookie == nil {
		t.Fatal("missing state cookie")
	}
	if oauthCookie.Domain != "" {
		t.Fatalf("host-only cookie must not set Domain, got %q", oauthCookie.Domain)
	}
	if oauthCookie.Path != "/" || !oauthCookie.HttpOnly || !oauthCookie.Secure ||
		oauthCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags %+v", oauthCookie)
	}
}

func TestGoogleOAuthStart_CanonicalHostUnchanged(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "connect.vpn-for-friends.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	serveGoogleOAuthStart(cfg)(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loc.Query().Get("redirect_uri") != cfg.WebAccount.GoogleRedirectURL {
		t.Fatalf("redirect_uri=%q", loc.Query().Get("redirect_uri"))
	}
}

func TestGoogleOAuthStart_UnknownHostRejected(t *testing.T) {
	cfg := multiHostVFFGoogleCfg(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	req.Host = "evil.example"
	serveGoogleOAuthStart(cfg)(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_host") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGoogleOAuthCallback_TokenExchangeUsesCallbackHost(t *testing.T) {
	secret := strings.Repeat("t", 44)
	var gotRedirect atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		switch path {
		case "/token":
			_ = r.ParseForm()
			gotRedirect.Store(r.Form.Get("redirect_uri"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"tok","token_type":"Bearer"}`)
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"email":"portal@example.com","email_verified":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	patchGoogleOAuthEndpoints(t, ts.URL+"/token", ts.URL+"/userinfo")

	cfg := testGoogleOAuthMinimalCfg(secret, true, "cid",
		"https://connect.vpn-for-friends.com/api/account/google/callback", "sec")
	cfg.Brand.AllowedHosts = []string{"connect.vpn-for-friends.com", "vff.portalbase.link"}

	norm, err := webuser.NormalizeEmail("portal@example.com")
	if err != nil {
		t.Fatal(err)
	}
	st := stubAccountWeb{
		findOrCreateRet:     &models.User{ID: 42, Login: webuser.WebLoginFromEmail(norm)},
		findOrCreateCreated: true,
	}

	recStart := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodGet, "/api/account/google/start", nil)
	startReq.Host = "vff.portalbase.link"
	startReq.Header.Set("X-Forwarded-Proto", "https")
	serveGoogleOAuthStart(cfg)(recStart, startReq)
	if recStart.Code != http.StatusFound {
		t.Fatalf("start code=%d", recStart.Code)
	}
	startLoc, _ := url.Parse(recStart.Header().Get("Location"))
	startRedirect := startLoc.Query().Get("redirect_uri")
	if startRedirect != "https://vff.portalbase.link/api/account/google/callback" {
		t.Fatalf("start redirect_uri=%q", startRedirect)
	}
	state := findCookieValue(recStart.Header(), googleOAuthCookieName)

	rec := httptest.NewRecorder()
	cb := "/api/account/google/callback?code=z&state=" + url.QueryEscape(state)
	req := httptest.NewRequest(http.MethodGet, cb, nil)
	req.Host = "vff.portalbase.link"
	req.Header.Set("X-Forwarded-Host", "connect.vpn-for-friends.com")
	req.AddCookie(&http.Cookie{Name: googleOAuthCookieName, Value: state})
	serveGoogleOAuthCallback(cfg, &st)(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback code=%d body=%s", rec.Code, rec.Body.String())
	}
	exchanged, _ := gotRedirect.Load().(string)
	if exchanged != startRedirect {
		t.Fatalf("token exchange redirect_uri=%q want %q (must match authorization)", exchanged, startRedirect)
	}
}
