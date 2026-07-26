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

func TestServeHappRedirect_Valid(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/redirect.html?url=happ%3A%2F%2Fadd%2Ftest", nil)
	serveHappRedirect(rec, req)
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
	for _, needle := range []string{
		"<title>Открытие Happ</title>",
		">Открытие Happ</h1>",
		"Открываем приложение Happ…",
		"Открыть Happ",
		"id=\"open-happ-btn\"",
		"URLSearchParams(window.location.search)",
		"happ://",
		"window.location.replace(target)",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("missing %q", needle)
		}
	}
	if strings.Contains(html, "happ://add/test") {
		t.Fatal("server must not interpolate query target into HTML")
	}
	for _, forbid := range []string{"vpn-for-friends.com", "friends-connect.club", "Friends Connect", "VPN for Friends"} {
		if strings.Contains(html, forbid) {
			t.Fatalf("helper must stay brand-generic: %q", forbid)
		}
	}
}

func TestServeHappRedirect_InvalidTargets(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "no_url", raw: "/redirect.html"},
		{name: "empty_url", raw: "/redirect.html?url="},
		{name: "https", raw: "/redirect.html?url=https%3A%2F%2Fexample.com"},
		{name: "javascript", raw: "/redirect.html?url=javascript%3Aalert(1)"},
		{name: "data", raw: "/redirect.html?url=data%3Atext%2Fhtml%2Ctest"},
		{name: "happ_single_colon", raw: "/redirect.html?url=happ%3Atest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			serveHappRedirect(rec, httptest.NewRequest(http.MethodGet, tc.raw, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code=%d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Invalid Happ URL") {
				t.Fatalf("body=%q", body)
			}
			if strings.Contains(body, "Открытие Happ") || strings.Contains(body, "open-happ-btn") {
				t.Fatal("helper HTML must not be served for invalid target")
			}
		})
	}
}

func TestServeHappRedirect_MethodAndPath(t *testing.T) {
	rec := httptest.NewRecorder()
	serveHappRedirect(rec, httptest.NewRequest(http.MethodPost, "/redirect.html?url=happ%3A%2F%2Fadd%2Ftest", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST code=%d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET" {
		t.Fatalf("Allow=%q", got)
	}

	rec = httptest.NewRecorder()
	serveHappRedirect(rec, httptest.NewRequest(http.MethodGet, "/redirect.html/extra?url=happ%3A%2F%2Fadd%2Ftest", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("extra path code=%d", rec.Code)
	}
}

func TestHappRedirectInlineJS_NodeCheck(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	re := regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	var parts []string
	for _, m := range re.FindAllStringSubmatch(string(happRedirectHTML), -1) {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			parts = append(parts, m[1])
		}
	}
	if len(parts) == 0 {
		t.Fatal("expected inline script")
	}
	path := t.TempDir() + "/happ-redirect-inline.js"
	if err := os.WriteFile(path, []byte(strings.Join(parts, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--check", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
}
