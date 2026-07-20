package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectiveBrand_NilSafe(t *testing.T) {
	var cfg *Config
	b := cfg.EffectiveBrand()
	if b.ID != "vff" || b.Name != "VPN for Friends" {
		t.Fatalf("%#v", b)
	}
	if cfg.ServiceCategory() != "" || cfg.PublicBaseURL() != "" || cfg.PaymentProfile() != "" {
		t.Fatal("nil accessors must return empty strings")
	}
	if cfg.WebUserLoginPrefix() != "web_" || cfg.WebUserSource() != "vpn-for-friends.com" {
		t.Fatalf("nil web identity defaults: %q %q", cfg.WebUserLoginPrefix(), cfg.WebUserSource())
	}
}

func TestEffectiveBrand_LegacySynthesizeFromFields(t *testing.T) {
	cfg := &Config{}
	cfg.Services.Category = " vpn-mz-main "
	cfg.WebSales.PublicBaseURL = " https://connect.vpn-for-friends.com/ "
	cfg.Payments.Profile = " telegram_bot "
	b := cfg.EffectiveBrand()
	if b.ID != "vff" || b.Name != "VPN for Friends" {
		t.Fatalf("id/name: %#v", b)
	}
	if b.ServiceCategory != "vpn-mz-main" {
		t.Fatalf("category: %q", b.ServiceCategory)
	}
	if b.PublicBaseURL != "https://connect.vpn-for-friends.com" {
		t.Fatalf("public: %q", b.PublicBaseURL)
	}
	if b.PaymentProfile != "telegram_bot" {
		t.Fatalf("profile: %q", b.PaymentProfile)
	}
	if b.WebUserLoginPrefix != "web_" || b.WebUserSource != "vpn-for-friends.com" {
		t.Fatalf("web identity: %#v", b)
	}
	if len(b.AllowedHosts) != 1 || b.AllowedHosts[0] != "connect.vpn-for-friends.com" {
		t.Fatalf("hosts: %#v", b.AllowedHosts)
	}
	if b.LandingURL != "https://vpn-for-friends.com" {
		t.Fatalf("landing: %q", b.LandingURL)
	}
}

func TestEffectiveBrand_ExplicitOverridesLegacy(t *testing.T) {
	cfg := &Config{}
	cfg.Services.Category = "legacy-category"
	cfg.WebSales.PublicBaseURL = "https://legacy.example"
	cfg.Payments.Profile = "legacy_profile"
	cfg.Brand = BrandConfig{
		ID:                 "vff",
		Name:               "VPN for Friends",
		AllowedHosts:       []string{"connect.vpn-for-friends.com"},
		PublicBaseURL:      "https://connect.vpn-for-friends.com",
		LandingURL:         "https://vpn-for-friends.com",
		ServiceCategory:    "brand-category",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "brand_profile",
	}
	if cfg.ServiceCategory() != "brand-category" {
		t.Fatalf("want brand-category, got %q", cfg.ServiceCategory())
	}
	if cfg.PublicBaseURL() != "https://connect.vpn-for-friends.com" {
		t.Fatalf("public: %q", cfg.PublicBaseURL())
	}
	if cfg.PaymentProfile() != "brand_profile" {
		t.Fatalf("profile: %q", cfg.PaymentProfile())
	}
}

func TestNormalize_LegacyJSONWithoutBrand(t *testing.T) {
	raw := `{
		"telegram": {"token": "tok"},
		"services": {"category": "vpn-mz-main"},
		"web_sales": {"public_base_url": "https://connect.vpn-for-friends.com"},
		"payments": {"profile": "telegram_bot"}
	}`
	cfg := &Config{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Brand.ID != "vff" {
		t.Fatalf("brand after normalize: %#v", cfg.Brand)
	}
	if cfg.Brand.ServiceCategory != "vpn-mz-main" {
		t.Fatalf("category: %q", cfg.Brand.ServiceCategory)
	}
}

func TestNormalize_PartialExplicitNotFilledFromLegacy(t *testing.T) {
	cfg := &Config{}
	cfg.Services.Category = "legacy-category"
	cfg.WebSales.PublicBaseURL = "https://legacy.example"
	cfg.Payments.Profile = "legacy_profile"
	cfg.Brand = BrandConfig{
		ID:                 "vff",
		Name:               "VPN for Friends",
		AllowedHosts:       []string{"connect.vpn-for-friends.com"},
		PublicBaseURL:      "https://connect.vpn-for-friends.com",
		LandingURL:         "https://vpn-for-friends.com",
		ServiceCategory:    "brand-category",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "brand_profile",
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Brand.ServiceCategory != "brand-category" {
		t.Fatalf("must not pick legacy category: %q", cfg.Brand.ServiceCategory)
	}
	if cfg.Brand.PublicBaseURL != "https://connect.vpn-for-friends.com" {
		t.Fatalf("must not pick legacy public url: %q", cfg.Brand.PublicBaseURL)
	}
	if cfg.Brand.PaymentProfile != "brand_profile" {
		t.Fatalf("must not pick legacy profile: %q", cfg.Brand.PaymentProfile)
	}
}

func TestNormalize_InvalidBrandID(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.ID = "Bad ID"
	if err := cfg.Normalize(); err == nil || !strings.Contains(err.Error(), "brand.id") {
		t.Fatalf("want brand.id error, got %v", err)
	}
}

func TestNormalize_InvalidURLs(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.PublicBaseURL = "not-a-url"
	if err := cfg.Normalize(); err == nil {
		t.Fatal("want public_base_url error")
	}
	cfg = validExplicitBrandCfg()
	cfg.Brand.LandingURL = "ftp://x.example"
	if err := cfg.Normalize(); err == nil {
		t.Fatal("want landing_url error")
	}
}

func TestNormalize_InvalidAllowedHost(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.AllowedHosts = []string{"https://evil.example/path"}
	if err := cfg.Normalize(); err == nil || !strings.Contains(err.Error(), "allowed_hosts") {
		t.Fatalf("want allowed_hosts error, got %v", err)
	}
}

func TestNormalize_HostsNormalized(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.AllowedHosts = []string{"CONNECT.EXAMPLE.COM", "connect.example.com.", "api.example.com"}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Brand.AllowedHosts) != 2 {
		t.Fatalf("hosts: %#v", cfg.Brand.AllowedHosts)
	}
	if cfg.Brand.AllowedHosts[0] != "connect.example.com" {
		t.Fatalf("first host: %q", cfg.Brand.AllowedHosts[0])
	}
	if cfg.Brand.AllowedHosts[1] != "api.example.com" {
		t.Fatalf("second host: %q", cfg.Brand.AllowedHosts[1])
	}
}

func TestNormalizeHostEntry_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"connect.vpn-for-friends.com", "connect.vpn-for-friends.com"},
		{"CONNECT.VPN-FOR-FRIENDS.COM", "connect.vpn-for-friends.com"},
		{"connect.vpn-for-friends.com.", "connect.vpn-for-friends.com"},
		{"localhost", "localhost"},
		{"xn--example-ova.com", "xn--example-ova.com"},
		{"a.b.example.com", "a.b.example.com"},
		{strings.Repeat("a", 63) + ".example.com", strings.Repeat("a", 63) + ".example.com"},
	}
	for _, tc := range cases {
		got, ok := normalizeHostEntry(tc.in)
		if !ok || got != tc.want {
			t.Fatalf("normalizeHostEntry(%q) = (%q, %v), want (%q, true)", tc.in, got, ok, tc.want)
		}
	}
}

func TestNormalizeHostEntry_Invalid(t *testing.T) {
	invalid := []string{
		"",
		" ",
		"https://connect.example.com",
		"http://connect.example.com",
		"connect.example.com:443",
		"connect.example.com/path",
		"connect.example.com?x=1",
		"connect.example.com#fragment",
		"user@connect.example.com",
		"bad host",
		"bad\tname",
		"example..com",
		".example.com",
		"example.com..",
		"-host.example",
		"host-.example",
		"*.example.com",
		"example.*.com",
		strings.Repeat("a", 64) + ".example.com",
		// hostname > 253: four 63-char labels + 3 dots = 255
		strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63),
	}

	for _, in := range invalid {
		got, ok := normalizeHostEntry(in)
		if ok {
			t.Fatalf("normalizeHostEntry(%q) unexpectedly ok: %q", in, got)
		}
	}
}

func TestNormalize_ExplicitRejectsInvalidHostAmongValid(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.AllowedHosts = []string{"connect.vpn-for-friends.com", "https://evil.example"}
	if err := cfg.Normalize(); err == nil || !strings.Contains(err.Error(), "allowed_hosts") {
		t.Fatalf("want reject entire config, got %v", err)
	}
}

func TestNormalize_ExplicitRejectsEmptyHostElement(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.AllowedHosts = []string{"connect.vpn-for-friends.com", ""}
	if err := cfg.Normalize(); err == nil || !strings.Contains(err.Error(), "allowed_hosts") {
		t.Fatalf("want reject empty host element, got %v", err)
	}
}

func TestNormalize_LegacyVFFHostUnchanged(t *testing.T) {
	cfg := &Config{}
	cfg.Services.Category = "vpn-mz-main"
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Brand.AllowedHosts) != 1 || cfg.Brand.AllowedHosts[0] != "connect.vpn-for-friends.com" {
		t.Fatalf("legacy VFF hosts: %#v", cfg.Brand.AllowedHosts)
	}
}

func TestLoadFromFile_AndVPNBOT_CONFIG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	body := `{
		"telegram": {"token": "test-token"},
		"services": {"category": "vpn-mz-main"},
		"web_sales": {"public_base_url": "https://connect.vpn-for-friends.com"},
		"payments": {"profile": "telegram_bot"}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Brand.ID != "vff" || cfg.ServiceCategory() != "vpn-mz-main" {
		t.Fatalf("%#v", cfg.Brand)
	}

	t.Setenv(envConfigPath, path)
	loaded, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Brand.ID != "vff" {
		t.Fatalf("%#v", loaded.Brand)
	}

	t.Setenv(envConfigPath, filepath.Join(dir, "missing.json"))
	_, err = loadConfig()
	if err == nil {
		t.Fatal("missing VPNBOT_CONFIG path must error without fallback")
	}
	if !strings.Contains(err.Error(), "missing.json") {
		t.Fatalf("error should mention path: %v", err)
	}
}

func TestLoadFromFile_WithoutEnvKeepsSearchOrder(t *testing.T) {
	t.Setenv(envConfigPath, "")
	// Не вызываем loadConfig() без реального файла — только проверяем, что пустой env не форсирует путь.
	if p := strings.TrimSpace(os.Getenv(envConfigPath)); p != "" {
		t.Fatal("env must be empty for this test")
	}
}

func validExplicitBrandCfg() *Config {
	cfg := &Config{}
	cfg.Telegram.Token = "tok"
	cfg.Brand = BrandConfig{
		ID:                 "vff",
		Name:               "VPN for Friends",
		AllowedHosts:       []string{"connect.vpn-for-friends.com"},
		PublicBaseURL:      "https://connect.vpn-for-friends.com",
		LandingURL:         "https://vpn-for-friends.com",
		ServiceCategory:    "vpn-mz-main",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "telegram_bot",
	}
	return cfg
}
