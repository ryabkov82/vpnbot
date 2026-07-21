package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

type stubAdminAccountApp struct {
	user        *models.User
	userErr     error
	services    []models.UserService
	servicesErr error
	lastLogin   string
	lastUserID  int
}

func (s *stubAdminAccountApp) GetUserByLogin(login string) (*models.User, error) {
	s.lastLogin = login
	if s.userErr != nil {
		return nil, s.userErr
	}
	return s.user, nil
}

func (s *stubAdminAccountApp) GetUserServicesByUserID(userID int) ([]models.UserService, error) {
	s.lastUserID = userID
	if s.servicesErr != nil {
		return nil, s.servicesErr
	}
	return s.services, nil
}

func testAdminAccountCfg(token string) *config.Config {
	cfg := &config.Config{}
	cfg.Admin.Token = token
	cfg.Brand.WebUserLoginPrefix = "web_"
	cfg.Brand.WebUserSource = "vpn-for-friends.com"
	return cfg
}

func TestServeAdminAccountTest_ForbiddenNoToken(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	h := serveAdminAccountTest(cfg, &stubAdminAccountApp{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/account/test", strings.NewReader(`{"email":"a@b.c"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "forbidden")
}

func TestServeAdminAccountTest_ForbiddenWrongToken(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	h := serveAdminAccountTest(cfg, &stubAdminAccountApp{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/account/test", strings.NewReader(`{"email":"a@b.c"}`))
	req.Header.Set("X-Admin-Token", "wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestServeAdminAccountTest_MethodNotAllowed(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	h := serveAdminAccountTest(cfg, &stubAdminAccountApp{})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/account/test", nil)
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
	if g := rec.Result().Header.Get("Allow"); g != http.MethodPost {
		t.Fatalf("Allow: %q", g)
	}
}

func TestServeAdminAccountTest_BadJSON(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	h := serveAdminAccountTest(cfg, &stubAdminAccountApp{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/account/test", strings.NewReader(`{`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "bad_request")
}

func TestServeAdminAccountTest_InvalidEmail(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	h := serveAdminAccountTest(cfg, &stubAdminAccountApp{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/account/test", strings.NewReader(`{"email":"not-an-email"}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_email")
}

func TestServeAdminAccountTest_UserNotFound(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	app := &stubAdminAccountApp{user: nil}
	h := serveAdminAccountTest(cfg, app)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/account/test", strings.NewReader(`{"email":"web-test@example.com"}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "user_not_found")
	wantLogin := webuser.WebLoginFromEmail("web-test@example.com")
	if app.lastLogin != wantLogin {
		t.Fatalf("login %q want %q", app.lastLogin, wantLogin)
	}
}

func TestServeAdminAccountTest_ServicesFailed(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	app := &stubAdminAccountApp{
		user:        &models.User{ID: 510, Login: "web_x", Balance: 0},
		servicesErr: errors.New("api down"),
	}
	h := serveAdminAccountTest(cfg, app)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/account/test", strings.NewReader(`{"email":"web-test@example.com"}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "services_failed")
	if app.lastUserID != 510 {
		t.Fatalf("user id %d", app.lastUserID)
	}
}

func TestServeAdminAccountTest_Success(t *testing.T) {
	cfg := testAdminAccountCfg("secret")
	app := &stubAdminAccountApp{
		user: &models.User{
			ID:      510,
			Login:   "web_abc",
			Balance: 12.5,
			Settings: models.UserSettings{
				Telegram: models.TelegramInfo{ChatID: 999},
				Web:      models.WebInfo{Email: "should-not-appear@x.com"},
			},
		},
		services: []models.UserService{
			{
				Name:          "1 месяц",
				ServiceID:     334,
				BaseServiceID: 3,
				Status:        "ACTIVE",
				Expire:        "2099-01-01",
				Period:        "30",
				Category:      "vpn-mz-test",
				KeyMarzban: models.UserKeyMarzban{
					SubscriptionURL: "https://secret-sub.example/",
					Links:           []string{"https://raw.example/"},
				},
			},
		},
	}
	h := serveAdminAccountTest(cfg, app)
	body := `{"email":" Web-Test@Example.com "}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/account/test", strings.NewReader(body))
	req.Header.Set("X-Admin-Token", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "subscription_url") || strings.Contains(raw, `"links"`) || strings.Contains(raw, `"settings"`) {
		t.Fatalf("response leaked sensitive fields: %s", raw)
	}

	var out adminAccountTestOKJSON
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.User.UserID != 510 || out.User.Login != "web_abc" || out.User.Email != "web-test@example.com" || out.User.Balance != 12.5 {
		t.Fatalf("user %+v", out.User)
	}
	if len(out.Services) != 1 {
		t.Fatalf("services len %d", len(out.Services))
	}
	svc := out.Services[0]
	if svc.UserServiceID != 334 || svc.ServiceID != 3 || svc.Name != "1 месяц" || svc.Status != "ACTIVE" || svc.Expire != "2099-01-01" || svc.Period != "30" || svc.Category != "vpn-mz-test" {
		t.Fatalf("service %+v", svc)
	}
}
