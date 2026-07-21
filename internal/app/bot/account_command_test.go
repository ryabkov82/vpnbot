package bot

import (
	"net/url"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"gopkg.in/telebot.v3"
)

func TestBotMenuCommands_HasAccount(t *testing.T) {
	t.Parallel()
	var found bool
	for _, c := range botMenuCommands() {
		if c.Text == "/account" && c.Description == "Личный кабинет (NEW)" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("commands=%v", botMenuCommands())
	}
}

func TestAccountCommandReply_MessageAndInlineURLButton(t *testing.T) {
	const chatID int64 = 44001122
	const shmUID = 17
	base := "https://cabinet.example.com"
	secret := strings.Repeat("a", 40)
	cfg := &config.Config{}
	cfg.Brand.PublicBaseURL = base
	cfg.WebSales.OrderTokenSecret = secret
	s := NewService(nil, cfg)
	reply := s.accountCommandReply(chatID, shmUID)

	if !strings.Contains(reply.Message, "🌐 Личный кабинет (NEW)") {
		t.Fatalf("message=%q", reply.Message)
	}
	if !strings.Contains(reply.Message, "web-кабинет") {
		t.Fatal("missing web-cabinet explanation")
	}
	if reply.ButtonText != webCabinetCommandButtonLabel {
		t.Fatalf("button text=%q", reply.ButtonText)
	}
	if reply.ButtonURL == "" {
		t.Fatal("expected cabinet URL")
	}
	u, err := url.Parse(reply.ButtonURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme+"://"+u.Host != strings.TrimRight(base, "/") {
		t.Fatalf("host=%q want base %q", u.Scheme+"://"+u.Host, base)
	}
	if u.Path != "/account/link" {
		t.Fatalf("path=%q", u.Path)
	}
	if strings.TrimSpace(u.Query().Get("token")) == "" {
		t.Fatalf("missing link token in %q", reply.ButtonURL)
	}
}

func TestTelegramWebCabinetURL_UsesPublicBaseURL(t *testing.T) {
	base := "https://connect.vpn-for-friends.com/"
	secret := strings.Repeat("b", 40)
	cfg := &config.Config{}
	cfg.Brand.PublicBaseURL = base
	cfg.WebSales.OrderTokenSecret = secret
	s := NewService(nil, cfg)
	got := s.telegramWebCabinetURL(99112233, 3)
	wantPrefix := "https://connect.vpn-for-friends.com/account/link?token="
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("url=%q want prefix %q", got, wantPrefix)
	}
}

func TestWebCabinetMenuButton_ReusesURLHelper(t *testing.T) {
	base := "https://site.test"
	secret := strings.Repeat("c", 40)
	cfg := &config.Config{}
	cfg.Brand.PublicBaseURL = base
	cfg.WebSales.OrderTokenSecret = secret
	s := NewService(nil, cfg)
	m := &telebot.ReplyMarkup{}
	btn := s.webCabinetMenuButton(m, 1, 2)
	if btn == nil {
		t.Fatal("expected menu button")
	}
	reply := s.accountCommandReply(1, 2)
	if btn.URL != reply.ButtonURL {
		t.Fatalf("menu URL=%q account URL=%q", btn.URL, reply.ButtonURL)
	}
}
