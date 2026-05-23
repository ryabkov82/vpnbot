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

func TestServeAccountCatalog_InvalidTokenMissing(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example.com"
	h := serveAccountCatalogServices(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountCatalog_InvalidTokenMalformed(t *testing.T) {
	cfg := orderStartTestCfg()
	h := serveAccountCatalogServices(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token=x.y.z", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountCatalog_Success_TrialExcluded(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example.com"
	cfg.Features.Trial.Enabled = true
	cfg.Features.Trial.BaseServiceID = 77
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "u@test.com", 5, "web_ab", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{
			{ServiceID: 77, Name: "Trial?", Cost: 0, Period: 1},
			{ServiceID: 3, Name: "1 месяц", Descr: "d", Cost: 150, Period: 1, AllowToOrder: 1},
		},
	}
	h := serveAccountCatalogServices(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Services) != 1 || out.Services[0].ServiceID != 3 {
		t.Fatalf("%#v", out.Services)
	}
}

func TestServeAccountCatalog_PremiumFieldsMatchPublicOffer(t *testing.T) {
	squad := "cat-prem-squad-z"
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example.com"
	cfg.PremiumSquadName = squad
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "u@test.com", 44, "web_aa", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{{
			ServiceID:    31,
			Name:         "AntiBlock Happ",
			Descr:        "D",
			Cost:         400,
			Period:       1,
			AllowToOrder: 1,
			Config: &models.ServiceConfig{Remnawave: models.ServiceRemnawaveConfig{
				InternalSquadName: squad,
			}},
		}},
	}
	rec := httptest.NewRecorder()
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Services) != 1 || out.Services[0].Tier != publicTierPremium {
		t.Fatalf("%#v", out.Services)
	}
	if out.Services[0].ConnectApp != publicConnectHapp || len(out.Services[0].Badges) != 3 {
		t.Fatalf("%#v", out.Services[0])
	}
}

func TestServeAccountCatalog_NoInternalFieldsLeak(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "u@test.com", 12, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{{
			ServiceID:    31,
			Name:         "N",
			Descr:        "D",
			Cost:         199,
			Period:       2,
			AllowToOrder: 1,
			Config: &models.ServiceConfig{Remnawave: models.ServiceRemnawaveConfig{
				InternalSquadName: "secret-squad",
				Bot:               models.ServiceBotConfig{Title: "BT", Description: "BD"},
			}},
		}},
	}
	h := serveAccountCatalogServices(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	low := strings.ToLower(raw)
	for _, leaked := range []string{"config", "allow_to_order", "internal_squad"} {
		if strings.Contains(low, leaked) {
			t.Fatalf("leaked %s in body: %s", leaked, raw)
		}
	}
}

func TestServeAccountCatalog_GetServicesFails(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@z.z", 1, "lg", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServicesErr: errors.New("boom"),
	}
	h := serveAccountCatalogServices(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "services_unavailable")
}

func TestServeAccountServiceOrder_InvalidToken(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://pay.example/"
	h := serveAccountServiceOrder(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"bad.one","service_id":3}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountServiceOrder_InvalidService(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 8, "w", time.Hour)
	h := serveAccountServiceOrder(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":0}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_service")
}

func TestServeAccountServiceOrder_TrialBlocked(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Features.Trial.Enabled = true
	cfg.Features.Trial.BaseServiceID = 900
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 77, "w77", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			900: {ServiceID: 900, Name: "T", AllowToOrder: 1},
		},
	}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":900}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
}

func TestServeAccountServiceOrder_ServiceNotFound(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 71, "w71", time.Hour)
	st := &stubAccountWeb{}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":901}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
}

func TestServeAccountServiceOrder_GetServiceByIDNotFoundError(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.x/"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 2, "w", time.Hour)
	st := &stubAccountWeb{getSvcByErr: errors.New("service 55 not found")}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":55}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
}

func TestServeAccountServiceOrder_NotOrderable_ServiceNotFound(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 71, "w71", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			902: {ServiceID: 902, Name: "Hidden", AllowToOrder: 0, Cost: 10},
		},
	}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":902}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
}

func TestServeAccountServiceOrder_SuccessCreatesUserService(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "u@buy.com", 3381, "web_b3381", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, Name: "1 месяц", Descr: "x", Cost: 150, Period: 1, AllowToOrder: 1},
		},
		serviceOrderRet: &models.UserService{ServiceID: 338, BaseServiceID: 3, Status: "NOT PAID"},
	}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if st.serviceOrderUID != 3381 || st.serviceOrderSID != 3 {
		t.Fatalf("want order uid 3381 sid 3 got %d %d", st.serviceOrderUID, st.serviceOrderSID)
	}
	var out accountServiceOrderOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "created" || out.ServiceID != 3 || out.UserServiceID != 338 ||
		out.UserServiceStatus != "NOT PAID" || out.Amount != 150 || out.PaymentURL == "" {
		t.Fatalf("%#v", out)
	}
	if out.ExistingUnpaid != false {
		t.Fatalf("existing_unpaid=%v", out.ExistingUnpaid)
	}
	if out.RequestedServiceID != 3 || out.ReturnedServiceID != 3 || strings.TrimSpace(out.ReturnedServiceName) == "" {
		t.Fatalf("ids/name: %#v", out)
	}
	if !strings.Contains(out.Message, "Услуга ожидает оплаты") || strings.Contains(out.Message, "создана") {
		t.Fatalf("unexpected message: %q", out.Message)
	}
	if !strings.Contains(out.PaymentURL, "yookassa.cgi") ||
		!strings.Contains(out.PaymentURL, "3381") || !strings.Contains(out.PaymentURL, "amount=150") {
		t.Fatal(out.PaymentURL)
	}
}

func TestServeAccountServiceOrder_ExistingUnpaidOtherTariffReturned(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "buyer@x.com", 501, "web_buy", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			4: {ServiceID: 4, Name: "3 месяца", Descr: "x", Cost: 399, Period: 3, AllowToOrder: 1},
		},
		serviceOrderRet: &models.UserService{
			ServiceID:     771,
			BaseServiceID: 3,
			Status:        "NOT PAID",
			Name:          "1 месяц",
		},
	}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":4}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if st.serviceOrderSID != 4 {
		t.Fatalf("want ServiceOrder svc 4 got %d", st.serviceOrderSID)
	}
	var out accountServiceOrderOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.ExistingUnpaid || out.RequestedServiceID != 4 || out.ReturnedServiceID != 3 {
		t.Fatalf("%#v", out)
	}
	if strings.TrimSpace(out.ReturnedServiceName) != "1 месяц" {
		t.Fatalf("name %q", out.ReturnedServiceName)
	}
	if !strings.Contains(out.Message, "У вас уже есть услуга, ожидающая оплаты") {
		t.Fatalf("message: %q", out.Message)
	}
	if !strings.Contains(out.Message, "Новая услуга не создана") {
		t.Fatalf("message: %q", out.Message)
	}
	if strings.Contains(strings.ToUpper(out.Message), "SHM") {
		t.Fatalf("message leaked SHM: %q", out.Message)
	}
	if out.PaymentURL == "" {
		t.Fatal("missing payment_url")
	}
	if !strings.Contains(out.PaymentURL, "501") || !strings.Contains(out.PaymentURL, "399") {
		t.Fatal(out.PaymentURL)
	}
}

func TestServeAccountServiceOrder_ServiceOrderFails(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 71, "w71", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 100},
		},
		serviceOrderErr: errors.New("shm order fail"),
	}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "order_failed")
}

func TestServeAccountServiceOrder_PaymentURLFailed(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = ""
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 701, "w701", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 120},
		},
		serviceOrderRet: &models.UserService{ServiceID: 999, Status: "NOT PAID"},
	}
	h := serveAccountServiceOrder(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "payment_url_failed")
}
