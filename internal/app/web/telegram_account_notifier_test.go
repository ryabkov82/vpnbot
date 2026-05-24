package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func patchAccountWebUserRegisteredTelegramNotifier(t *testing.T, fn func(cfg *config.Config, email string, userID int, login, ip string)) {
	t.Helper()
	prev := accountWebUserRegisteredTelegramNotifier
	accountWebUserRegisteredTelegramNotifier = fn
	t.Cleanup(func() { accountWebUserRegisteredTelegramNotifier = prev })
}

func TestSendAccountUserRegisteredTelegramNotification_MessageSent(t *testing.T) {
	old := leadTelegramHTTPPost
	var gotText string
	var gotChat int64
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		gotChat = payload.ChatID
		gotText = payload.Text
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Telegram.Token = "test-bot-token-field"
	cfg.Telegram.SupportChatID = 303
	cfg.Telegram.LeadsChatID = 9191

	prevHook := accountWebUserRegisteredTelegramNotifier
	accountWebUserRegisteredTelegramNotifier = sendAccountWebUserRegisteredTelegramImpl
	t.Cleanup(func() { accountWebUserRegisteredTelegramNotifier = prevHook })

	sendAccountUserRegisteredTelegramNotification(cfg, "who@test.com", 441, "web_abc441", "198.51.100.22")

	if gotChat != 9191 {
		t.Fatalf("chat_id=%d", gotChat)
	}
	for _, want := range []string{
		"🆕 Web user registered",
		"Email: who@test.com",
		"SHM user_id: 441",
		"Login: web_abc441",
		"IP: 198.51.100.22",
		"Пользователь подтвердил email и вошел в личный кабинет.",
	} {
		if !strings.Contains(gotText, want) {
			t.Fatalf("text missing %q in:\n%s", want, gotText)
		}
	}
}
