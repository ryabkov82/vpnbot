package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func accountIndexHTMLPath(t *testing.T) string {
	t.Helper()
	_, fname, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(fname), "static", "account", "index.html")
}

func TestAccountIndexStatic_LoggedOutUX(t *testing.T) {
	ru := mustRenderAccountLoginHTML(t, orderStartTestCfg(), accountLocaleRU)
	if !strings.Contains(ru, `logged-out-msg`) {
		t.Fatal("logged-out-msg id missing")
	}
	if !strings.Contains(ru, `Вы вышли из личного кабинета.`) {
		t.Fatal("logged-out copy missing")
	}
	if !strings.Contains(ru, `params.get('logged_out') === '1'`) {
		t.Fatal("logged_out query gate missing")
	}
	if !strings.Contains(ru, `window.history.replaceState({}, document.title, "/account")`) {
		t.Fatal("RU logged_out strip query via replaceState missing")
	}
	en := mustRenderAccountLoginHTML(t, orderStartTestCfg(), accountLocaleEN)
	if !strings.Contains(en, `window.history.replaceState({}, document.title, "/account?lang=en")`) {
		t.Fatal("EN logged_out must preserve lang=en in replaceState")
	}
}

func TestAccountIndexStaticHasFaviconLinks(t *testing.T) {
	b, err := os.ReadFile(accountIndexHTMLPath(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, line := range []string{
		`<link rel="icon" href="/favicon.ico" sizes="any">`,
		`<link rel="icon" type="image/png" href="/favicon-32x32.png">`,
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">`,
	} {
		if !strings.Contains(s, line) {
			t.Fatalf("account index.html missing %q", line)
		}
	}
}

func TestAccountSessionStaticHasFaviconLinks(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, line := range []string{
		`<link rel="icon" href="/favicon.ico" sizes="any">`,
		`<link rel="icon" type="image/png" href="/favicon-32x32.png">`,
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">`,
	} {
		if !strings.Contains(s, line) {
			t.Fatalf("session.html missing %q", line)
		}
	}
}

func TestGETFaviconICO(t *testing.T) {
	h := serveEmbeddedAsset("image/x-icon", faviconICO)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(faviconICO) == 0 {
		t.Fatalf("code=%d len(ico)=%d", rec.Code, len(faviconICO))
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/x-icon" {
		t.Fatalf("content-type %q", ct)
	}
}

func TestGETFavicon32PNG(t *testing.T) {
	h := serveEmbeddedAsset("image/png", favicon32PNG)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/favicon-32x32.png", nil))
	if rec.Code != http.StatusOK || len(favicon32PNG) == 0 {
		t.Fatalf("code=%d len=%d", rec.Code, len(favicon32PNG))
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatal(rec.Header().Get("Content-Type"))
	}
}

func TestGETAppleTouchIconPNG(t *testing.T) {
	h := serveEmbeddedAsset("image/png", appleTouchIconPNG)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/apple-touch-icon.png", nil))
	if rec.Code != http.StatusOK || len(appleTouchIconPNG) == 0 {
		t.Fatalf("code=%d len=%d", rec.Code, len(appleTouchIconPNG))
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatal(rec.Header().Get("Content-Type"))
	}
}
