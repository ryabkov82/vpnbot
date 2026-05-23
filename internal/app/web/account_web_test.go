package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/models"
)

type stubAccountWeb struct {
	userByLogin    *models.User
	userByLoginErr error
	services       []models.UserService
	servicesErr    error
	single         map[int]*models.UserService

	balance    *models.UserBalance
	balanceErr error

	shmServices    []models.Service
	shmServicesErr error

	svcByID     map[int]*models.Service
	getSvcByErr error

	serviceOrderRet *models.UserService
	serviceOrderErr error
	serviceOrderUID int
	serviceOrderSID int

	deleteCalls             int
	deleteErr               error
	deleteLastUID           int
	deleteLastUserServiceID string
}

func (s *stubAccountWeb) GetUserBalanceByUserID(userID int) (*models.UserBalance, error) {
	if s.balanceErr != nil {
		return nil, s.balanceErr
	}
	return s.balance, nil
}

func (s *stubAccountWeb) GetUserByLogin(login string) (*models.User, error) {
	if s.userByLoginErr != nil {
		return nil, s.userByLoginErr
	}
	return s.userByLogin, nil
}
func (s *stubAccountWeb) GetUserServicesByUserID(userID int) ([]models.UserService, error) {
	if s.servicesErr != nil {
		return nil, s.servicesErr
	}
	return s.services, nil
}

func (s *stubAccountWeb) GetUserService(serviceID string) (*models.UserService, error) {
	if s.single == nil {
		return nil, nil
	}
	id, _ := strconv.Atoi(serviceID)
	us := s.single[id]
	return us, nil
}

func (s *stubAccountWeb) GetServices() ([]models.Service, error) {
	if s.shmServicesErr != nil {
		return nil, s.shmServicesErr
	}
	return s.shmServices, nil
}

func (s *stubAccountWeb) GetServiceByID(serviceID int) (*models.Service, error) {
	if s.getSvcByErr != nil {
		return nil, s.getSvcByErr
	}
	if s.svcByID == nil {
		return nil, nil
	}
	return s.svcByID[serviceID], nil
}

func (s *stubAccountWeb) ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error) {
	s.serviceOrderUID = userID
	s.serviceOrderSID = serviceID
	if s.serviceOrderErr != nil {
		return nil, s.serviceOrderErr
	}
	return s.serviceOrderRet, nil
}

func (s *stubAccountWeb) DeleteUserServiceByUserID(userID int, userServiceID string) error {
	s.deleteCalls++
	s.deleteLastUID = userID
	s.deleteLastUserServiceID = userServiceID
	return s.deleteErr
}

func TestServeAccountLoginStart_Honeypot(t *testing.T) {
	var smtpN int
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		smtpN++
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := serveAccountLoginStart(cfg, &stubAccountWeb{}, rl)
	body := `{"email":"a@b.c","website":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || smtpN != 0 {
		t.Fatalf("code=%d smtp=%d body=%s", rec.Code, smtpN, rec.Body.String())
	}
	var out accountLoginStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || out.Status != "email_sent" {
		t.Fatalf("%#v err=%v", out, err)
	}
}

func TestServeAccountLoginStart_UnknownEmailNoSMTP(t *testing.T) {
	var smtpN int
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		smtpN++
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	st := &stubAccountWeb{}
	h := serveAccountLoginStart(cfg, st, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"nouser@test.com","website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || smtpN != 0 {
		t.Fatalf("code=%d smtp=%d", rec.Code, smtpN)
	}
	var out accountLoginStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || out.Status != "email_sent" {
		t.Fatalf("%#v", out)
	}
}

func TestServeAccountLoginStart_KnownEmailSendsMail(t *testing.T) {
	var gotMail []byte
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotMail = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	cfg.WebSales.PublicBaseURL = "https://shop.example"
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	u := &models.User{ID: 511, Login: "web_abc"}
	st := &stubAccountWeb{userByLogin: u}
	h := serveAccountLoginStart(cfg, st, rl)
	em := `known@test.com`
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"`+em+`","website":""}`))
	req.Host = "localhost:9090"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	raw := string(gotMail)
	if !strings.Contains(raw, "/account/session?token=") {
		t.Fatalf("missing magic link body: %s", raw[:min(600, len(raw))])
	}
}

func TestServeAccountLoginStart_SMTPError(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return errors.New("smtp down")
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	st := &stubAccountWeb{userByLogin: &models.User{ID: 1, Login: "web_x"}}
	h := serveAccountLoginStart(cfg, st, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"u@test.com","website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "email_send_failed")
}

func TestServeAccountServices_InvalidToken(t *testing.T) {
	cfg := orderStartTestCfg()
	h := serveAccountServices(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodGet, "/api/account/services?token=bad.token.here", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountServices_SuccessNoSensitiveLeak(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateAccountToken(secret, "ok@test.com", 99, "web_l99", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		balance: &models.UserBalance{Balance: 0.93, Forecast: 0},
		services: []models.UserService{{
			Name:          "1 мес",
			ServiceID:     336,
			BaseServiceID: 3,
			Status:        "ACTIVE",
			Expire:        "2099",
			Period:        "1",
			Category:      "vpn-mz-test",
			KeyMarzban: models.UserKeyMarzban{
				SubscriptionURL: "https://sub-secret.example/",
				Links:           []string{"http://never"},
			},
		}},
	}
	h := serveAccountServices(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(strings.ToLower(raw), "subscription_url") || strings.Contains(raw, `"links"`) {
		t.Fatal("response leaks subscription_url or links")
	}
	var env struct {
		User struct {
			Email    string  `json:"email"`
			ID       int     `json:"user_id"`
			Balance  float64 `json:"balance"`
			Forecast float64 `json:"forecast"`
		} `json:"user"`
		Services []struct {
			UserServiceID int  `json:"user_service_id"`
			ServiceID     int  `json:"service_id"`
			CanConnect    bool `json:"can_connect"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	if env.User.Email != "ok@test.com" || env.User.ID != 99 || env.User.Balance != 0.93 || env.User.Forecast != 0 {
		t.Fatalf("user %+v", env.User)
	}
	if len(env.Services) != 1 || !env.Services[0].CanConnect || env.Services[0].UserServiceID != 336 || env.Services[0].ServiceID != 3 {
		t.Fatalf("services %+v", env.Services)
	}
}

func TestServeAccountConnect_ACTIVE_OK(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateAccountToken(secret, "me@test.com", 10, "web_aa", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:        10,
				ServiceID:     336,
				BaseServiceID: 3,
				Status:        "ACTIVE",
				Category:      "vpn-mz-x",
				KeyMarzban:    models.UserKeyMarzban{SubscriptionURL: "https://sub.example/connect"},
			},
		},
	}
	h := serveAccountServiceConnect(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var out accountConnectOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || out.ConnectURL != "https://sub.example/connect" || out.ConnectTitle != accountConnectTitle {
		t.Fatalf("%#v", out)
	}
}

func TestServeAccountConnect_UserMismatchForbidden(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:    999,
				ServiceID: 336,
				Status:    "ACTIVE",
				Category:  "vpn-mz-x",
			},
		},
	}
	h := serveAccountServiceConnect(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestServeAccountConnect_NotPaid_NotReady(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:    10,
				ServiceID: 336,
				Status:    "NOT PAID",
				Category:  "vpn-mz-x",
			},
		},
	}
	h := serveAccountServiceConnect(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out accountConnectOKJSON
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if rec.Code != http.StatusOK || out.Status != "not_ready" || out.ConnectURL != "" {
		t.Fatalf("%#v code=%d", out, rec.Code)
	}
}

func TestServeAccountServices_BalanceFailed(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 50, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{balanceErr: errors.New("shm down")}
	h := serveAccountServices(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "balance_failed")
}

func TestServeAccountBalanceTopup_InvalidToken(t *testing.T) {
	cfg := orderStartTestCfg()
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(`{"token":"","amount":150}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountBalanceTopup_InvalidAmount(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example.com"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 51, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	for _, body := range []string{
		`{"token":"` + tok + `","amount":49}`,
		`{"token":"` + tok + `","amount":10000.01}`,
		`{"token":"` + tok + `","amount":150.001}`,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(body))
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400 got %d %s for %s", rec.Code, rec.Body.String(), body)
		}
		assertJSONErrorField(t, rec.Body.String(), "invalid_amount")
	}
}

func TestServeAccountBalanceTopup_SuccessPaymentURL(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.fix.test"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 701, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	body := `{"token":"` + tok + `","amount":150}`
	req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(body))
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
	if !strings.Contains(out.PaymentURL, "yookassa.cgi") || !strings.Contains(out.PaymentURL, "701") || !strings.Contains(out.PaymentURL, "amount=150") {
		t.Fatal(out.PaymentURL)
	}
}

func TestServeAccountBalanceTopup_PaymentURLFailed_EmptyAPIBase(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = ""
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "z@z.z", 2, "web_yy", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(`{"token":"`+tok+`","amount":100}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "payment_url_failed")
}
