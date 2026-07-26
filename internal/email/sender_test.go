package email

import (
	"errors"
	"net/smtp"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestIsConfigured(t *testing.T) {
	if IsConfigured(nil) {
		t.Fatal("nil")
	}
	cfg := &config.Config{}
	if IsConfigured(cfg) {
		t.Fatal("empty")
	}
	cfg.Email.Enabled = true
	cfg.Email.SMTPHost = "h"
	cfg.Email.FromEmail = "f@x"
	cfg.Email.SMTPUsername = "u"
	cfg.Email.SMTPPassword = "p"
	if !IsConfigured(cfg) {
		t.Fatal("should be configured")
	}
	cfg.Email.FromEmail = ""
	if IsConfigured(cfg) {
		t.Fatal("missing from")
	}
}

func configuredEmailCfg(brandName string) *config.Config {
	cfg := &config.Config{}
	cfg.Brand.ID = "test"
	cfg.Brand.Name = brandName
	cfg.Email.Enabled = true
	cfg.Email.SMTPHost = "smtp.test"
	cfg.Email.SMTPPort = 587
	cfg.Email.SMTPUsername = "u"
	cfg.Email.SMTPPassword = "p"
	cfg.Email.FromEmail = "noreply@test.example"
	return cfg
}

func captureSendMail(t *testing.T) *string {
	t.Helper()
	prev := SendMail
	var captured string
	SendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		captured = string(msg)
		return nil
	}
	t.Cleanup(func() { SendMail = prev })
	return &captured
}

func forbidSendMail(t *testing.T) {
	t.Helper()
	prev := SendMail
	SendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		t.Fatal("SendMail must not be called")
		return nil
	}
	t.Cleanup(func() { SendMail = prev })
}

func TestSendAccountLoginEmail_VFFUnchanged(t *testing.T) {
	msg := captureSendMail(t)
	cfg := configuredEmailCfg("VPN for Friends")
	if err := SendAccountLoginEmail(cfg, "user@example.com", "https://example/session?token=x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*msg, "Subject: VPN for Friends — вход в личный кабинет\r\n") {
		t.Fatalf("subject: %s", *msg)
	}
	if !strings.Contains(*msg, "\r\n\r\nVPN for Friends\r\n") {
		t.Fatalf("body brand line: %s", *msg)
	}
	if !strings.Contains(*msg, `From: "VPN for Friends" <noreply@test.example>`) {
		t.Fatalf("from: %s", *msg)
	}
}

func TestSendAccountLoginEmail_FCBrandAware(t *testing.T) {
	msg := captureSendMail(t)
	cfg := configuredEmailCfg("Friends Connect")
	cfg.Brand.ID = "fc"
	if err := SendAccountLoginEmail(cfg, "user@example.com", "https://example/session?token=x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*msg, "Subject: Friends Connect — вход в личный кабинет\r\n") {
		t.Fatalf("subject: %s", *msg)
	}
	if !strings.Contains(*msg, "\r\n\r\nFriends Connect\r\n") {
		t.Fatalf("body brand line: %s", *msg)
	}
	if !strings.Contains(*msg, `From: "Friends Connect" <noreply@test.example>`) {
		t.Fatalf("from: %s", *msg)
	}
	if strings.Contains(*msg, "VPN for Friends") {
		t.Fatalf("must not contain VFF: %s", *msg)
	}
}

func TestSendAccountLoginEmail_TrimmedBrandName(t *testing.T) {
	msg := captureSendMail(t)
	cfg := configuredEmailCfg(" Friends Connect ")
	cfg.Brand.ID = "fc"
	if err := SendAccountLoginEmail(cfg, "user@example.com", "https://example/session?token=x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*msg, "Subject: Friends Connect — вход в личный кабинет\r\n") {
		t.Fatalf("trimmed subject: %s", *msg)
	}
	if !strings.Contains(*msg, "\r\n\r\nFriends Connect\r\n") {
		t.Fatalf("trimmed body: %s", *msg)
	}
}

func TestSendAccountLinkConfirmEmail_FCBrandAware(t *testing.T) {
	msg := captureSendMail(t)
	cfg := configuredEmailCfg("Friends Connect")
	cfg.Brand.ID = "fc"
	if err := SendAccountLinkConfirmEmail(cfg, "user@example.com", "https://example/link/confirm?token=y"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*msg, "Subject: Friends Connect — подтвердите привязку личного кабинета\r\n") {
		t.Fatalf("subject: %s", *msg)
	}
	if !strings.Contains(*msg, "\r\n\r\nFriends Connect\r\n") {
		t.Fatalf("body brand line: %s", *msg)
	}
	if strings.Contains(*msg, "VPN for Friends") {
		t.Fatalf("must not contain VFF: %s", *msg)
	}
}

func TestSendAccountLoginEmail_ExplicitFromNamePriority(t *testing.T) {
	msg := captureSendMail(t)
	cfg := configuredEmailCfg("Friends Connect")
	cfg.Brand.ID = "fc"
	cfg.Email.FromName = "Custom Sender"
	if err := SendAccountLoginEmail(cfg, "user@example.com", "https://example/session?token=x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*msg, `From: "Custom Sender" <noreply@test.example>`) {
		t.Fatalf("from must use explicit from_name: %s", *msg)
	}
	if !strings.Contains(*msg, "Subject: Friends Connect — вход в личный кабинет\r\n") {
		t.Fatalf("subject must still use brand.name: %s", *msg)
	}
	if !strings.Contains(*msg, "\r\n\r\nFriends Connect\r\n") {
		t.Fatalf("body must still use brand.name: %s", *msg)
	}
}

func TestSendAccountLoginEmail_FailClosedBrandName(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil", cfg: nil},
		{name: "empty", cfg: configuredEmailCfg("")},
		{name: "whitespace", cfg: configuredEmailCfg("  \t  ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forbidSendMail(t)
			err := SendAccountLoginEmail(tc.cfg, "user@example.com", "https://example/session?token=x")
			if !errors.Is(err, ErrBrandNameRequired) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestSendAccountLoginEmail_FromNameDoesNotBypassBrandName(t *testing.T) {
	forbidSendMail(t)
	cfg := configuredEmailCfg("")
	cfg.Email.FromName = "Custom Sender"
	err := SendAccountLoginEmail(cfg, "user@example.com", "https://example/session?token=x")
	if !errors.Is(err, ErrBrandNameRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestBrandDisplayName_StripsCRLF(t *testing.T) {
	cfg := &config.Config{}
	cfg.Brand.Name = "Friends\r\nConnect"
	got, err := brandDisplayName(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != "FriendsConnect" {
		t.Fatalf("got %q", got)
	}
}

func TestBrandDisplayName_FailClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil", cfg: nil},
		{name: "empty", cfg: &config.Config{}},
		{name: "whitespace", cfg: func() *config.Config {
			c := &config.Config{}
			c.Brand.Name = " \t "
			return c
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := brandDisplayName(tc.cfg)
			if !errors.Is(err, ErrBrandNameRequired) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
