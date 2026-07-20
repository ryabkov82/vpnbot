package api

import (
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestExpectedServiceCategory_ExplicitBrandWinsOverLegacy(t *testing.T) {
	cfg := &config.Config{}
	cfg.Services.Category = "legacy-category"
	cfg.Brand = config.BrandConfig{
		ID:                 "vff",
		Name:               "VPN for Friends",
		AllowedHosts:       []string{"connect.vpn-for-friends.com"},
		PublicBaseURL:      "https://connect.vpn-for-friends.com",
		LandingURL:         "https://vpn-for-friends.com",
		ServiceCategory:    "brand-category",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "telegram_bot",
	}
	c := &APIClient{config: cfg}
	if got := c.expectedServiceCategory(); got != "brand-category" {
		t.Fatalf("want brand-category, got %q", got)
	}
}

func TestExpectedServiceCategory_LegacyWhenBrandEmpty(t *testing.T) {
	cfg := &config.Config{}
	cfg.Services.Category = "legacy-category"
	c := &APIClient{config: cfg}
	if got := c.expectedServiceCategory(); got != "legacy-category" {
		t.Fatalf("want legacy-category, got %q", got)
	}
}
