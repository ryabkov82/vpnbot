package web

import (
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestBuildPremiumConnectURLForWeb_OK(t *testing.T) {
	cfg := &config.Config{
		PremiumConnectBaseURL:    "https://connect.example/vip/premium-connect",
		PremiumLinkSigningSecret: "k",
	}
	u, err := BuildPremiumConnectURLForWebAccount(cfg, 501, 9002)
	if err != nil || u == "" {
		t.Fatalf("err=%v url=%s", err, u)
	}
	if !strings.Contains(u, "access_token=") || !strings.Contains(u, "service_id=9002") {
		t.Fatal(u)
	}
}

func TestBuildPremiumConnectURL_NotConfigured_NoURL(t *testing.T) {
	cfg := &config.Config{PremiumConnectBaseURL: "https://x", PremiumLinkSigningSecret: ""}
	_, err := BuildPremiumConnectURLForWebAccount(cfg, 1, 2)
	if err == nil {
		t.Fatal("expected error")
	}
}
