package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuyPageContainsAccountLink(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/buy", nil)
	rec := httptest.NewRecorder()
	serveBuy(rec, req)
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
}
