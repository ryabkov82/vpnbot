package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type noopPremiumApp struct{}

func (noopPremiumApp) GetUser(int64) (*models.User, error) {
	panic("noopPremiumApp.GetUser must not be called")
}

func (noopPremiumApp) GetUserService(string) (*models.UserService, error) {
	panic("noopPremiumApp.GetUserService must not be called")
}

func TestServePremiumService_NoAccessToken(t *testing.T) {
	cfg := &config.Config{PremiumLinkSigningSecret: "secret"}
	h := servePremiumService(cfg, noopPremiumApp{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/premium/service?service_id=1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "forbidden" {
		t.Fatalf("body %#v", body)
	}
}

func TestServePremiumHappLink_NoAccessToken(t *testing.T) {
	cfg := &config.Config{PremiumLinkSigningSecret: "secret"}
	h := servePremiumHappLink(cfg, noopPremiumApp{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/premium/happ-link?service_id=1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "forbidden" {
		t.Fatalf("body %#v", body)
	}
}
