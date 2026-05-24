package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
