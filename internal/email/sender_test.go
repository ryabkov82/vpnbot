package email

import (
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
