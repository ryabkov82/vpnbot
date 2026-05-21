package email

import (
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

// SendMail подменяется в тестах (по умолчанию — net/smtp.SendMail).
var SendMail = smtp.SendMail

var ErrNotConfigured = errors.New("email not configured")

// IsConfigured — включённый SMTP с минимально необходимыми полями для web-order писем.
func IsConfigured(cfg *config.Config) bool {
	if cfg == nil || !cfg.Email.Enabled {
		return false
	}
	e := &cfg.Email
	if strings.TrimSpace(e.SMTPHost) == "" || strings.TrimSpace(e.FromEmail) == "" {
		return false
	}
	if strings.TrimSpace(e.SMTPUsername) == "" || strings.TrimSpace(e.SMTPPassword) == "" {
		return false
	}
	return true
}

func smtpPort(cfg *config.Config) int {
	p := cfg.Email.SMTPPort
	if p <= 0 {
		return 587
	}
	return p
}

func formatFrom(cfg *config.Config) string {
	name := strings.TrimSpace(cfg.Email.FromName)
	if name == "" {
		name = "VPN for Friends"
	}
	from := strings.TrimSpace(cfg.Email.FromEmail)
	if strings.ContainsAny(name, "\r\n\"") {
		name = strings.ReplaceAll(name, "\"", "'")
	}
	return fmt.Sprintf("%q <%s>", name, from)
}

func sendPlain(cfg *config.Config, to, subject, body string) error {
	if !IsConfigured(cfg) {
		return ErrNotConfigured
	}
	host := strings.TrimSpace(cfg.Email.SMTPHost)
	addr := fmt.Sprintf("%s:%d", host, smtpPort(cfg))
	auth := smtp.PlainAuth("", strings.TrimSpace(cfg.Email.SMTPUsername), cfg.Email.SMTPPassword, host)
	envelopeFrom := strings.TrimSpace(cfg.Email.FromEmail)
	msg := buildRFC822(formatFrom(cfg), to, subject, body)
	return SendMail(addr, auth, envelopeFrom, []string{to}, []byte(msg))
}

func buildRFC822(fromHeader, to, subject, body string) string {
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(fromHeader)
	b.WriteString("\r\nTo: ")
	b.WriteString(to)
	b.WriteString("\r\nSubject: ")
	b.WriteString(subject)
	b.WriteString("\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\r\n") {
		b.WriteString("\r\n")
	}
	return b.String()
}

// SendOrderStartEmail отправляет ссылку на /buy/pay.
func SendOrderStartEmail(cfg *config.Config, to, serviceName, amount, payURL string) error {
	subject := "VPN for Friends — ссылка на оплату"
	body := fmt.Sprintf(`VPN for Friends

Вы выбрали тариф: %s
Сумма к оплате: %s ₽

Перейти к оплате:
%s

Если вы не оформляли покупку VPN, просто проигнорируйте это письмо.
`, serviceName, amount, payURL)
	return sendPlain(cfg, strings.TrimSpace(to), subject, body)
}

// SendOrderStatusEmail отправляет ссылку на проверку оплаты.
func SendOrderStatusEmail(cfg *config.Config, to, serviceName, amount, statusURL string) error {
	subject := "VPN for Friends — проверка оплаты"
	body := fmt.Sprintf(`VPN for Friends

Ваш заказ создан.

Тариф: %s
Сумма: %s ₽

Ссылка для проверки оплаты:
%s

Если вы закрыли страницу оплаты, откройте эту ссылку повторно.
`, serviceName, amount, statusURL)
	return sendPlain(cfg, strings.TrimSpace(to), subject, body)
}
