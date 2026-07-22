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

const legacyBrandDisplayName = "VPN for Friends"

// IsConfigured — включённый SMTP с минимально необходимыми полями (magic-link вход в кабинет и др.).
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

// brandDisplayName — пользовательское название бренда для писем.
// Пустой/nil config → legacy "VPN for Friends". Удаляет CR/LF перед заголовками.
func brandDisplayName(cfg *config.Config) string {
	name := ""
	if cfg != nil {
		name = strings.TrimSpace(cfg.EffectiveBrand().Name)
	}
	if name == "" {
		name = legacyBrandDisplayName
	}
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	return name
}

func formatFrom(cfg *config.Config) string {
	name := ""
	if cfg != nil {
		name = strings.TrimSpace(cfg.Email.FromName)
	}
	if name == "" {
		name = brandDisplayName(cfg)
	}
	from := ""
	if cfg != nil {
		from = strings.TrimSpace(cfg.Email.FromEmail)
	}
	if strings.ContainsAny(name, "\r\n\"") {
		name = strings.ReplaceAll(name, "\"", "'")
		name = strings.ReplaceAll(name, "\r", "")
		name = strings.ReplaceAll(name, "\n", "")
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

// SendAccountLoginEmail — magic-link вход в личный кабинет.
func SendAccountLoginEmail(cfg *config.Config, to, loginURL string) error {
	brand := brandDisplayName(cfg)
	subject := brand + " — вход в личный кабинет"
	body := fmt.Sprintf(`%s

Для входа в личный кабинет откройте ссылку:
%s

Если вы не запрашивали вход, просто проигнорируйте это письмо.
`, brand, strings.TrimSpace(loginURL))
	return sendPlain(cfg, strings.TrimSpace(to), subject, body)
}

// SendAccountLinkConfirmEmail — письмо для завершения привязки Telegram → web после ввода email на /account/link.
func SendAccountLinkConfirmEmail(cfg *config.Config, to, confirmURL string) error {
	brand := brandDisplayName(cfg)
	subject := brand + " — подтвердите привязку личного кабинета"
	body := fmt.Sprintf(`%s

Подтвердите email, чтобы связать ваш web-личный кабинет с текущим аккаунтом в Telegram и видеть ваши услуги и баланс на сайте.

Откройте ссылку:
%s

Если вы не запрашивали привязку, проигнорируйте письмо.
`, brand, strings.TrimSpace(confirmURL))
	return sendPlain(cfg, strings.TrimSpace(to), subject, body)
}
