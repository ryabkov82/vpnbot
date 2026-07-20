package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	appService "github.com/ryabkov82/vpnbot/internal/service"
)

// Compile-time: user-facing apps expose ownership API, not raw GetUserService.
var (
	_ accountWebApp = (*stubAccountWeb)(nil)
	_ premiumAPIApp = (*stubPremiumOwnedApp)(nil)
)

func TestAccountWebAppInterface_NoRawGetUserService(t *testing.T) {
	rt := reflect.TypeOf((*accountWebApp)(nil)).Elem()
	if _, ok := rt.MethodByName("GetUserService"); ok {
		t.Fatal("accountWebApp must not expose GetUserService")
	}
	if _, ok := rt.MethodByName("GetOwnedUserServiceByUserID"); !ok {
		t.Fatal("accountWebApp must expose GetOwnedUserServiceByUserID")
	}
}

func TestPremiumAPIAppInterface_NoRawGetUserService(t *testing.T) {
	rt := reflect.TypeOf((*premiumAPIApp)(nil)).Elem()
	if _, ok := rt.MethodByName("GetUserService"); ok {
		t.Fatal("premiumAPIApp must not expose GetUserService")
	}
}

type stubPremiumOwnedApp struct {
	user     *models.User
	owned    *models.UserService
	ownedErr error
}

func (s *stubPremiumOwnedApp) GetUser(int64) (*models.User, error) { return s.user, nil }
func (s *stubPremiumOwnedApp) GetUserByID(int) (*models.User, error) {
	return s.user, nil
}
func (s *stubPremiumOwnedApp) GetOwnedUserServiceByUserID(int, string) (*models.UserService, error) {
	if s.ownedErr != nil {
		return nil, s.ownedErr
	}
	return s.owned, nil
}

func TestServePremiumService_OwnedOK(t *testing.T) {
	secret := "premium-secret-premium-secret-xx"
	cfg := &config.Config{
		PremiumLinkSigningSecret: secret,
		PremiumSquadName:         "premium-squad",
	}
	tok, err := CreatePremiumSHMAccessToken(secret, 42, 9001, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &stubPremiumOwnedApp{
		user: &models.User{ID: 42, Login: "web_x"},
		owned: &models.UserService{
			UserID: 42, ServiceID: 9001, Status: "ACTIVE", Name: "Premium",
			Expire:    "2099-01-01",
			ConfigRaw: `{"remnawave":{"internal_squad_name":"premium-squad"}}`,
		},
	}
	rec := httptest.NewRecorder()
	servePremiumService(cfg, app, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/premium/service?service_id=9001&access_token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServePremiumService_UnavailableSameForbidden(t *testing.T) {
	secret := "premium-secret-premium-secret-xx"
	cfg := &config.Config{PremiumLinkSigningSecret: secret}
	tok, err := CreatePremiumSHMAccessToken(secret, 42, 9001, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &stubPremiumOwnedApp{
		user:     &models.User{ID: 42},
		ownedErr: appService.ErrUserServiceUnavailable,
	}
	rec := httptest.NewRecorder()
	servePremiumService(cfg, app, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/premium/service?service_id=9001&access_token="+tok, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "forbidden" {
		t.Fatalf("%#v", body)
	}
	if strings.Contains(rec.Body.String(), "subscription") {
		t.Fatalf("must not leak subscription data: %s", rec.Body.String())
	}
}
