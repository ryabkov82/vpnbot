package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServeAccountBalanceTopupCrypto_SuccessPaymentURL(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://bill.fix.test"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 701, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountBalanceTopupCrypto(cfg, &stubAccountWeb{})
	body := `{"token":"` + tok + `","amount":150}`
	req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup/cryptocloud", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountBalanceTopupOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "payment_required" || out.Amount != 150 || out.PaymentURL == "" {
		t.Fatalf("%#v", out)
	}
	for _, needle := range []string{"cryptocloud.cgi", "ps=cryptocloud", "701", "amount=150"} {
		if !strings.Contains(out.PaymentURL, needle) {
			t.Fatalf("payment url missing %q: %s", needle, out.PaymentURL)
		}
	}
	if !strings.Contains(out.Message, "При частичной оплате") || !strings.Contains(out.Message, "обратитесь в поддержку") {
		t.Fatalf("message: %q", out.Message)
	}
}

func TestRenderedAccountSessionPageIncludesCryptoPaymentMethod(t *testing.T) {
	raw := string(renderedAccountSessionPageHTML(orderStartTestCfg()))
	for _, needle := range []string{
		`id="topup-payment-methods"`,
		`name="topup-payment-method" value="yookassa" checked`,
		`name="topup-payment-method" value="cryptocloud"`,
		`Криптовалюта`,
		`Оплата через Trybit: USDT, TON и другие доступные валюты`,
		`При частичной оплате доступ может не активироваться автоматически. Если платеж не зачислился, обратитесь в поддержку.`,
		`Telegram @friends_connect_support`,
		`support@vpn-for-friends.com`,
		`function selectedTopupBalanceURL()`,
		`/api/account/balance/topup/cryptocloud`,
		`var topupEndpoint = selectedTopupBalanceURL()`,
		`non_json_response`,
		`topup non-json response`,
		`Не удалось создать счет Trybit`,
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("rendered session missing %q", needle)
		}
	}
	for _, forbid := range []string{"аноним", "обход блокировок", "без ограничений"} {
		if strings.Contains(strings.ToLower(raw), forbid) {
			t.Fatalf("rendered session contains risky copy %q", forbid)
		}
	}
}
