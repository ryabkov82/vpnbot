package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type stubPublicLeadApp struct {
	services       []models.Service
	getServicesErr error
	byID           *models.Service
	byIDErr        error
	callsGet       int
	callsByID      int
}

func (s *stubPublicLeadApp) GetServices() ([]models.Service, error) {
	s.callsGet++
	if s.getServicesErr != nil {
		return nil, s.getServicesErr
	}
	return s.services, nil
}

func (s *stubPublicLeadApp) GetServiceByID(serviceID int) (*models.Service, error) {
	s.callsByID++
	if s.byIDErr != nil {
		return nil, s.byIDErr
	}
	return s.byID, nil
}

func TestServePublicLead_POST_OK(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{
		services: []models.Service{
			{ServiceID: 10, Name: "VPN 1 мес.", Cost: 199, Period: 1},
		},
	}
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := servePublicLeadWithLimiter(cfg, app, rl)
	body := `{"service_id":10,"email":"user@example.com","contact":"@tg","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	var out publicLeadAcceptedJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "accepted" {
		t.Fatalf("body %#v", out)
	}
	if app.callsByID != 0 {
		t.Fatalf("expected GetServiceByID not called when in list, got %d", app.callsByID)
	}
}

func TestServePublicLead_InvalidEmail(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{
		services: []models.Service{{ServiceID: 1, Name: "X", Cost: 1, Period: 1}},
	}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	body := `{"service_id":1,"email":"not-an-email","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	assertJSONError(t, rec.Body.String(), "invalid_email")
}

func TestServePublicLead_InvalidServiceID(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	body := `{"service_id":0,"email":"a@b.co","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	assertJSONError(t, rec.Body.String(), "invalid_service")
}

func TestServePublicLead_TrialServiceBlocked(t *testing.T) {
	const trialID = 42
	cfg := &config.Config{
		Features: config.Features{
			Trial: config.TrialFeature{Enabled: true, BaseServiceID: trialID},
		},
	}
	app := &stubPublicLeadApp{
		services: []models.Service{{ServiceID: trialID, Name: "Trial", Cost: 0, Period: 1}},
	}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	body := `{"service_id":42,"email":"user@example.com","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	assertJSONError(t, rec.Body.String(), "service_not_found")
	if app.callsGet != 0 {
		t.Fatalf("trial must be rejected before SHM calls, got GetServices=%d", app.callsGet)
	}
}

func TestServePublicLead_HoneypotNoBusinessLogic(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPanicLeadApp{}
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := servePublicLeadWithLimiter(cfg, app, rl)
	body := `{"service_id":1,"email":"x@y.z","website":"http://spam.example"}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var out publicLeadAcceptedJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "accepted" {
		t.Fatalf("got %#v", out)
	}
}

type stubPanicLeadApp struct{}

func (stubPanicLeadApp) GetServices() ([]models.Service, error) {
	panic("GetServices must not be called for honeypot")
}

func (stubPanicLeadApp) GetServiceByID(int) (*models.Service, error) {
	panic("GetServiceByID must not be called for honeypot")
}

func TestServePublicLead_RateLimitByIP(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{
		services: []models.Service{{ServiceID: 7, Name: "P", Cost: 1, Period: 1}},
	}
	rl := newLeadRateLimiter(5, time.Hour, 50, time.Hour)
	h := servePublicLeadWithLimiter(cfg, app, rl)
	for i := 0; i < 5; i++ {
		body := `{"service_id":7,"email":"u` + string(rune('0'+i)) + `@ex.com","website":""}`
		req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
		req.RemoteAddr = "192.0.2.1:5555"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d %s", i, rec.Code, rec.Body.String())
		}
	}
	body := `{"service_id":7,"email":"last@ex.com","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	req.RemoteAddr = "192.0.2.1:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec.Body.String(), "rate_limited")
}

func TestServePublicLead_RateLimitByEmail(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{
		services: []models.Service{{ServiceID: 7, Name: "P", Cost: 1, Period: 1}},
	}
	rl := newLeadRateLimiter(50, time.Hour, 3, time.Hour)
	h := servePublicLeadWithLimiter(cfg, app, rl)
	email := "same@example.com"
	for i := 0; i < 3; i++ {
		body := `{"service_id":7,"email":"` + email + `","website":""}`
		req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
		req.RemoteAddr = "192.0.2." + string(rune('1'+i)) + ":1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d %s", i, rec.Code, rec.Body.String())
		}
	}
	body := strings.NewReader(`{"service_id":7,"email":"` + email + `","website":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", body)
	req.RemoteAddr = "192.0.2.9:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec.Body.String(), "rate_limited")
}

func TestServePublicLead_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/api/public/lead", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("Allow: %q", rec.Header().Get("Allow"))
	}
	assertJSONError(t, rec.Body.String(), "method_not_allowed")
}

func TestServePublicLead_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(`{`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	assertJSONError(t, rec.Body.String(), "bad_request")
}

func TestServePublicLead_XForwardedForIP(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{
		services: []models.Service{{ServiceID: 7, Name: "P", Cost: 1, Period: 1}},
	}
	rl := newLeadRateLimiter(2, time.Hour, 50, time.Hour)
	h := servePublicLeadWithLimiter(cfg, app, rl)
	do := func(email string) *httptest.ResponseRecorder {
		body := `{"service_id":7,"email":"` + email + `","website":""}`
		req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
		req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
		req.RemoteAddr = "127.0.0.1:9999"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if do("a@x.com").Code != http.StatusOK || do("b@x.com").Code != http.StatusOK {
		t.Fatal("expected first two OK")
	}
	rec := do("c@x.com")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 from shared XFF IP, got %d", rec.Code)
	}
}

func TestServePublicLead_ResolveViaGetServiceByID(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{
		services: []models.Service{},
		byID:     &models.Service{ServiceID: 99, Name: "OnlyByID", Cost: 10, Period: 1},
	}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	body := `{"service_id":99,"email":"u@ex.com","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	if app.callsByID != 1 {
		t.Fatalf("GetServiceByID calls: %d", app.callsByID)
	}
}

func TestServePublicLead_GetServicesError(t *testing.T) {
	cfg := &config.Config{}
	app := &stubPublicLeadApp{getServicesErr: errors.New("upstream")}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	body := `{"service_id":1,"email":"u@ex.com","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	assertJSONError(t, rec.Body.String(), "services_unavailable")
}

func TestServePublicLead_TelegramAPIErrorStillAccepted(t *testing.T) {
	old := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"description":"fake error"}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Telegram.Token = "dummy-token-for-tests"
	cfg.Telegram.LeadsChatID = 42
	app := &stubPublicLeadApp{
		services: []models.Service{{ServiceID: 10, Name: "VPN", Cost: 1, Period: 1}},
	}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	body := `{"service_id":10,"email":"ok@example.com","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestServePublicLead_TelegramHTTPTransportErrorStillAccepted(t *testing.T) {
	old := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Telegram.Token = "dummy-token-for-tests"
	cfg.Telegram.SupportChatID = 7
	app := &stubPublicLeadApp{
		services: []models.Service{{ServiceID: 10, Name: "VPN", Cost: 1, Period: 1}},
	}
	h := servePublicLeadWithLimiter(cfg, app, newLeadRateLimiter(50, time.Hour, 50, time.Hour))
	body := `{"service_id":10,"email":"ok2@example.com","website":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/lead", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
}

func assertJSONError(t *testing.T, body, want string) {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("json: %v body %q", err, body)
	}
	if m["error"] != want {
		t.Fatalf("error field: got %q, want %q (body %q)", m["error"], want, body)
	}
}
