package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestServeAccountBalanceTopup_FCSharedYooKassaPaySystem(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.fc.test"
	cfg.Brand.ID = "fc"
	cfg.Brand.YooKassaPaySystem = "yookassa"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "fc", "a@b.c", 55, "web_fc_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	serveAccountBalanceTopup(cfg, &stubAccountWeb{}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/account/balance/topup",
			strings.NewReader(`{"token":"`+tok+`","amount":200}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountBalanceTopupOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(out.PaymentURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("ps") != "yookassa" || u.Query().Get("brand_id") != "fc" {
		t.Fatalf("want ps=yookassa brand_id=fc, got %s", out.PaymentURL)
	}
}

func TestServeAccountBalanceTopup_InvalidBrandIDFailClosed(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.test"
	cfg.Brand.ID = "Bad ID"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "Bad ID", "a@b.c", 9, "web_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	serveAccountBalanceTopup(cfg, &stubAccountWeb{}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/account/balance/topup",
			strings.NewReader(`{"token":"`+tok+`","amount":100}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "payment_url_failed")
}

func TestServeAccountBalanceTopup_EmptyYooKassaPaySystemFailClosed(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.test"
	cfg.Brand.YooKassaPaySystem = ""
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@b.c", 9, "web_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	serveAccountBalanceTopup(cfg, &stubAccountWeb{}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/account/balance/topup",
			strings.NewReader(`{"token":"`+tok+`","amount":100}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "payment_url_failed")
}
