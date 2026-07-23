package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestServePaymentReturn_PublicNoAuth(t *testing.T) {
	cfg := &config.Config{}
	cfg.Brand.Name = "VPN for Friends"
	h := servePaymentReturn(cfg)

	req := httptest.NewRequest(http.MethodGet, "/payment/return", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "VPN for Friends") {
		t.Fatalf("missing brand name: %s", body)
	}
	if !strings.Contains(body, `href="/account"`) || !strings.Contains(body, "Перейти в личный кабинет") {
		t.Fatalf("missing account CTA: %s", body)
	}
	if !strings.Contains(body, "Платёж принят в обработку") {
		t.Fatalf("missing processing message: %s", body)
	}
	for _, banned := range []string{
		"успешно зачислен",
		"платёж подтверждён",
		"платеж подтвержден",
		"оплата прошла успешно",
		"payment confirmed",
		"payment successful",
	} {
		if strings.Contains(strings.ToLower(body), banned) {
			t.Fatalf("must not claim payment success (%q): %s", banned, body)
		}
	}
	if !strings.Contains(body, `/favicon.ico`) || !strings.Contains(body, `/favicon-32x32.png`) {
		t.Fatalf("missing favicon links used by account templates: %s", body)
	}
}

func TestServePaymentReturn_FCBranding(t *testing.T) {
	cfg := &config.Config{}
	cfg.Brand.Name = "Friends Connect"
	rec := httptest.NewRecorder()
	servePaymentReturn(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/payment/return/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Friends Connect") {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "VPN for Friends") {
		t.Fatal("FC page must not show VFF name")
	}
}

func TestServePaymentReturn_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	servePaymentReturn(&config.Config{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/payment/return", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d", rec.Code)
	}
}
