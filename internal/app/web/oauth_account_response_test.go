package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRespondAccountEmailAlreadyLinked_BrowserRedirect(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	respondAccountEmailAlreadyLinked(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/account?error=email_already_linked" {
		t.Fatalf("location=%q", loc)
	}
}

func TestRespondAccountEmailAlreadyLinked_JSON409(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept", "application/json")
	respondAccountEmailAlreadyLinked(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != accountErrorEmailAlreadyLinked {
		t.Fatalf("error field=%v", body)
	}
}

func TestRespondLinkEmailAlreadyLinked_RedirectPreservesToken(t *testing.T) {
	t.Parallel()
	const linkTok = "eyJhbGciOiJIUzI1NiJ9.test"
	rec := httptest.NewRecorder()
	respondLinkEmailAlreadyLinked(rec, httptest.NewRequest(http.MethodGet, "/", nil), linkTok)
	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/account/link" || u.Query().Get("token") != linkTok || u.Query().Get("err") != "email_already_linked" {
		t.Fatalf("location=%q", loc)
	}
	if u.Query().Get("error") != "" {
		t.Fatalf("must use err= not error=, location=%q", loc)
	}
}
