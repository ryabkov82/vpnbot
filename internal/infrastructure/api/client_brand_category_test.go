package api

import (
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestExpectedServiceCategory_ExplicitBrandUsed(t *testing.T) {
	cfg := &config.Config{}
	cfg.Services.Category = "legacy-category" // legacy fields must never be read
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

func TestExpectedServiceCategory_EmptyWhenBrandEmpty(t *testing.T) {
	// Legacy Services.Category больше не подмешивается: пустой brand → нет ограничения.
	cfg := &config.Config{}
	cfg.Services.Category = "legacy-category"
	c := &APIClient{config: cfg}
	if got := c.expectedServiceCategory(); got != "" {
		t.Fatalf("legacy category must not be synthesized, got %q", got)
	}
}
