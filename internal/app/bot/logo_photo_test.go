package bot

import (
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestLogoPhoto_UsesExactAssetsLogoURL_VFFAndFC(t *testing.T) {
	const (
		vffLogo = "https://vpn-for-friends.com/logobot.jpg"
		fcLogo  = "https://vpn-for-friends.com/logobot_friends_connect.png"
	)

	vffCfg := &config.Config{}
	vffCfg.Assets.LogoURL = vffLogo
	vff := NewService(nil, vffCfg)
	vffPhoto := vff.logoPhoto("caption-vff")
	if vffPhoto.FileURL != vffLogo {
		t.Fatalf("VFF logo=%q, want %q", vffPhoto.FileURL, vffLogo)
	}
	if vffPhoto.Caption != "caption-vff" {
		t.Fatalf("VFF caption=%q", vffPhoto.Caption)
	}

	fcCfg := &config.Config{}
	fcCfg.Assets.LogoURL = fcLogo
	fc := NewService(nil, fcCfg)
	fcPhoto := fc.logoPhoto("caption-fc")
	if fcPhoto.FileURL != fcLogo {
		t.Fatalf("FC logo=%q, want %q", fcPhoto.FileURL, fcLogo)
	}
	if fcPhoto.FileURL == vffLogo || strings.HasSuffix(fcPhoto.FileURL, "logobot.jpg") {
		t.Fatalf("FC must not use VFF logobot.jpg, got %q", fcPhoto.FileURL)
	}
	if fcPhoto.Caption != "caption-fc" {
		t.Fatalf("FC caption=%q", fcPhoto.Caption)
	}
}
