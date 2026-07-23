package bot

import (
	"net/url"
	"strings"
	"testing"
)

func TestTelegramPaymentsWebAppURL_VFFProfile(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 42, "telegram_bot")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/shm/v1/public/tg_payments_webapp" {
		t.Fatalf("path=%q", u.Path)
	}
	q := u.Query()
	if q.Get("format") != "html" || q.Get("user_id") != "42" {
		t.Fatalf("query=%v", q)
	}
	if q.Get("profile") != "telegram_bot" {
		t.Fatalf("profile=%q", q.Get("profile"))
	}
}

func TestTelegramPaymentsWebAppURL_FCProfile(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example/", 99, "telegram_friends_connect_bot")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("profile") != "telegram_friends_connect_bot" {
		t.Fatalf("profile=%q", u.Query().Get("profile"))
	}
	if strings.Contains(got, "profile=telegram_bot") && !strings.Contains(got, "telegram_friends_connect_bot") {
		t.Fatalf("must not substitute VFF profile: %s", got)
	}
	// FC profile must not be rewritten to telegram_bot
	if u.Query().Get("profile") == "telegram_bot" {
		t.Fatal("FC must keep telegram_friends_connect_bot")
	}
}

func TestTelegramPaymentsWebAppURL_EmptyProfileFailClosed(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 1, "")
	if err == nil {
		t.Fatal("empty profile must fail")
	}
	if got != "" {
		t.Fatalf("url must be empty, got %q", got)
	}
	if !strings.Contains(err.Error(), "payment profile is empty") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "telegram_bot") {
		t.Fatalf("must not mention fallback profile: %v", err)
	}
}

func TestTelegramPaymentsWebAppURL_WhitespaceProfileFailClosed(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 1, "   \t  ")
	if err == nil || got != "" {
		t.Fatalf("whitespace profile must fail-closed, got url=%q err=%v", got, err)
	}
}

func TestTelegramPaymentsWebAppURL_EncodesSpecialCharacters(t *testing.T) {
	t.Parallel()
	// BrandConfig currently only requires non-empty profile; encoding must be safe.
	profile := "telegram_bot+extra/test"
	got, err := telegramPaymentsWebAppURL("https://bill.example", 7, profile)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("profile") != profile {
		t.Fatalf("decoded profile=%q want %q; raw=%s", u.Query().Get("profile"), profile, got)
	}
	wantEnc := url.QueryEscape(profile)
	if !strings.Contains(u.RawQuery, wantEnc) {
		t.Fatalf("raw query must encode profile as %q; got %q", wantEnc, u.RawQuery)
	}
}

func TestTelegramPaymentsWebAppURL_TrimsBaseSlashRegression(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example/", 3, "telegram_bot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://bill.example/shm/v1/public/tg_payments_webapp?") {
		t.Fatalf("url=%q", got)
	}
	if strings.Contains(got, "//shm/") {
		t.Fatalf("double slash: %q", got)
	}
}

func TestTelegramPaymentsWebAppURL_InvalidUserOrBase(t *testing.T) {
	t.Parallel()
	if _, err := telegramPaymentsWebAppURL("https://x", 0, "telegram_bot"); err == nil {
		t.Fatal("user id 0 must fail")
	}
	if _, err := telegramPaymentsWebAppURL("  ", 1, "telegram_bot"); err == nil {
		t.Fatal("empty base must fail")
	}
}
