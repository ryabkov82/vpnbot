package config

import (
	"strings"
	"testing"
)

func TestNormalize_AssetsLogoURL_ValidHTTPS(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = "https://assets.example.test/logo.png"
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Assets.LogoURL != "https://assets.example.test/logo.png" {
		t.Fatalf("logo=%q", cfg.Assets.LogoURL)
	}
}

func TestNormalize_AssetsLogoURL_ValidHTTP(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = "http://assets.example.test/logo.png"
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Assets.LogoURL != "http://assets.example.test/logo.png" {
		t.Fatalf("logo=%q", cfg.Assets.LogoURL)
	}
}

func TestNormalize_AssetsLogoURL_TrimsSpaces(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = "  https://assets.example.test/logo.png  "
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Assets.LogoURL != "https://assets.example.test/logo.png" {
		t.Fatalf("logo=%q", cfg.Assets.LogoURL)
	}
}

func TestNormalize_AssetsLogoURL_EmptyFail(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = ""
	err := cfg.Normalize()
	if err == nil || !strings.Contains(err.Error(), "assets.logo_url") {
		t.Fatalf("want assets.logo_url error, got %v", err)
	}
}

func TestNormalize_AssetsLogoURL_WhitespaceFail(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = " \t  "
	err := cfg.Normalize()
	if err == nil || !strings.Contains(err.Error(), "assets.logo_url is required") {
		t.Fatalf("want required error, got %v", err)
	}
}

func TestNormalize_AssetsLogoURL_RelativeFail(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = "/static/logo.png"
	err := cfg.Normalize()
	if err == nil || !strings.Contains(err.Error(), "assets.logo_url") {
		t.Fatalf("want assets.logo_url error, got %v", err)
	}
}

func TestNormalize_AssetsLogoURL_NoHostFail(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = "https://"
	err := cfg.Normalize()
	if err == nil || !strings.Contains(err.Error(), "assets.logo_url") {
		t.Fatalf("want assets.logo_url error, got %v", err)
	}
}

func TestNormalize_AssetsLogoURL_UnsupportedSchemeFail(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Assets.LogoURL = "ftp://assets.example.test/logo.png"
	err := cfg.Normalize()
	if err == nil || !strings.Contains(err.Error(), "assets.logo_url") {
		t.Fatalf("want assets.logo_url error, got %v", err)
	}
}
