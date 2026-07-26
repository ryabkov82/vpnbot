package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestBuildLeadTelegramMessage_BrandMatrix(t *testing.T) {
	lead := publicLead{
		ServiceID: 7,
		Email:     "u@example.com",
		Contact:   "@user",
	}
	const (
		serviceName = "Premium 1 мес."
		ip          = "203.0.113.1"
	)
	wantBody := func(brand string) string {
		return "🆕 Заявка с сайта " + brand + "\n\n" +
			"Тариф: Premium 1 мес.\n" +
			"service_id: 7\n" +
			"Email: u@example.com\n" +
			"Контакт: @user\n" +
			"IP: 203.0.113.1"
	}

	for _, tc := range []struct {
		name      string
		brandName string
		want      string
		forbid    string
	}{
		{
			name:      "vff",
			brandName: "VPN for Friends",
			want:      wantBody("VPN for Friends"),
			forbid:    "Friends Connect",
		},
		{
			name:      "fc",
			brandName: "Friends Connect",
			want:      wantBody("Friends Connect"),
			forbid:    "VPN for Friends",
		},
		{
			name:      "fc_trimmed",
			brandName: " Friends Connect ",
			want:      wantBody("Friends Connect"),
			forbid:    "VPN for Friends",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Brand.Name = tc.brandName
			got, err := buildLeadTelegramMessage(cfg, lead, serviceName, ip)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("message mismatch\ngot:\n%s\nwant:\n%s", got, tc.want)
			}
			if strings.Contains(got, tc.forbid) {
				t.Fatalf("must not contain %q: %q", tc.forbid, got)
			}
		})
	}
}

func TestBuildLeadTelegramMessage_FailClosed(t *testing.T) {
	lead := publicLead{ServiceID: 1, Email: "a@b.c"}
	for _, tc := range []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil", cfg: nil},
		{name: "empty", cfg: func() *config.Config {
			c := &config.Config{}
			c.Brand.Name = ""
			return c
		}()},
		{name: "whitespace", cfg: func() *config.Config {
			c := &config.Config{}
			c.Brand.Name = "  \t  "
			return c
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildLeadTelegramMessage(tc.cfg, lead, "T", "1.1.1.1")
			if err == nil || !strings.Contains(err.Error(), "lead notification brand name is required") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestSendLeadTelegramNotification_UsesLeadsChatID(t *testing.T) {
	old := leadTelegramHTTPPost
	var gotChat int64
	var gotText string
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
	cfg.Brand.Name = "VPN for Friends"
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
	wantBanner := "🆕 Заявка с сайта VPN for Friends"
	if !strings.HasPrefix(gotText, wantBanner+"\n\n") {
		t.Fatalf("want exact VFF banner prefix %q, got %q", wantBanner, gotText)
	}
	if strings.Contains(gotText, "Friends Connect") {
		t.Fatalf("VFF message must not contain Friends Connect: %q", gotText)
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
	cfg.Brand.Name = "Friends Connect"
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
	cfg.Brand.Name = "VPN for Friends"
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
		if !strings.HasPrefix(payload.Text, "🆕 Заявка с сайта Friends Connect\n\n") {
			t.Fatalf("want FC banner, got %q", payload.Text)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Brand.Name = "Friends Connect"
	cfg.Telegram.Token = "x"
	cfg.Telegram.LeadsChatID = 1

	sendLeadTelegramNotification(cfg, publicLead{ServiceID: 2, Email: "e@e.e", Contact: ""}, "Name", "0.0.0.0")
}

func TestSendLeadTelegramNotification_EmptyBrandSkipsHTTP(t *testing.T) {
	old := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(req *http.Request) (*http.Response, error) {
		t.Fatal("telegram HTTP must not be called when brand name is empty")
		return nil, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	cfg := &config.Config{}
	cfg.Brand.Name = ""
	cfg.Telegram.Token = "test-token"
	cfg.Telegram.LeadsChatID = 999

	sendLeadTelegramNotification(cfg, publicLead{ServiceID: 1, Email: "a@b.c"}, "T", "9.9.9.9")
}
