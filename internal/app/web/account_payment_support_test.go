package web

import (
	"strings"
	"testing"
)

func clearSupportURLEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envTelegramSupportURL, "")
	t.Setenv(envTelegramSupportURLLegacy, "")
}

func TestRenderedPaymentSupport_VFF_RUAndEN(t *testing.T) {
	clearSupportURLEnv(t)
	const support = "https://t.me/vpn_for_friends_support"
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = support

	ru := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	for _, needle := range []string{
		`id="topup-payment-methods"`,
		`Поддержка: <a href="` + support + `" target="_blank" rel="noopener noreferrer">Telegram</a>`,
		`name="topup-payment-method" value="yookassa" checked`,
		`name="topup-payment-method" value="cryptocloud"`,
	} {
		if !strings.Contains(ru, needle) {
			t.Fatalf("VFF RU payment support missing %q", needle)
		}
	}
	for _, forbid := range []string{
		"friends_connect_support",
		"support@vpn-for-friends.com",
		"mailto:",
	} {
		if strings.Contains(ru, forbid) {
			t.Fatalf("VFF RU payment support must not contain %q", forbid)
		}
	}

	en := mustRenderAccountSessionHTML(t, cfg, accountLocaleEN)
	for _, needle := range []string{
		`Support: <a href="` + support + `" target="_blank" rel="noopener noreferrer">Telegram</a>`,
		`name="topup-payment-method" value="cryptocloud"`,
	} {
		if !strings.Contains(en, needle) {
			t.Fatalf("VFF EN payment support missing %q", needle)
		}
	}
	for _, forbid := range []string{
		"friends_connect_support",
		"support@vpn-for-friends.com",
		"mailto:",
		`value="yookassa"`,
	} {
		if strings.Contains(en, forbid) {
			t.Fatalf("VFF EN payment support must not contain %q", forbid)
		}
	}
}

func TestRenderedPaymentSupport_FriendsConnect_RUAndEN(t *testing.T) {
	clearSupportURLEnv(t)
	const support = "https://t.me/friends_connect_support"
	cfg := friendsConnectAccountTestCfg()
	cfg.Telegram.SupportChat = support

	ru := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	if !strings.Contains(ru, `Поддержка: <a href="`+support+`" target="_blank" rel="noopener noreferrer">Telegram</a>`) {
		t.Fatal("FC RU payment support link missing")
	}
	for _, forbid := range []string{
		"vpn_for_friends_support",
		"support@vpn-for-friends.com",
		"mailto:",
	} {
		if strings.Contains(ru, forbid) {
			t.Fatalf("FC RU payment support must not contain %q", forbid)
		}
	}

	en := mustRenderAccountSessionHTML(t, cfg, accountLocaleEN)
	if !strings.Contains(en, `Support: <a href="`+support+`" target="_blank" rel="noopener noreferrer">Telegram</a>`) {
		t.Fatal("FC EN payment support link missing")
	}
	for _, forbid := range []string{
		"vpn_for_friends_support",
		"support@vpn-for-friends.com",
		"mailto:",
	} {
		if strings.Contains(en, forbid) {
			t.Fatalf("FC EN payment support must not contain %q", forbid)
		}
	}
}

func TestRenderedPaymentSupport_MissingOmitsLink(t *testing.T) {
	clearSupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = ""

	ru := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	if !strings.Contains(ru, `id="topup-payment-methods"`) {
		t.Fatal("payment methods must still render")
	}
	if !strings.Contains(ru, `name="topup-payment-method" value="yookassa"`) {
		t.Fatal("RU payment methods regression")
	}
	for _, forbid := range []string{
		`href="https://t.me/`,
		">Telegram</a>",
		"friends_connect_support",
		"support@vpn-for-friends.com",
		"mailto:",
	} {
		if strings.Contains(ru, forbid) {
			t.Fatalf("empty support must omit contact fallback %q", forbid)
		}
	}
}

func TestRenderedPaymentSupport_InvalidOmitsLink(t *testing.T) {
	clearSupportURLEnv(t)
	cfg := orderStartTestCfg()
	cfg.Telegram.SupportChat = "javascript:alert(1)"

	ru := mustRenderAccountSessionHTML(t, cfg, accountLocaleRU)
	if !strings.Contains(ru, `id="topup-payment-methods"`) {
		t.Fatal("payment methods must still render")
	}
	if strings.Contains(ru, "javascript:") || strings.Contains(ru, ">Telegram</a>") {
		t.Fatal("invalid support must not appear in payment modal HTML")
	}
	if strings.Contains(ru, "friends_connect_support") || strings.Contains(ru, "support@vpn-for-friends.com") {
		t.Fatal("invalid support must not fall back to hardcoded contacts")
	}
}
