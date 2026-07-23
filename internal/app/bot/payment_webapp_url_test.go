package bot

import (
	"net/url"
	"strings"
	"testing"
)

func TestTelegramPaymentsWebAppURL_VFF(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 42, "telegram_bot", "yookassa")
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
	if q.Get("yookassa_ps") != "yookassa" {
		t.Fatalf("yookassa_ps=%q", q.Get("yookassa_ps"))
	}
}

func TestTelegramPaymentsWebAppURL_FC(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example/", 99, "telegram_friends_connect_bot", "yookassa")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("profile") != "telegram_friends_connect_bot" {
		t.Fatalf("profile=%q", q.Get("profile"))
	}
	if q.Get("yookassa_ps") != "yookassa" {
		t.Fatalf("yookassa_ps=%q", q.Get("yookassa_ps"))
	}
	if q.Get("profile") == "telegram_bot" {
		t.Fatalf("must keep FC profile: %s", got)
	}
}

func TestTelegramPaymentsWebAppURL_EmptyProfileFailClosed(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 1, "", "yookassa")
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

func TestTelegramPaymentsWebAppURL_EmptyYooKassaPSFailClosed(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 1, "telegram_bot", "")
	if err == nil || got != "" {
		t.Fatalf("empty yookassa_ps must fail-closed, got url=%q err=%v", got, err)
	}
	if !strings.Contains(err.Error(), "yookassa pay system is empty") {
		t.Fatalf("err=%v", err)
	}
}

func TestTelegramPaymentsWebAppURL_WhitespaceProfileFailClosed(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 1, "   \t  ", "yookassa")
	if err == nil || got != "" {
		t.Fatalf("whitespace profile must fail-closed, got url=%q err=%v", got, err)
	}
}

func TestTelegramPaymentsWebAppURL_WhitespaceYooKassaPSFailClosed(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example", 1, "telegram_bot", "  \t ")
	if err == nil || got != "" {
		t.Fatalf("whitespace yookassa_ps must fail-closed, got url=%q err=%v", got, err)
	}
}

func TestTelegramPaymentsWebAppURL_EncodesSpecialCharacters(t *testing.T) {
	t.Parallel()
	profile := "telegram_bot+extra/test"
	ps := "yookassa+extra"
	got, err := telegramPaymentsWebAppURL("https://bill.example", 7, profile, ps)
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
	if u.Query().Get("yookassa_ps") != ps {
		t.Fatalf("decoded yookassa_ps=%q want %q; raw=%s", u.Query().Get("yookassa_ps"), ps, got)
	}
	if !strings.Contains(u.RawQuery, url.QueryEscape(profile)) {
		t.Fatalf("raw query must encode profile; got %q", u.RawQuery)
	}
	if !strings.Contains(u.RawQuery, url.QueryEscape(ps)) {
		t.Fatalf("raw query must encode yookassa_ps; got %q", u.RawQuery)
	}
}

func TestTelegramPaymentsWebAppURL_TrimsBaseSlashRegression(t *testing.T) {
	t.Parallel()
	got, err := telegramPaymentsWebAppURL("https://bill.example/", 3, "telegram_bot", "yookassa")
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
	if _, err := telegramPaymentsWebAppURL("https://x", 0, "telegram_bot", "yookassa"); err == nil {
		t.Fatal("user id 0 must fail")
	}
	if _, err := telegramPaymentsWebAppURL("  ", 1, "telegram_bot", "yookassa"); err == nil {
		t.Fatal("empty base must fail")
	}
}
