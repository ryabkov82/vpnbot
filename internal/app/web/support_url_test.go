package web

import (
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func cfgWithSupport(chat string) *config.Config {
	c := &config.Config{}
	c.Telegram.SupportChat = chat
	return c
}

func TestWebCabinetResolvedSupportURL_empty(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	if got := WebCabinetResolvedSupportURL(cfgWithSupport("")); got != "" {
		t.Fatalf("want empty got %q", got)
	}
	if got := WebCabinetResolvedSupportURL(nil); got != "" {
		t.Fatal("nil cfg expected empty URL")
	}
}

func TestWebCabinetResolvedSupportURL_envOverridesConfig(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "https://env.example/help")
	t.Setenv(envTelegramSupportURLLegacy, "https://legacy.example/")
	got := WebCabinetResolvedSupportURL(cfgWithSupport("https://config.example/ignored"))
	if got != "https://env.example/help" {
		t.Fatalf("env override: got %q", got)
	}
}

func TestWebCabinetResolvedSupportURL_legacyEnvWhenPrimaryEmpty(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	t.Setenv(envTelegramSupportURLLegacy, "https://legacy.example/help")
	got := WebCabinetResolvedSupportURL(cfgWithSupport(""))
	if got != "https://legacy.example/help" {
		t.Fatalf("legacy env: got %q", got)
	}
}

func TestWebCabinetResolvedSupportURL_usernameAt(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	got := WebCabinetResolvedSupportURL(cfgWithSupport("@vpn_test_support"))
	if got != "https://t.me/vpn_test_support" {
		t.Fatalf("got %q", got)
	}
}

func TestWebCabinetResolvedSupportURL_tmeHostNoScheme(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	got := WebCabinetResolvedSupportURL(cfgWithSupport("t.me/vpn_test_support"))
	if got != "https://t.me/vpn_test_support" {
		t.Fatalf("got %q", got)
	}
}

func TestWebCabinetResolvedSupportURL_https(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	in := "https://example.org/support?page=1&x=y"
	got := WebCabinetResolvedSupportURL(cfgWithSupport(in))
	if got != in {
		t.Fatalf("got %q want %q", got, in)
	}
}

func TestWebCabinetResolvedSupportURL_tgScheme(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	in := "tg://resolve?domain=vpn_test_support"
	got := WebCabinetResolvedSupportURL(cfgWithSupport(in))
	if got != in {
		t.Fatalf("got %q want %q", got, in)
	}
}

func TestWebCabinetResolvedSupportURL_rejectsShortUsername(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	got := WebCabinetResolvedSupportURL(cfgWithSupport("ab"))
	if got != "" {
		t.Fatalf("expected empty for short username-ish string, got %q", got)
	}
}

func TestWebCabinetResolvedSupportURL_rejectsJavaScriptScheme(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	got := WebCabinetResolvedSupportURL(cfgWithSupport("javascript:alert(1)"))
	if got != "" {
		t.Fatal("must reject javascript:")
	}
	t.Setenv(envTelegramSupportURL, "JaVaScRiPt:alert(1)")
	got = WebCabinetResolvedSupportURL(cfgWithSupport(""))
	if got != "" {
		t.Fatal("must reject javascript: in env")
	}
}

func TestWebCabinetResolvedSupportURL_rejectsUnknownScheme(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	got := WebCabinetResolvedSupportURL(cfgWithSupport("ftp://example.com/x"))
	if got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderedAccountSessionPageHTML_supportLink(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	cfg := cfgWithSupport("@vpn_test_support")
	out := string(renderedAccountSessionPageHTML(cfg))
	if !strings.Contains(out, `href="https://t.me/vpn_test_support"`) {
		t.Fatalf("missing escaped support href; snippet around Поддержка not found:\n%s", out[:min(2500, len(out))])
	}
	if !strings.Contains(out, "Поддержка") || !strings.Contains(out, `target="_blank"`) || !strings.Contains(out, `rel="noopener noreferrer"`) {
		t.Fatal("expected support anchor attributes")
	}
	if strings.Contains(out, "<!--ACCOUNT_SESSION_SUPPORT_LINK_BLOCK-->") {
		t.Fatal("placeholder must be replaced")
	}
}

func TestRenderedAccountSessionPageHTML_emptySupportOmitsLink(t *testing.T) {
	t.Setenv(envTelegramSupportURL, "")
	out := string(renderedAccountSessionPageHTML(cfgWithSupport("")))
	if strings.Contains(out, ">Поддержка<") {
		t.Fatal("must omit support label when URL empty")
	}
	if strings.Contains(out, "<!--ACCOUNT_SESSION_SUPPORT_LINK_BLOCK-->") {
		t.Fatal("placeholder comment must be stripped when empty")
	}
}
