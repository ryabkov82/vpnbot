package web

import (
	"net/http/httptest"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestPublicOrderBaseURL_TrimsBrandPublicBaseURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.Brand.PublicBaseURL = "  https://x.example/  "
	if got := publicOrderBaseURL(cfg, nil); got != "https://x.example" {
		t.Fatalf("got %q", got)
	}
}

func TestPublicOrderBaseURL_FallbackRequestHost(t *testing.T) {
	cfg := &config.Config{}
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "shop.example:443"
	if got := publicOrderBaseURL(cfg, req); got != "http://shop.example:443" {
		t.Fatalf("got %q", got)
	}
}

func TestPublicOrderBaseURL_IgnoresPremiumConnect(t *testing.T) {
	cfg := &config.Config{}
	cfg.PremiumConnectBaseURL = "https://connect.example/premium-connect"
	cfg.Brand.PublicBaseURL = "https://connect.example"
	if got := publicOrderBaseURL(cfg, nil); got != "https://connect.example" {
		t.Fatalf("got %q", got)
	}
}
