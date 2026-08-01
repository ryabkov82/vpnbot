package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeAccountCSS_GET(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account/assets/account.css", nil)
	serveAccountCSS(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/css; charset=utf-8" {
		t.Fatalf("Content-Type=%q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	etag := rec.Header().Get("ETag")
	if etag == "" || !strings.HasPrefix(etag, `"`) {
		t.Fatalf("ETag=%q", etag)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatalf("Cache-Control=%q", rec.Header().Get("Cache-Control"))
	}
	body := rec.Body.String()
	if len(body) == 0 {
		t.Fatal("empty CSS body")
	}
	for _, needle := range []string{
		`html[data-brand="fc"]`,
		`--account-bg`,
		`--account-surface`,
		`--account-accent`,
		`@media (max-width: 576px)`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("CSS missing %q", needle)
		}
	}
	for _, forbid := range []string{
		"Кончится лето",
		"Первый месяц",
		"31 августа 2026",
		"100 ₽",
	} {
		if strings.Contains(body, forbid) {
			t.Fatalf("CSS must not contain promo string %q", forbid)
		}
	}
}

func TestServeAccountCSS_HEAD(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/account/assets/account.css", nil)
	serveAccountCSS(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status=%d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD body must be empty, got %d bytes", rec.Body.Len())
	}
	if rec.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatal("HEAD Content-Type")
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("HEAD ETag missing")
	}
}

func TestServeAccountCSS_NotModified(t *testing.T) {
	base := httptest.NewRecorder()
	serveAccountCSS(base, httptest.NewRequest(http.MethodGet, "/account/assets/account.css", nil))
	etag := base.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no etag")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account/assets/account.css", nil)
	req.Header.Set("If-None-Match", etag)
	serveAccountCSS(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status=%d want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatal("304 must have empty body")
	}
}

func TestServeAccountCSS_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	serveAccountCSS(rec, httptest.NewRequest(http.MethodPost, "/account/assets/account.css", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
	allow := rec.Header().Get("Allow")
	if !strings.Contains(allow, http.MethodGet) || !strings.Contains(allow, http.MethodHead) {
		t.Fatalf("Allow=%q", allow)
	}
}

func TestServeAccountCSS_RegisteredOnMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/account", serveAccount(orderStartTestCfg()))
	mux.HandleFunc("/account/", serveAccount(orderStartTestCfg()))
	mux.HandleFunc("/account/assets/account.css", serveAccountCSS)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/account/assets/account.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("mux route status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "--account-bg") {
		t.Fatal("mux did not serve embedded CSS")
	}
}
