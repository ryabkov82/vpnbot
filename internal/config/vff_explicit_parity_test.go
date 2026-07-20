package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExplicitVFFParityWithLegacy(t *testing.T) {
	legacy := &Config{}
	legacy.Telegram.Token = "test-token"
	legacy.Services.Category = "vpn-mz-test"
	legacy.WebSales.PublicBaseURL = "https://connect.vpn-for-friends.com"
	legacy.Payments.Profile = "telegram_bot"
	if err := legacy.Normalize(); err != nil {
		t.Fatal(err)
	}

	explicit := &Config{}
	explicit.Telegram.Token = "test-token"
	explicit.Services.Category = "vpn-mz-test" // legacy fields retained, unused when brand.id set
	explicit.WebSales.PublicBaseURL = "https://connect.vpn-for-friends.com"
	explicit.Payments.Profile = "telegram_bot"
	explicit.Brand = BrandConfig{
		ID:                 "vff",
		Name:               "VPN for Friends",
		AllowedHosts:       []string{"connect.vpn-for-friends.com"},
		PublicBaseURL:      "https://connect.vpn-for-friends.com",
		LandingURL:         "https://vpn-for-friends.com",
		ServiceCategory:    "vpn-mz-test",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "telegram_bot",
	}
	if err := explicit.Normalize(); err != nil {
		t.Fatal(err)
	}

	if explicit.ServiceCategory() != legacy.ServiceCategory() {
		t.Fatalf("ServiceCategory: explicit=%q legacy=%q", explicit.ServiceCategory(), legacy.ServiceCategory())
	}
	if explicit.PublicBaseURL() != legacy.PublicBaseURL() {
		t.Fatalf("PublicBaseURL: explicit=%q legacy=%q", explicit.PublicBaseURL(), legacy.PublicBaseURL())
	}
	if explicit.PaymentProfile() != legacy.PaymentProfile() {
		t.Fatalf("PaymentProfile: explicit=%q legacy=%q", explicit.PaymentProfile(), legacy.PaymentProfile())
	}
	if explicit.WebUserLoginPrefix() != legacy.WebUserLoginPrefix() {
		t.Fatalf("WebUserLoginPrefix mismatch")
	}
	if explicit.WebUserSource() != legacy.WebUserSource() {
		t.Fatalf("WebUserSource mismatch")
	}

	b := explicit.EffectiveBrand()
	if b.ID != "vff" {
		t.Fatalf("id=%q", b.ID)
	}
	if len(b.AllowedHosts) != 1 || b.AllowedHosts[0] != "connect.vpn-for-friends.com" {
		t.Fatalf("hosts=%#v", b.AllowedHosts)
	}
	if b.LandingURL != "https://vpn-for-friends.com" {
		t.Fatalf("landing=%q", b.LandingURL)
	}
}

func TestFormatSafeBrandSummary_NoSecrets(t *testing.T) {
	cfg := validExplicitBrandCfg()
	cfg.Telegram.Token = "SECRET-TELEGRAM-TOKEN-VALUE"
	cfg.API.APILogin = "secret-api-login"
	cfg.API.APIPass = "secret-api-pass"
	cfg.Email.SMTPPassword = "secret-smtp-password"
	cfg.WebSales.OrderTokenSecret = "secret-order-token"
	cfg.WebAccount.GoogleClientSecret = "secret-google-oauth"
	cfg.PremiumLinkSigningSecret = "secret-premium-signing"
	cfg.Admin.Token = "secret-admin-token"
	cfg.RemnawaveAPIToken = "secret-remnawave-token"

	out := FormatSafeBrandSummary(cfg)
	if !strings.HasPrefix(out, "config valid\n") {
		t.Fatalf("prefix: %q", out)
	}
	for _, secret := range []string{
		"SECRET-TELEGRAM-TOKEN-VALUE",
		"secret-api-login",
		"secret-api-pass",
		"secret-smtp-password",
		"secret-order-token",
		"secret-google-oauth",
		"secret-premium-signing",
		"secret-admin-token",
		"secret-remnawave-token",
		`"token"`,
		"api_pass",
	} {
		if strings.Contains(out, secret) {
			t.Fatalf("summary leaked %q: %s", secret, out)
		}
	}
	for _, need := range []string{
		"brand.id=vff",
		"brand.name=VPN for Friends",
		"brand.public_base_url=https://connect.vpn-for-friends.com",
		"brand.service_category=vpn-mz-main",
		"brand.allowed_hosts=connect.vpn-for-friends.com",
		"brand.web_user_login_prefix=web_",
		"brand.web_user_source=vpn-for-friends.com",
		"brand.payment_profile=telegram_bot",
	} {
		if !strings.Contains(out, need) {
			t.Fatalf("missing %q in %s", need, out)
		}
	}

	line := FormatActiveBrandLogLine(cfg)
	if !strings.Contains(line, `active brand: id=vff name="VPN for Friends"`) {
		t.Fatalf("log line: %s", line)
	}
	if strings.Contains(line, "SECRET-TELEGRAM-TOKEN-VALUE") {
		t.Fatal("startup log leaked telegram token")
	}
}

func TestConfigcheckBinary_NoSecretsInStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	body := `{
		"telegram": {"token": "SECRET-TELEGRAM-TOKEN-VALUE"},
		"api": {"api_login": "secret-api-login", "api_pass": "secret-api-pass"},
		"brand": {
			"id": "vff",
			"name": "VPN for Friends",
			"allowed_hosts": ["connect.vpn-for-friends.com"],
			"public_base_url": "https://connect.vpn-for-friends.com",
			"landing_url": "https://vpn-for-friends.com",
			"service_category": "vpn-mz-test",
			"web_user_login_prefix": "web_",
			"web_user_source": "vpn-for-friends.com",
			"payment_profile": "telegram_bot"
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	modRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	cmd := exec.Command("go", "run", "./cmd/configcheck", "-config", path)
	cmd.Dir = modRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("configcheck failed: %v\n%s", err, out)
	}
	s := string(out)
	if strings.Contains(s, "SECRET-TELEGRAM-TOKEN-VALUE") || strings.Contains(s, "secret-api-pass") {
		t.Fatalf("configcheck leaked secrets:\n%s", s)
	}
	if !strings.Contains(s, "config valid") || !strings.Contains(s, "brand.id=vff") {
		t.Fatalf("unexpected output:\n%s", s)
	}
}

func TestVPNBOT_CONFIG_InvalidFileNoFallback(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"telegram":{"token":"t"},"brand":{"id":"vff"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Also place a valid legacy config that must NOT be used as fallback.
	goodLegacy := filepath.Join(dir, "config.json")
	if err := os.WriteFile(goodLegacy, []byte(`{
		"telegram":{"token":"t"},
		"services":{"category":"vpn-mz-main"},
		"web_sales":{"public_base_url":"https://connect.vpn-for-friends.com"},
		"payments":{"profile":"telegram_bot"}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envConfigPath, bad)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	_, err = loadConfig()
	if err == nil {
		t.Fatal("invalid explicit brand must fail")
	}
	if !strings.Contains(err.Error(), "brand.") {
		t.Fatalf("want brand validation error, got %v", err)
	}
}
