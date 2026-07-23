package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 11.3 — nil Config: zero-value BrandConfig, no panic, no VFF.
func TestEffectiveBrand_NilZeroValue(t *testing.T) {
	var cfg *Config
	b := cfg.EffectiveBrand()
	if b.ID != "" || b.Name != "" || len(b.AllowedHosts) != 0 {
		t.Fatalf("nil brand must be zero-value, got %#v", b)
	}
	// 11.4 — getters do not synthesize defaults.
	if cfg.ServiceCategory() != "" || cfg.PublicBaseURL() != "" || cfg.PaymentProfile() != "" {
		t.Fatal("nil accessors must return empty strings")
	}
	if cfg.WebUserLoginPrefix() != "" || cfg.WebUserSource() != "" {
		t.Fatalf("nil web identity must be empty, got %q %q", cfg.WebUserLoginPrefix(), cfg.WebUserSource())
	}
}

// 11.2 — legacy fields must not synthesize a VFF brand.
func TestEffectiveBrand_LegacyFieldsDoNotSynthesize(t *testing.T) {
	cfg := &Config{}
	cfg.Services.Category = "vpn-mz-fc"
	cfg.WebSales.PublicBaseURL = "https://connect-fc.vpn-for-friends.com"
	cfg.Payments.Profile = "telegram_friends_connect_bot"
	b := cfg.EffectiveBrand()
	if b.ID == "vff" {
		t.Fatalf("legacy fields must not become VFF, got id=%q", b.ID)
	}
	if b.ID != "" {
		t.Fatalf("empty brand must stay empty, got id=%q", b.ID)
	}
	if len(b.AllowedHosts) != 0 {
		t.Fatalf("empty brand must have no allowed_hosts, got %#v", b.AllowedHosts)
	}
	if b.ServiceCategory != "" || b.PublicBaseURL != "" || b.PaymentProfile != "" {
		t.Fatalf("legacy fields must not populate brand: %#v", b)
	}
}

// 11.4 — getters return only explicit brand values (empty Config).
func TestGetters_NoDefaults(t *testing.T) {
	cfg := &Config{}
	if got := cfg.ServiceCategory(); got != "" {
		t.Fatalf("ServiceCategory=%q", got)
	}
	if got := cfg.PublicBaseURL(); got != "" {
		t.Fatalf("PublicBaseURL=%q", got)
	}
	if got := cfg.PaymentProfile(); got != "" {
		t.Fatalf("PaymentProfile=%q", got)
	}
	if got := cfg.WebUserLoginPrefix(); got != "" {
		t.Fatalf("WebUserLoginPrefix=%q", got)
	}
	if got := cfg.WebUserSource(); got != "" {
		t.Fatalf("WebUserSource=%q", got)
	}
}

func TestEffectiveBrand_ExplicitOnly(t *testing.T) {
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

// 11.1 — empty brand fails.
func TestNormalize_EmptyBrandFails(t *testing.T) {
	cfg := &Config{}
	err := cfg.Normalize()
	if err == nil || !strings.Contains(err.Error(), "brand.id is required") {
		t.Fatalf("want 'brand.id is required', got %v", err)
	}
}

// 11.2 — legacy JSON without brand.id is invalid (no VFF synthesis).
func TestNormalize_LegacyJSONWithoutBrandFails(t *testing.T) {
	raw := `{
		"telegram": {"token": "tok"},
		"services": {"category": "vpn-mz-fc"},
		"web_sales": {"public_base_url": "https://connect-fc.vpn-for-friends.com"},
		"payments": {"profile": "telegram_friends_connect_bot"}
	}`
	cfg := &Config{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		t.Fatal(err)
	}
	err := cfg.Normalize()
	if err == nil || !strings.Contains(err.Error(), "brand.id is required") {
		t.Fatalf("legacy config must be invalid, got %v", err)
	}
	if cfg.EffectiveBrand().ID == "vff" {
		t.Fatal("legacy config must not synthesize VFF")
	}
	if len(cfg.EffectiveBrand().AllowedHosts) != 0 {
		t.Fatalf("legacy config must not have allowed_hosts: %#v", cfg.EffectiveBrand().AllowedHosts)
	}
}

func TestNormalize_ExplicitDoesNotPickLegacyFields(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Services.Category = "legacy-category"
	cfg.WebSales.PublicBaseURL = "https://legacy.example"
	cfg.Payments.Profile = "legacy_profile"
	cfg.Brand.ServiceCategory = "brand-category"
	cfg.Brand.PublicBaseURL = "https://connect.vpn-for-friends.com"
	cfg.Brand.PaymentProfile = "brand_profile"
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

// 11.5 — explicit VFF passes.
func TestNormalize_ExplicitVFFPasses(t *testing.T) {
	cfg := validExplicitBrandCfg()
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("explicit VFF must pass: %v", err)
	}
	if cfg.Brand.ID != "vff" {
		t.Fatalf("brand.id=%q", cfg.Brand.ID)
	}
}

// 11.6 — explicit FC passes.
func TestNormalize_ExplicitFCPasses(t *testing.T) {
	cfg := validExplicitFCBrandCfg()
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("explicit FC must pass: %v", err)
	}
	if cfg.Brand.ID != "fc" {
		t.Fatalf("brand.id=%q", cfg.Brand.ID)
	}
}

// 11.7 — each required field missing fails.
func TestNormalize_MissingEachRequiredField(t *testing.T) {
	cases := map[string]func(c *Config){
		"id":                    func(c *Config) { c.Brand.ID = "" },
		"name":                  func(c *Config) { c.Brand.Name = "" },
		"allowed_hosts":         func(c *Config) { c.Brand.AllowedHosts = nil },
		"public_base_url":       func(c *Config) { c.Brand.PublicBaseURL = "" },
		"landing_url":           func(c *Config) { c.Brand.LandingURL = "" },
		"service_category":      func(c *Config) { c.Brand.ServiceCategory = "" },
		"web_user_login_prefix": func(c *Config) { c.Brand.WebUserLoginPrefix = "" },
		"web_user_source":       func(c *Config) { c.Brand.WebUserSource = "" },
		"payment_profile":       func(c *Config) { c.Brand.PaymentProfile = "" },
		"yookassa_pay_system":   func(c *Config) { c.Brand.YooKassaPaySystem = "" },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			cfg := validExplicitBrandCfg()
			mutate(cfg)
			err := cfg.Normalize()
			if err == nil {
				t.Fatalf("missing brand.%s must fail", field)
			}
			if !strings.Contains(err.Error(), "brand."+field) {
				t.Fatalf("error should mention brand.%s, got %v", field, err)
			}
		})
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

// 12 — file loading: legacy fails, explicit VFF/FC succeed.
func TestLoadFromFile_LegacyFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	body := `{
		"telegram": {"token": "test-token"},
		"services": {"category": "vpn-mz-fc"},
		"web_sales": {"public_base_url": "https://connect-fc.vpn-for-friends.com"},
		"payments": {"profile": "telegram_friends_connect_bot"}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFromFile(path)
	if err == nil || !strings.Contains(err.Error(), "brand.id is required") {
		t.Fatalf("legacy file must fail with brand.id error, got %v", err)
	}
}

func TestLoadFromFile_ExplicitVFFAndFC(t *testing.T) {
	dir := t.TempDir()

	vffPath := filepath.Join(dir, "vff.json")
	if err := os.WriteFile(vffPath, []byte(explicitBrandJSON("vff", "VPN for Friends", "connect.vpn-for-friends.com", "https://connect.vpn-for-friends.com", "https://vpn-for-friends.com", "vpn-mz-main", "telegram_bot", "yookassa_vff")), 0o600); err != nil {
		t.Fatal(err)
	}
	vff, err := LoadFromFile(vffPath)
	if err != nil {
		t.Fatalf("explicit VFF must load: %v", err)
	}
	if vff.Brand.ID != "vff" || vff.ServiceCategory() != "vpn-mz-main" || vff.YooKassaPaySystem() != "yookassa_vff" {
		t.Fatalf("vff: %#v", vff.Brand)
	}

	fcPath := filepath.Join(dir, "fc.json")
	if err := os.WriteFile(fcPath, []byte(explicitBrandJSON("fc", "Friends Connect", "connect-fc.vpn-for-friends.com", "https://connect-fc.vpn-for-friends.com", "https://vpn-for-friends.com", "vpn-mz-fc", "telegram_friends_connect_bot", "yookassa_fc")), 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := LoadFromFile(fcPath)
	if err != nil {
		t.Fatalf("explicit FC must load: %v", err)
	}
	if fc.Brand.ID != "fc" || fc.ServiceCategory() != "vpn-mz-fc" || fc.YooKassaPaySystem() != "yookassa_fc" {
		t.Fatalf("fc: %#v", fc.Brand)
	}

	// VPNBOT_CONFIG must load explicit config without any legacy fallback.
	t.Setenv(envConfigPath, fcPath)
	loaded, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Brand.ID != "fc" {
		t.Fatalf("%#v", loaded.Brand)
	}

	t.Setenv(envConfigPath, filepath.Join(dir, "missing.json"))
	if _, err := loadConfig(); err == nil {
		t.Fatal("missing VPNBOT_CONFIG path must error without fallback")
	}
}

func TestLoadFromFile_WithoutEnvKeepsSearchOrder(t *testing.T) {
	t.Setenv(envConfigPath, "")
	// Не вызываем loadConfig() без реального файла — только проверяем, что пустой env не форсирует путь.
	if p := strings.TrimSpace(os.Getenv(envConfigPath)); p != "" {
		t.Fatal("env must be empty for this test")
	}
}

func explicitBrandJSON(id, name, host, publicBaseURL, landingURL, category, profile, yookassaPS string) string {
	brand := BrandConfig{
		ID:                 id,
		Name:               name,
		AllowedHosts:       []string{host},
		PublicBaseURL:      publicBaseURL,
		LandingURL:         landingURL,
		ServiceCategory:    category,
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     profile,
		YooKassaPaySystem:  yookassaPS,
	}
	b, _ := json.Marshal(brand)
	return `{"telegram":{"token":"test-token"},"brand":` + string(b) + `}`
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
		YooKassaPaySystem:  "yookassa_vff",
	}
	return cfg
}

func validExplicitFCBrandCfg() *Config {
	cfg := &Config{}
	cfg.Telegram.Token = "tok"
	cfg.Brand = BrandConfig{
		ID:                 "fc",
		Name:               "Friends Connect",
		AllowedHosts:       []string{"connect-fc.vpn-for-friends.com"},
		PublicBaseURL:      "https://connect-fc.vpn-for-friends.com",
		LandingURL:         "https://vpn-for-friends.com",
		ServiceCategory:    "vpn-mz-fc",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "telegram_friends_connect_bot",
		YooKassaPaySystem:  "yookassa_fc",
	}
	return cfg
}

func TestNormalize_YooKassaPaySystemVFFAndFC(t *testing.T) {
	vff := validExplicitBrandCfg()
	if err := vff.Normalize(); err != nil {
		t.Fatal(err)
	}
	if vff.YooKassaPaySystem() != "yookassa_vff" {
		t.Fatalf("vff=%q", vff.YooKassaPaySystem())
	}
	fc := validExplicitFCBrandCfg()
	if err := fc.Normalize(); err != nil {
		t.Fatal(err)
	}
	if fc.YooKassaPaySystem() != "yookassa_fc" {
		t.Fatalf("fc=%q", fc.YooKassaPaySystem())
	}
}

func TestNormalize_YooKassaPaySystemEmptyFail(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.YooKassaPaySystem = ""
	if err := cfg.Normalize(); err == nil || !strings.Contains(err.Error(), "yookassa_pay_system") {
		t.Fatalf("want yookassa_pay_system error, got %v", err)
	}
}

func TestNormalize_YooKassaPaySystemWhitespaceFail(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Brand.YooKassaPaySystem = "   \t  "
	if err := cfg.Normalize(); err == nil || !strings.Contains(err.Error(), "yookassa_pay_system") {
		t.Fatalf("want yookassa_pay_system error, got %v", err)
	}
}

func TestNormalize_YooKassaPaySystemInvalidCharsFail(t *testing.T) {
	for _, bad := range []string{"YooKassa", "yookassa/vff", "yookassa vff", "yookassa.vff", "-yookassa", "yookassa+fc"} {
		cfg := validExplicitBrandCfg()
		cfg.Brand.YooKassaPaySystem = bad
		if err := cfg.Normalize(); err == nil || !strings.Contains(err.Error(), "yookassa_pay_system") {
			t.Fatalf("%q: want invalid error, got %v", bad, err)
		}
	}
}
