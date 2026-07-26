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
	cfg.Brand.ID = "vff"
	cfg.Brand.Name = "VPN for Friends"
	cfg.Brand.PublicBaseURL = "https://shop.example"
	cfg.Brand.LandingURL = "https://vpn-for-friends.com"
	cfg.Brand.WebUserLoginPrefix = "web_"
	cfg.Brand.WebUserSource = "vpn-for-friends.com"
	cfg.Brand.YooKassaPaySystem = "yookassa"
	cfg.Email.Enabled = true
	cfg.Email.SMTPHost = "smtp.test"
	cfg.Email.SMTPPort = 587
	cfg.Email.SMTPUsername = "u"
	cfg.Email.SMTPPassword = "pw"
	cfg.Email.FromEmail = "noreply@test"
	return cfg
}

func friendsConnectAccountTestCfg() *config.Config {
	cfg := orderStartTestCfg()
	cfg.Brand.ID = "fc"
	cfg.Brand.Name = "Friends Connect"
	cfg.Brand.AllowedHosts = []string{"connect.friends-connect.club"}
	cfg.Brand.PublicBaseURL = "https://connect.friends-connect.club"
	cfg.Brand.LandingURL = "https://friends-connect.club"
	cfg.Brand.ServiceCategory = "vpn-mz-fc"
	cfg.Brand.WebUserLoginPrefix = "web_fc_"
	cfg.Brand.PaymentProfile = "telegram_friends_connect_bot"
	return cfg
}

func patchSMTP(t *testing.T, fn func(addr string, a smtp.Auth, from string, to []string, msg []byte) error) {
	old := email.SendMail
	email.SendMail = fn
	t.Cleanup(func() { email.SendMail = old })
}
