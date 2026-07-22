package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestServeAccountServiceOrder_EN_CryptoPaymentURLUsesRUBAmount(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://bill.fix.test"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "en@buy.com", 3381, "web_en", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {
				ServiceID:    3,
				Name:         "1 месяц",
				Cost:         150,
				Period:       1,
				AllowToOrder: 1,
				Config: &models.ServiceConfig{
					Pricing: models.ServicePricingConfig{
						InternationalEnabled:     true,
						InternationalCurrency:    "USD",
						InternationalAmountCents: 200,
					},
				},
			},
		},
		serviceOrderRet: &models.UserService{ServiceID: 338, BaseServiceID: 3, Status: "NOT PAID"},
		balance:         &models.UserBalance{Balance: 0, Forecast: 150},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/service/order?lang=en",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`))
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountServiceOrderOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Amount != 150 {
		t.Fatalf("amount=%v want 150 RUB forecast", out.Amount)
	}
	if out.PaymentURL == "" {
		t.Fatal("expected payment_url")
	}
	for _, needle := range []string{"cryptocloud.cgi", "ps=cryptocloud", "amount=150", "user_id=3381"} {
		if !strings.Contains(out.PaymentURL, needle) {
			t.Fatalf("payment url missing %q: %s", needle, out.PaymentURL)
		}
	}
	if strings.Contains(out.PaymentURL, "currency=USD") || strings.Contains(out.PaymentURL, "amount=2") {
		t.Fatalf("payment url must not use USD display amount: %s", out.PaymentURL)
	}
	if !strings.Contains(out.Message, "RUB") || !strings.Contains(strings.ToLower(out.Message), "crypto") {
		t.Fatalf("message: %q", out.Message)
	}
}

func TestServeAccountServiceOrder_EN_CryptoPaymentURLFailed(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = ""
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "en@buy.com", 9, "web_en9", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 150},
		},
		serviceOrderRet: &models.UserService{ServiceID: 1, BaseServiceID: 3, Status: "NOT PAID"},
		balance:         &models.UserBalance{Forecast: 150},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/service/order?lang=en",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`))
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "crypto_payment_url_failed")
}

func TestServeAccountServiceOrder_RU_NoCryptoPaymentURL(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://bill.fix.test"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "ru@buy.com", 55, "web_ru", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 150},
		},
		serviceOrderRet: &models.UserService{ServiceID: 11, BaseServiceID: 3, Status: "NOT PAID"},
		balance:         &models.UserBalance{Forecast: 150},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`))
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountServiceOrderOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.PaymentURL) != "" {
		t.Fatalf("RU order must not auto-create payment_url: %q", out.PaymentURL)
	}
	if !strings.Contains(out.Message, "Услуга ожидает оплаты") {
		t.Fatalf("message: %q", out.Message)
	}
}

func TestRenderedAccountSession_EN_DefaultCryptoPaymentMethod(t *testing.T) {
	raw := mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleEN)
	if !strings.Contains(raw, `name="topup-payment-method" value="cryptocloud" checked`) {
		t.Fatal("EN session must default to cryptocloud payment method")
	}
	if strings.Contains(raw, `value="yookassa"`) {
		t.Fatal("EN session must not include yookassa payment method")
	}
	for _, needle := range []string{
		"Cryptocurrency",
		"/api/account/balance/topup/cryptocloud",
		"cfg.lang === 'en'",
		"formatTopupAmountInput",
		"setTopupCustomAmount",
		"parseTopupAmountInput",
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("EN session missing %q", needle)
		}
	}
	for _, forbid := range []string{
		"Bank card",
		"Card payment via the current payment gateway",
	} {
		if strings.Contains(raw, forbid) {
			t.Fatalf("EN session must not contain %q", forbid)
		}
	}
}

func TestAccountServiceOrderMessageEN_CryptoExplainsRUB(t *testing.T) {
	msg := accountServiceOrderMessageEN(false, false, 150, true)
	if !strings.Contains(msg, "150 RUB") || !strings.Contains(strings.ToLower(msg), "crypto") {
		t.Fatalf("msg: %q", msg)
	}
}
