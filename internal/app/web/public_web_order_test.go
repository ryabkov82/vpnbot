package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/email"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestOrderStartTokenExpires(t *testing.T) {
	secret := "secret-secret-secret-secret-sx"
	tok, err := CreateOrderStartToken(secret, "a@b.c", 1, 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	_, err = ParseAndVerifyOrderStartToken(secret, tok)
	if err != ErrOrderTokenExpired {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestCreateAndVerifyOrderToken(t *testing.T) {
	secret := "order-secret-order-secret-order-"
	tok, err := CreateOrderToken(secret, "u@x.y", 2, 99, 1001, 199.5, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ParseAndVerifyOrderToken(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if cl.UserID != 99 || cl.UserServiceID != 1001 || cl.Amount != 199.5 {
		t.Fatalf("%#v", cl)
	}
	_, err = ParseAndVerifyOrderToken("wrong", tok)
	if err != ErrOrderTokenSignature {
		t.Fatalf("got %v", err)
	}
}

func TestParseOrderStartAsOrderTokenFails(t *testing.T) {
	secret := "order-secret-order-secret-order-"
	st, err := CreateOrderStartToken(secret, "a@b.c", 2, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyOrderToken(secret, st)
	if err != ErrOrderTokenType {
		t.Fatalf("want type err, got %v", err)
	}
}

func TestWebSalesOrderTokenTTLDefault(t *testing.T) {
	if webSalesOrderTokenTTL(nil) != 24*time.Hour {
		t.Fatal("nil cfg ttl")
	}
	cfg := &config.Config{}
	cfg.WebSales.OrderTokenTTLHours = 48
	if webSalesOrderTokenTTL(cfg) != 48*time.Hour {
		t.Fatal("custom ttl")
	}
}

type stubOrderStartApp struct {
	svc    *models.Service
	svcErr error
}

func (s *stubOrderStartApp) GetServices() ([]models.Service, error) {
	if s.svcErr != nil {
		return nil, s.svcErr
	}
	if s.svc != nil {
		return []models.Service{*s.svc}, nil
	}
	return nil, nil
}

func (s *stubOrderStartApp) GetServiceByID(serviceID int) (*models.Service, error) {
	if s.svcErr != nil {
		return nil, s.svcErr
	}
	if s.svc != nil && s.svc.ServiceID == serviceID {
		return s.svc, nil
	}
	return nil, errors.New("service 9 not found")
}

func orderStartTestCfg() *config.Config {
	cfg := &config.Config{}
	cfg.WebSales.Enabled = true
	cfg.WebSales.OrderTokenSecret = "order-token-secret-order-token-sec"
	cfg.WebSales.PublicBaseURL = "https://shop.example"
	cfg.Email.Enabled = true
	cfg.Email.SMTPHost = "smtp.test"
	cfg.Email.SMTPPort = 587
	cfg.Email.SMTPUsername = "u"
	cfg.Email.SMTPPassword = "pw"
	cfg.Email.FromEmail = "noreply@test"
	return cfg
}

func patchSMTP(t *testing.T, fn func(addr string, a smtp.Auth, from string, to []string, msg []byte) error) {
	old := email.SendMail
	email.SendMail = fn
	t.Cleanup(func() { email.SendMail = old })
}

func TestServePublicOrderStart_Disabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.WebSales.Enabled = false
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := servePublicOrderStart(cfg, &stubOrderStartApp{svc: &models.Service{ServiceID: 1, Name: "X", Cost: 1, Period: 1}}, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(`{"email":"a@b.c","service_id":1,"website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestServePublicOrderStart_Honeypot(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Email.Enabled = false
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := servePublicOrderStart(cfg, &stubOrderStartApp{}, rl)
	body := `{"email":"a@b.c","service_id":1,"website":"spam"}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestServePublicOrderStart_HoneypotDoesNotCallSMTP(t *testing.T) {
	smtpCalls := 0
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		smtpCalls++
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := servePublicOrderStart(cfg, &stubOrderStartApp{}, rl)
	body := `{"email":"a@b.c","service_id":1,"website":"spam"}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if smtpCalls != 0 {
		t.Fatalf("honeypot must not send email, smtp calls=%d", smtpCalls)
	}
}

func TestServePublicOrderStart_OKSendsEmailNoPayURL(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error { return nil })
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	app := &stubOrderStartApp{svc: &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1}}
	h := servePublicOrderStart(cfg, app, rl)
	body := `{"email":"user@example.com","service_id":3,"website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(body))
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var out publicOrderStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "email_sent" || out.Message == "" {
		t.Fatalf("%#v", out)
	}
	if out.PayURL != "" {
		t.Fatalf("pay_url must be empty outside development, got %q", out.PayURL)
	}
}

func TestServePublicOrderStart_DevReturnsPayURL(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error { return nil })
	cfg := orderStartTestCfg()
	cfg.Env = "development"
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	app := &stubOrderStartApp{svc: &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1}}
	h := servePublicOrderStart(cfg, app, rl)
	body := `{"email":"user@example.com","service_id":3,"website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var out publicOrderStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.PayURL, "https://shop.example/buy/pay?token=") {
		t.Fatalf("dev pay_url: %q", out.PayURL)
	}
}

func TestServePublicOrderStart_EmailBodyUsesPublicBaseNotPremium(t *testing.T) {
	var got []byte
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		got = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	cfg.PremiumConnectBaseURL = "https://connect.vpn-for-friends.com/premium-connect"
	cfg.WebSales.PublicBaseURL = "https://connect.vpn-for-friends.com"
	cfg.Email.Enabled = true
	cfg.Email.SMTPHost = "smtp.test"
	cfg.Email.SMTPUsername = "u"
	cfg.Email.SMTPPassword = "p"
	cfg.Email.FromEmail = "noreply@test"
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	app := &stubOrderStartApp{svc: &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1}}
	h := servePublicOrderStart(cfg, app, rl)
	body := `{"email":"user@example.com","service_id":3,"website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	s := string(got)
	if !strings.Contains(s, "https://connect.vpn-for-friends.com/buy/pay?token=") {
		t.Fatalf("email body missing correct pay link: %s", s[:min(500, len(s))])
	}
	if strings.Contains(s, "/premium-connect/") {
		t.Fatalf("email must not use premium path")
	}
}

func TestServePublicOrderStart_EmailUnavailable(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Email.Enabled = false
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := servePublicOrderStart(cfg, &stubOrderStartApp{svc: &models.Service{ServiceID: 1, Name: "X", Cost: 1, Period: 1}}, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(`{"email":"a@b.c","service_id":1,"website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "email_unavailable")
}

func TestServePublicOrderStart_EmailSendFailed(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return errors.New("smtp down")
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	app := &stubOrderStartApp{svc: &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1}}
	h := servePublicOrderStart(cfg, app, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/public/order/start", strings.NewReader(`{"email":"u@e.com","service_id":3,"website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "email_send_failed")
}

type stubPayApp struct {
	user     *models.User
	userErr  error
	order    *models.UserService
	orderErr error
	svc      *models.Service
}

func (s *stubPayApp) FindOrCreateWebUser(email string) (*models.User, error) {
	if s.userErr != nil {
		return nil, s.userErr
	}
	return s.user, nil
}

func (s *stubPayApp) ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error) {
	if s.orderErr != nil {
		return nil, s.orderErr
	}
	return s.order, nil
}

func (s *stubPayApp) GetServiceByID(serviceID int) (*models.Service, error) {
	return s.svc, nil
}

func TestServeBuyPay_CreatesOrderAndPaymentURL(t *testing.T) {
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
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "yookassa.cgi") || !strings.Contains(body, "Перейти к оплате") {
		t.Fatalf("missing payment UI: %s", body[:min(400, len(body))])
	}
	req2 := httptest.NewRequest(http.MethodGet, "/buy/pay?token="+startTok, nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second request: want 409, got %d", rec2.Code)
	}
	b2 := rec2.Body.String()
	if !strings.Contains(b2, "email") {
		t.Fatalf("409 page should mention email: %s", b2[:min(500, len(b2))])
	}
}

func TestServeBuyPay_StatusEmailFailsStillOK(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return errors.New("smtp down")
	})
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
	h := serveBuyPay(cfg, app, NewUsedStartTokenStore())
	req := httptest.NewRequest(http.MethodGet, "/buy/pay?token="+startTok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "yookassa.cgi") {
		t.Fatalf("expected payment page despite status email failure")
	}
}

type stubStatusApp struct {
	us *models.UserService
}

func (s *stubStatusApp) GetUserService(serviceID string) (*models.UserService, error) {
	return s.us, nil
}

func statusUSMatchToken() *models.UserService {
	return &models.UserService{
		UserID:        10,
		ServiceID:     20,
		BaseServiceID: 1,
	}
}

func TestServePublicOrderStatus_ACTIVE_VPNMarzban_SubscriptionURL(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.Status = "ACTIVE"
	us.Category = "vpn-mz-1"
	us.KeyMarzban = models.UserKeyMarzban{
		SubscriptionURL: "https://sub.example/v1/x",
		Links:           []string{"https://raw-never-leak.example/"},
	}
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var out publicOrderStatusOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Paid || out.Status != "ACTIVE" || out.Message != publicOrderStatusMsgVPNReady {
		t.Fatalf("%#v", out)
	}
	if out.ConnectURL != us.KeyMarzban.SubscriptionURL {
		t.Fatalf("connect_url=%q", out.ConnectURL)
	}
	if out.ConnectTitle != publicOrderStatusConnectTitle {
		t.Fatalf("connect_title=%q", out.ConnectTitle)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "raw-never-leak") {
		t.Fatal("response must not expose KeyMarzban.Links")
	}
}

func TestServePublicOrderStatus_ACTIVE_VPNMarzban_NoSubscriptionURL(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.Status = "ACTIVE"
	us.Category = "vpn-mz-1"
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out publicOrderStatusOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Paid || out.ConnectURL != "" || out.ConnectTitle != "" {
		t.Fatalf("%#v", out)
	}
	if out.Message != publicOrderStatusMsgVPNProvisioning {
		t.Fatalf("msg=%q", out.Message)
	}
}

func TestServePublicOrderStatus_ACTIVE_NonVPNMessage(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.Status = "ACTIVE"
	us.Category = "premium-antiblock"
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out publicOrderStatusOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Paid || out.ConnectURL != "" {
		t.Fatalf("%#v", out)
	}
	if out.Message != publicOrderStatusMsgPremiumLater {
		t.Fatalf("msg=%q", out.Message)
	}
}

func TestServePublicOrderStatus_NotPaid(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.Status = "NOT PAID"
	us.Category = "vpn-mz-1"
	us.KeyMarzban = models.UserKeyMarzban{SubscriptionURL: "https://should-not-appear.example/"}
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out publicOrderStatusOKJSON
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if out.Paid || out.Status != "NOT PAID" || out.ConnectURL != "" {
		t.Fatalf("%#v", out)
	}
}

func TestServePublicOrderStatus_UserMismatch(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.UserID = 999
	us.Status = "ACTIVE"
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestServePublicOrderStatus_UserServiceIDMismatch(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.ServiceID = 999
	us.Status = "ACTIVE"
	us.Category = "vpn-mz-1"
	us.KeyMarzban = models.UserKeyMarzban{SubscriptionURL: "https://leak.example/"}
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServePublicOrderStatus_BaseServiceIDMismatch(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateOrderToken(secret, "u@e.e", 1, 10, 20, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	us := statusUSMatchToken()
	us.BaseServiceID = 2
	us.Status = "ACTIVE"
	app := &stubStatusApp{us: us}
	h := servePublicOrderStatus(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/order/status?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestBuyStatusPageServes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/buy/status", nil)
	rec := httptest.NewRecorder()
	serveBuyStatus(rec, req)
	body := rec.Body.Bytes()
	if rec.Code != http.StatusOK || !bytes.Contains(body, []byte("Статус оплаты")) {
		t.Fatalf("bad response %d", rec.Code)
	}
	if !bytes.Contains(body, []byte(`id="success-with-connect"`)) || !bytes.Contains(body, []byte(`id="btn-copy-connect"`)) {
		t.Fatal("status page missing connect UI blocks")
	}
}
