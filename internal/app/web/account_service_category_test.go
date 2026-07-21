package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func categoryTestCfg(category string) *config.Config {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	cfg.Brand.ServiceCategory = category
	return cfg
}

func stubWithCategory(category string) *stubAccountWeb {
	return &stubAccountWeb{requireCategory: category}
}

// --- POST /api/account/service/order ---

func TestServeAccountServiceOrder_AllowedCategoryOrdered(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 42, "web_a", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, Name: "1 месяц", AllowToOrder: 1, Cost: 100, Category: "vpn-mz-main"},
		},
		serviceOrderRet: &models.UserService{ServiceID: 700, BaseServiceID: 3, Status: "NOT PAID", Name: "1 месяц"},
		balance:         &models.UserBalance{Forecast: 100},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	if st.serviceOrderCalls != 1 || st.serviceOrderSID != 3 || st.serviceOrderUID != 42 {
		t.Fatalf("order calls=%d sid=%d uid=%d", st.serviceOrderCalls, st.serviceOrderSID, st.serviceOrderUID)
	}
}

func TestServeAccountServiceOrder_OtherCategoryNotFound(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 42, "web_a", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			9: {ServiceID: 9, Name: "Foreign", AllowToOrder: 1, Cost: 500, Category: "vpn-mz-other"},
		},
		serviceOrderRet: &models.UserService{ServiceID: 900, BaseServiceID: 9, Status: "NOT PAID"},
		balance:         &models.UserBalance{Forecast: 500},
	}
	rec := httptest.NewRecorder()
	// EN-локаль: именно на этом пути при needsTopUp строится платёжная ссылка (CryptoCloud).
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order?lang=en",
		strings.NewReader(`{"token":"`+tok+`","service_id":9}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d: %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
	if st.serviceOrderCalls != 0 {
		t.Fatalf("ServiceOrderByUserID must not be called, calls=%d", st.serviceOrderCalls)
	}
	if st.balanceCalls != 0 {
		t.Fatalf("GetUserBalanceByUserID must not be called, calls=%d", st.balanceCalls)
	}
	if strings.Contains(rec.Body.String(), "payment_url") || strings.Contains(rec.Body.String(), "cryptocloud") {
		t.Fatalf("payment url must not be built: %s", rec.Body.String())
	}
}

func TestServeAccountServiceOrder_EmptyCategoryLegacyAllows(t *testing.T) {
	cfg := categoryTestCfg("")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 42, "web_a", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			9: {ServiceID: 9, AllowToOrder: 1, Cost: 100, Category: "vpn-mz-anything"},
		},
		serviceOrderRet: &models.UserService{ServiceID: 901, BaseServiceID: 9, Status: "NOT PAID"},
		balance:         &models.UserBalance{Forecast: 0},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":9}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy empty category must allow, got %d: %s", rec.Code, rec.Body.String())
	}
	if st.serviceOrderCalls != 1 {
		t.Fatalf("order calls=%d", st.serviceOrderCalls)
	}
}

// --- GET /api/account/service/connect ---

func TestServeAccountConnect_AllowedCategoryReturnsURL(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:     10,
				ServiceID:  336,
				Status:     "ACTIVE",
				Category:   "vpn-mz-main",
				KeyMarzban: models.UserKeyMarzban{SubscriptionURL: "https://sub.example/connect"},
			},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceConnect(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	var out accountConnectOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || out.ConnectURL != "https://sub.example/connect" {
		t.Fatalf("%#v", out)
	}
}

func TestServeAccountConnect_OtherCategoryForbiddenNoURL(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		requireCategory: "vpn-mz-main",
		single: map[int]*models.UserService{
			336: {
				UserID:     10, // услуга принадлежит пользователю — блокирует именно категория
				ServiceID:  336,
				Status:     "ACTIVE",
				Category:   "vpn-mz-other",
				KeyMarzban: models.UserKeyMarzban{SubscriptionURL: "https://sub.example/foreign"},
			},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceConnect(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d: %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "forbidden")
	if strings.Contains(rec.Body.String(), "sub.example") {
		t.Fatalf("connect url leaked: %s", rec.Body.String())
	}
}

func TestServeAccountConnect_OtherCategoryPremiumURLNotBuilt(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	cfg.PremiumSquadName = "premium-squad"
	cfg.PremiumConnectBaseURL = "https://premium.example/connect"
	cfg.PremiumLinkSigningSecret = "premium-secret-premium-secret-xx"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		requireCategory: "vpn-mz-main",
		single: map[int]*models.UserService{
			337: {
				UserID:    10,
				ServiceID: 337,
				Status:    "ACTIVE",
				Category:  "vpn-mz-other",
				ConfigRaw: `{"remnawave":{"internal_squad_name":"premium-squad"}}`,
			},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceConnect(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=337", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "premium.example") || strings.Contains(rec.Body.String(), "access_token") {
		t.Fatalf("premium connect url must not be built: %s", rec.Body.String())
	}
}

func TestServeAccountConnect_OwnershipStillEnforcedWithCategory(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:    999, // чужая услуга, категория разрешённая
				ServiceID: 336,
				Status:    "ACTIVE",
				Category:  "vpn-mz-main",
			},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceConnect(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ownership check broken: want 403 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "forbidden")
}

func TestServeAccountConnect_MissingServiceSameForbidden(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{} // single == nil → ErrUserServiceUnavailable
	rec := httptest.NewRecorder()
	serveAccountServiceConnect(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=404", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d: %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "forbidden")
}

// --- POST /api/account/service/delete ---

func TestServeAccountServiceDelete_AllowedCategoryDeleted(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			500: {UserID: 10, ServiceID: 500, Status: "NOT PAID", Category: "vpn-mz-main"},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceDelete(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":500}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	if st.deleteCalls != 1 || st.deleteLastUID != 10 || st.deleteLastUserServiceID != "500" {
		t.Fatalf("delete calls=%d uid=%d usid=%s", st.deleteCalls, st.deleteLastUID, st.deleteLastUserServiceID)
	}
}

func TestServeAccountServiceDelete_OtherCategoryForbiddenNoDelete(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		requireCategory: "vpn-mz-main",
		single: map[int]*models.UserService{
			500: {UserID: 10, ServiceID: 500, Status: "NOT PAID", Category: "vpn-mz-other"},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceDelete(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":500}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d: %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "forbidden")
	if st.deleteCalls != 0 {
		t.Fatalf("DeleteUserServiceByUserID must not be called, calls=%d", st.deleteCalls)
	}
}

func TestServeAccountServiceDelete_OwnershipStillEnforcedWithCategory(t *testing.T) {
	cfg := categoryTestCfg("vpn-mz-main")
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			500: {UserID: 999, ServiceID: 500, Status: "NOT PAID", Category: "vpn-mz-main"},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServiceDelete(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":500}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ownership check broken: want 403 got %d", rec.Code)
	}
	if st.deleteCalls != 0 {
		t.Fatalf("delete must not be called, calls=%d", st.deleteCalls)
	}
}

// --- POST /api/admin/web-order/test ---

func TestServeAdminWebOrderTest_AllowedCategoryProcessed(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	cfg.Brand.ServiceCategory = "vpn-mz-main"
	app := &stubAdminWebOrderApp{
		svc:   &models.Service{ServiceID: 3, Name: "1 месяц", AllowToOrder: 1, Cost: 100, Category: "vpn-mz-main"},
		user:  &models.User{ID: 77, Login: "web_x"},
		order: &models.UserService{ServiceID: 800, BaseServiceID: 3, Status: "NOT PAID"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{"email":"a@b.c","service_id":3}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	serveAdminWebOrderTest(cfg, app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	if app.userCalls != 1 || app.orderCalls != 1 {
		t.Fatalf("user calls=%d order calls=%d", app.userCalls, app.orderCalls)
	}
	if !strings.Contains(rec.Body.String(), "payment_url") {
		t.Fatalf("payment url expected in admin OK response: %s", rec.Body.String())
	}
}

func TestServeAdminWebOrderTest_OtherCategoryNotFoundNoSideEffects(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	cfg.Brand.ServiceCategory = "vpn-mz-main"
	app := &stubAdminWebOrderApp{
		svc:   &models.Service{ServiceID: 9, Name: "Foreign", AllowToOrder: 1, Cost: 500, Category: "vpn-mz-other"},
		user:  &models.User{ID: 77, Login: "web_x"},
		order: &models.UserService{ServiceID: 900, BaseServiceID: 9, Status: "NOT PAID"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{"email":"a@b.c","service_id":9}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	serveAdminWebOrderTest(cfg, app).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d: %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
	if app.userCalls != 0 {
		t.Fatalf("FindOrCreateWebUser must not be called, calls=%d", app.userCalls)
	}
	if app.orderCalls != 0 {
		t.Fatalf("ServiceOrderByUserID must not be called, calls=%d", app.orderCalls)
	}
	if strings.Contains(rec.Body.String(), "payment_url") {
		t.Fatalf("payment url must not be built: %s", rec.Body.String())
	}
}
