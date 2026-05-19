package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestSendLeadTelegramNotification_UsesLeadsChatID(t *testing.T) {
	old := leadTelegramHTTPPost
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
		if !strings.Contains(payload.Text, "VPN for Friends") {
			t.Fatalf("text missing banner: %q", payload.Text)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Telegram.Token = "test-token"
	cfg.Telegram.SupportChatID = 111
	cfg.Telegram.LeadsChatID = 999

	sendLeadTelegramNotification(cfg, publicLead{
		ServiceID: 7,
		Email:     "u@example.com",
		Contact:   "@tg",
	}, "Premium 1 мес.", "203.0.113.1")

	if gotChat != 999 {
		t.Fatalf("chat_id: got %d, want 999", gotChat)
	}
}

func TestSendLeadTelegramNotification_FallbackToSupportChatID(t *testing.T) {
	old := leadTelegramHTTPPost
	var gotChat int64
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(req.Body)
		var payload struct {
			ChatID int64 `json:"chat_id"`
		}
		_ = json.Unmarshal(raw, &payload)
		gotChat = payload.ChatID
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Telegram.Token = "test-token"
	cfg.Telegram.SupportChatID = 555
	cfg.Telegram.LeadsChatID = 0

	sendLeadTelegramNotification(cfg, publicLead{ServiceID: 1, Email: "a@b.c"}, "Tariff", "1.2.3.4")
	if gotChat != 555 {
		t.Fatalf("chat_id: got %d, want 555", gotChat)
	}
}

func TestSendLeadTelegramNotification_NoChatSkipsHTTP(t *testing.T) {
	old := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		t.Fatal("telegram HTTP must not be called when chat ids are 0")
		return nil, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Telegram.Token = "test-token"
	cfg.Telegram.SupportChatID = 0
	cfg.Telegram.LeadsChatID = 0

	sendLeadTelegramNotification(cfg, publicLead{ServiceID: 1, Email: "a@b.c"}, "T", "9.9.9.9")
}

func TestSendLeadTelegramNotification_EmptyContactUsesDash(t *testing.T) {
	old := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(req.Body)
		var payload struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(raw, &payload)
		if !strings.Contains(payload.Text, "Контакт: —") {
			t.Fatalf("want em dash placeholder, got %q", payload.Text)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Telegram.Token = "x"
	cfg.Telegram.LeadsChatID = 1

	sendLeadTelegramNotification(cfg, publicLead{ServiceID: 2, Email: "e@e.e", Contact: ""}, "Name", "0.0.0.0")
}
