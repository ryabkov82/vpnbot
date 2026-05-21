package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func patchWebSalesTelegram(t *testing.T, fn func(cfg *config.Config, text string, logPrefix string)) {
	t.Helper()
	old := webSalesTelegramSend
	webSalesTelegramSend = fn
	t.Cleanup(func() { webSalesTelegramSend = old })
}

func TestServePublicOrderStart_SuccessCallsWebSalesTelegramAfterEmail(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error { return nil })
	var got int
	var last string
	patchWebSalesTelegram(t, func(cfg *config.Config, text string, prefix string) {
		got++
		last = text
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	app := &stubOrderStartApp{svc: &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1}}
	h := servePublicOrderStart(cfg, app, rl)
	body := `{"email":"user@example.com","service_id":3,"contact":"@me","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(body))
	req.Header.Set("X-Forwarded-For", "198.51.100.2, 10.0.0.1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	if got != 1 {
		t.Fatalf("telegram calls=%d", got)
	}
	if !strings.Contains(last, "🟡 Web order started") || !strings.Contains(last, "user@example.com") {
		t.Fatalf("unexpected telegram body: %s", last)
	}
	if !strings.Contains(last, "@me") || !strings.Contains(last, "198.51.100.2") {
		t.Fatalf("contact/ip missing: %s", last)
	}
}

func TestServePublicOrderStart_HoneypotDoesNotCallWebSalesTelegram(t *testing.T) {
	var got int
	patchWebSalesTelegram(t, func(cfg *config.Config, text string, prefix string) { got++ })
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error { return nil })
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := servePublicOrderStart(cfg, &stubOrderStartApp{}, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(`{"email":"a@b.c","service_id":1,"website":"spam"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || got != 0 {
		t.Fatalf("code=%d tg=%d", rec.Code, got)
	}
}

func TestServePublicOrderStart_EmailSendFailedDoesNotCallWebSalesTelegram(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return io.ErrUnexpectedEOF
	})
	var got int
	patchWebSalesTelegram(t, func(cfg *config.Config, text string, prefix string) { got++ })
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	app := &stubOrderStartApp{svc: &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1}}
	h := servePublicOrderStart(cfg, app, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(`{"email":"u@e.com","service_id":3,"website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || got != 0 {
		t.Fatalf("code=%d tg=%d", rec.Code, got)
	}
}

func TestServeBuyPay_SuccessCallsOrderCreatedTelegram(t *testing.T) {
	var got int
	var last string
	patchWebSalesTelegram(t, func(cfg *config.Config, text string, prefix string) {
		got++
		last = text
	})
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error { return nil })
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example"
	secret := cfg.WebSales.OrderTokenSecret
	startTok, err := CreateOrderStartToken(secret, "buyer@example.com", 3, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &stubPayApp{
		user:  &models.User{ID: 510, Login: "web_x"},
		order: &models.UserService{ServiceID: 334, Status: "NOT PAID"},
		svc:   &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1},
	}
	used := NewUsedStartTokenStore()
	h := serveBuyPay(cfg, app, used)
	req := httptest.NewRequest(http.MethodGet, "/buy/pay?token="+startTok, nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got != 1 {
		t.Fatalf("telegram calls=%d", got)
	}
	if !strings.Contains(last, "🧾 Web order created") || !strings.Contains(last, "SHM user_id: 510") || !strings.Contains(last, "203.0.113.9") {
		t.Fatalf("body: %s", last)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/buy/pay?token="+startTok, nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict || got != 1 {
		t.Fatalf("second: code=%d tg_calls=%d", rec2.Code, got)
	}
}

func TestServePublicOrderStatus_ACTIVE_TelegramNotifyOncePerUserService(t *testing.T) {
	t.Cleanup(resetWebSalesOrderActiveNotifiedForTest)
	resetWebSalesOrderActiveNotifiedForTest()

	var got int
	var bodies []string
	patchWebSalesTelegram(t, func(cfg *config.Config, text string, prefix string) {
		got++
		bodies = append(bodies, text)
	})

	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.Status = "ACTIVE"
	us.Category = "vpn-mz-1"
	us.Name = "Marz plan"
	us.KeyMarzban = models.UserKeyMarzban{SubscriptionURL: "https://sub.example/x"}
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	url := "/api/public/order/status?token=" + tok
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("iter %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if got != 1 {
		t.Fatalf("want 1 telegram, got %d: %#v", got, bodies)
	}
	if !strings.Contains(bodies[0], "✅ Web order paid") || !strings.Contains(bodies[0], "https://sub.example/x") {
		t.Fatalf("bad body: %s", bodies[0])
	}
}

func TestServePublicOrderStart_TelegramAPIErrorStillOK(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error { return nil })
	oldHTTP := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":false}`))),
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = oldHTTP })
	cfg := orderStartTestCfg()
	cfg.Telegram.Token = "dummy-token"
	cfg.Telegram.LeadsChatID = 1
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	app := &stubOrderStartApp{svc: &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1}}
	h := servePublicOrderStart(cfg, app, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(`{"email":"ok@e.com","service_id":3,"website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var out publicOrderStartOKJSON
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if out.Status != "email_sent" {
		t.Fatalf("%#v", out)
	}
}

func TestServeBuyPay_TelegramAPIErrorStillOK(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error { return nil })
	oldHTTP := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader(`{"ok":false}`)),
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = oldHTTP })
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example"
	cfg.Telegram.Token = "dummy-token"
	cfg.Telegram.LeadsChatID = 1
	secret := cfg.WebSales.OrderTokenSecret
	startTok, err := CreateOrderStartToken(secret, "buyer@example.com", 3, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &stubPayApp{
		user:  &models.User{ID: 510, Login: "web_x"},
		order: &models.UserService{ServiceID: 334, Status: "NOT PAID"},
		svc:   &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1},
	}
	h := serveBuyPay(cfg, app, NewUsedStartTokenStore())
	req := httptest.NewRequest(http.MethodGet, "/buy/pay?token="+startTok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestServePublicOrderStatus_TelegramAPIErrorStillOK(t *testing.T) {
	t.Cleanup(resetWebSalesOrderActiveNotifiedForTest)
	resetWebSalesOrderActiveNotifiedForTest()
	oldHTTP := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":false}`))),
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = oldHTTP })
	cfg := orderStartTestCfg()
	cfg.Telegram.Token = "dummy-token"
	cfg.Telegram.LeadsChatID = 1
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.Status = "ACTIVE"
	us.Category = "vpn-mz-1"
	us.KeyMarzban = models.UserKeyMarzban{SubscriptionURL: "https://sub.example/x"}
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestClientIPFromRequest_XForwardedForFirst(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", " 203.0.113.1 , 10.0.0.1 ")
	if got := ClientIPFromRequest(req); got != "203.0.113.1" {
		t.Fatalf("got %q", got)
	}
}
