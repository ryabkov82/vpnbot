package web

import (
	"net/smtp"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/email"
)

func orderStartTestCfg() *config.Config {
	cfg := &config.Config{}
	cfg.WebSales.OrderTokenSecret = "order-token-secret-order-token-sec"
	cfg.Brand.PublicBaseURL = "https://shop.example"
	cfg.Email.Enabled = true
	cfg.Email.SMTPHost = "smtp.test"
	cfg.Email.SMTPPort = 587
	cfg.Email.SMTPUsername = "u"
	cfg.Email.SMTPPassword = "pw"
	cfg.Email.FromEmail = "noreply@test"
	return cfg
}

func patchSMTP(t *testing.T, fn func(addr string, a smtp.Auth, from string, to []string, msg []byte) error) {
	old := email.SendMail
	email.SendMail = fn
	t.Cleanup(func() { email.SendMail = old })
}
