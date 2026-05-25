package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestAccountOrderPaymentFromSHMForecast(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fc          float64
		wantAmt     float64
		wantTopUp   bool
		wantInvalid bool
	}{
		{0, 0, false, false},
		{-12, 0, false, false},
		{155.94, 155.94, true, false},
		{30, 50, true, false},
		{10000.00, 10000, true, false},
		{10000.01, 0, false, true},
	}
	for _, tc := range cases {
		amt, need, inv := accountOrderPaymentFromSHMForecast(tc.fc)
		if tc.wantInvalid != inv ||
			tc.wantAmt != amt ||
			tc.wantTopUp != need {
			t.Fatalf("fc=%v want amt=%v needTopUp=%v invalid=%v got amt=%v needTopUp=%v invalid=%v",
				tc.fc, tc.wantAmt, tc.wantTopUp, tc.wantInvalid, amt, need, inv)
		}
	}
}

func TestServeAccountServiceOrder_ForecastZero_NoPaymentURL(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "u@paid.com", 881, "w881", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, Name: "1 мес", Cost: 200, Period: 1, AllowToOrder: 1},
		},
		serviceOrderRet: &models.UserService{ServiceID: 900, BaseServiceID: 3, Status: "NOT PAID"},
		balance:         &models.UserBalance{Balance: 500, Forecast: 0},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountServiceOrderOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Amount != 0 || strings.TrimSpace(out.PaymentURL) != "" {
		t.Fatalf("%#v", out)
	}
	if out.Message != accountServiceOrderCreatedNoPaymentMessage {
		t.Fatalf("message %q want %q", out.Message, accountServiceOrderCreatedNoPaymentMessage)
	}
}

func TestServeAccountServiceOrder_ForecastBelowMinCharges50(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 771, "w771", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 399},
		},
		serviceOrderRet: &models.UserService{ServiceID: 12, BaseServiceID: 3, Status: "NOT PAID"},
		balance:         &models.UserBalance{Forecast: 30},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountServiceOrderOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Amount != 50 || strings.TrimSpace(out.PaymentURL) != "" {
		t.Fatalf("%#v", out)
	}
}

func TestServeAccountServiceOrder_ForecastExceedsTopupMax(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "x@y.z", 600, "w600", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 20000},
		},
		serviceOrderRet: &models.UserService{ServiceID: 1, BaseServiceID: 3, Status: "NOT PAID"},
		balance:         &models.UserBalance{Forecast: 10000.01},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_payment_amount")
}

func TestServeAccountServiceOrder_GetBalanceByUserFails(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 9, "w9", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 100},
		},
		serviceOrderRet: &models.UserService{ServiceID: 1, Status: "NOT PAID"},
		balanceErr:      errors.New("shm boom"),
	}
	rec := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "balance_failed")
}
