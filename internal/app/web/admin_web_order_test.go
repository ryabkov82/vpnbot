package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type stubAdminWebOrderApp struct {
	svc      *models.Service
	svcErr   error
	user     *models.User
	userErr  error
	order    *models.UserService
	orderErr error
}

func (s *stubAdminWebOrderApp) GetServiceByID(serviceID int) (*models.Service, error) {
	if s.svcErr != nil {
		return nil, s.svcErr
	}
	return s.svc, nil
}

func (s *stubAdminWebOrderApp) FindOrCreateWebUser(email string) (*models.User, error) {
	if s.userErr != nil {
		return nil, s.userErr
	}
	return s.user, nil
}

func (s *stubAdminWebOrderApp) ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error) {
	if s.orderErr != nil {
		return nil, s.orderErr
	}
	return s.order, nil
}

func testAdminWebOrderCfg(token string) *config.Config {
	cfg := &config.Config{}
	cfg.Admin.Token = token
	cfg.API.BaseURL = "https://pay.example"
	return cfg
}

func TestServeAdminWebOrderTest_ForbiddenNoToken(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	h := serveAdminWebOrderTest(cfg, &stubAdminWebOrderApp{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{"email":"a@b.c","service_id":1}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "forbidden")
}

func TestServeAdminWebOrderTest_ForbiddenWrongToken(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	h := serveAdminWebOrderTest(cfg, &stubAdminWebOrderApp{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{"email":"a@b.c","service_id":1}`))
	req.Header.Set("X-Admin-Token", "wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestServeAdminWebOrderTest_MethodNotAllowed(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	h := serveAdminWebOrderTest(cfg, &stubAdminWebOrderApp{})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/web-order/test", nil)
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("Allow: %q", rec.Header().Get("Allow"))
	}
}

func TestServeAdminWebOrderTest_BadJSON(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	h := serveAdminWebOrderTest(cfg, &stubAdminWebOrderApp{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "bad_request")
}

func TestServeAdminWebOrderTest_InvalidServiceID(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	h := serveAdminWebOrderTest(cfg, &stubAdminWebOrderApp{})
	body := `{"email":"a@b.c","service_id":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(body))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_service")
}

func TestServeAdminWebOrderTest_TrialBlocked(t *testing.T) {
	const trialID = 77
	cfg := testAdminWebOrderCfg("secret")
	cfg.Features.Trial.Enabled = true
	cfg.Features.Trial.BaseServiceID = trialID
	h := serveAdminWebOrderTest(cfg, &stubAdminWebOrderApp{
		svc: &models.Service{ServiceID: trialID, Name: "Trial", Cost: 0, Period: 1},
	})
	body := `{"email":"a@b.c","service_id":77}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(body))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
}

func TestServeAdminWebOrderTest_SuccessPaymentURL(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	app := &stubAdminWebOrderApp{
		svc:  &models.Service{ServiceID: 3, Name: "VPN", Cost: 150, Period: 1},
		user: &models.User{ID: 123, Login: "web_abc"},
		order: &models.UserService{
			ServiceID: 456,
			Status:    "NOT PAID",
		},
	}
	h := serveAdminWebOrderTest(cfg, app)
	body := `{"email":"test@example.com","service_id":3}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(body))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var out adminWebOrderTestOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "created" || out.UserID != 123 || out.Login != "web_abc" || out.ServiceID != 3 {
		t.Fatalf("unexpected %#v", out)
	}
	if out.UserServiceID != 456 || out.UserServiceStatus != "NOT PAID" || out.Amount != 150 {
		t.Fatalf("unexpected %#v", out)
	}
	if !strings.Contains(out.PaymentURL, "yookassa.cgi") || !strings.Contains(out.PaymentURL, "user_id=123") {
		t.Fatalf("payment_url: %q", out.PaymentURL)
	}
}

func TestServeAdminWebOrderTest_FindOrCreateWebUserError(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	app := &stubAdminWebOrderApp{
		svc:     &models.Service{ServiceID: 1, Name: "X", Cost: 10, Period: 1},
		userErr: errors.New("boom"),
	}
	h := serveAdminWebOrderTest(cfg, app)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{"email":"a@b.c","service_id":1}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "web_user_failed")
}

func TestServeAdminWebOrderTest_ServiceOrderError(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	app := &stubAdminWebOrderApp{
		svc:      &models.Service{ServiceID: 1, Name: "X", Cost: 10, Period: 1},
		user:     &models.User{ID: 1, Login: "u"},
		orderErr: errors.New("order boom"),
	}
	h := serveAdminWebOrderTest(cfg, app)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{"email":"a@b.c","service_id":1}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "order_failed")
}

func TestServeAdminWebOrderTest_ServiceNotFound(t *testing.T) {
	cfg := testAdminWebOrderCfg("secret")
	app := &stubAdminWebOrderApp{
		svcErr: errors.New("service 9 not found"),
	}
	h := serveAdminWebOrderTest(cfg, app)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/web-order/test", strings.NewReader(`{"email":"a@b.c","service_id":9}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "service_not_found")
}

func TestAdminTokenMatches(t *testing.T) {
	if !adminTokenMatches("abc", "abc") {
		t.Fatal("same token should match")
	}
	if adminTokenMatches("abc", "abd") {
		t.Fatal("different tokens should not match")
	}
	if adminTokenMatches("", "x") || adminTokenMatches("x", "") {
		t.Fatal("empty should not match")
	}
}

func assertJSONErrorField(t *testing.T, body, want string) {
	t.Helper()
	var m map[string]string
	if err := json.NewDecoder(bytes.NewReader([]byte(body))).Decode(&m); err != nil {
		t.Fatalf("json: %v body %q", err, body)
	}
	if m["error"] != want {
		t.Fatalf("error: got %q want %q (body %q)", m["error"], want, body)
	}
}
