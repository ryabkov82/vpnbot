package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func TestCfgServiceCategory_ExplicitBrandWins(t *testing.T) {
	cfg := &config.Config{}
	cfg.Services.Category = "legacy-category"
	cfg.Brand.ID = "vff"
	cfg.Brand.ServiceCategory = "brand-category"
	if got := cfgServiceCategory(cfg); got != "brand-category" {
		t.Fatalf("got %q", got)
	}
}

func TestServeAccountServiceOrder_UsesBrandCategoryNotLegacy(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.good.test"
	cfg.Services.Category = "legacy-category"
	cfg.Brand = config.BrandConfig{
		ID:                 "vff",
		Name:               "VPN for Friends",
		AllowedHosts:       []string{"connect.vpn-for-friends.com"},
		PublicBaseURL:      "https://shop.example",
		LandingURL:         "https://vpn-for-friends.com",
		ServiceCategory:    "brand-category",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "telegram_bot",
	}
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 42, "web_a", time.Hour)
	st := &stubAccountWeb{
		svcByID: map[int]*models.Service{
			3: {ServiceID: 3, AllowToOrder: 1, Cost: 100, Category: "brand-category"},
			9: {ServiceID: 9, AllowToOrder: 1, Cost: 100, Category: "legacy-category"},
		},
		serviceOrderRet: &models.UserService{ServiceID: 700, BaseServiceID: 3, Status: "NOT PAID", Name: "ok"},
		balance:         &models.UserBalance{Forecast: 0},
	}

	recOK := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(recOK, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":3}`)))
	if recOK.Code != http.StatusOK {
		t.Fatalf("brand category order: %d %s", recOK.Code, recOK.Body.String())
	}

	recBad := httptest.NewRecorder()
	serveAccountServiceOrder(cfg, st).ServeHTTP(recBad, httptest.NewRequest(http.MethodPost, "/api/account/service/order",
		strings.NewReader(`{"token":"`+tok+`","service_id":9}`)))
	if recBad.Code != http.StatusNotFound {
		t.Fatalf("legacy category must be rejected when brand set: %d", recBad.Code)
	}
}

func TestPublicOrderBaseURL_UsesBrandPublicBaseURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.WebSales.PublicBaseURL = "https://legacy.example"
	cfg.Brand = config.BrandConfig{
		ID:            "vff",
		PublicBaseURL: "https://brand.example",
	}
	got := publicOrderBaseURL(cfg, nil)
	if got != "https://brand.example" {
		t.Fatalf("got %q", got)
	}
}

func TestSignupToken_UsesEffectiveWebLoginPrefix(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.ID = "fc"
	cfg.Brand.WebUserLoginPrefix = "web_fc_"
	em := "x@y.zz"
	want := webuser.WebLoginFromEmailWithPrefix(em, cfg.WebUserLoginPrefix())
	legacy := webuser.WebLoginFromEmail(em)
	if want == legacy {
		t.Fatal("prefix must change login")
	}
	tok, err := CreateAccountSignupToken(cfg.WebSales.OrderTokenSecret, em, want, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseAndVerifyAccountSignupToken(cfg.WebSales.OrderTokenSecret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Login != want {
		t.Fatalf("signup login %q want %q", claims.Login, want)
	}
}
