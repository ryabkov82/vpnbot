package web

import (
	"strconv"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

// accountWebUserRegisteredTelegramNotifier вызывается при первой регистрации web-пользователя после подтверждения email (signup-token → RegisterUser).
// Подменяется в тестах.
var accountWebUserRegisteredTelegramNotifier = sendAccountWebUserRegisteredTelegramImpl

func sendAccountUserRegisteredTelegramNotification(cfg *config.Config, email string, userID int, login, ip string) {
	accountWebUserRegisteredTelegramNotifier(cfg, email, userID, login, ip)
}

func sendAccountWebUserRegisteredTelegramImpl(cfg *config.Config, email string, userID int, login, ip string) {
	var b strings.Builder
	b.WriteString("🆕 Web user registered\n\n")
	b.WriteString("Email: ")
	b.WriteString(strings.TrimSpace(email))
	b.WriteString("\nSHM user_id: ")
	b.WriteString(strconv.Itoa(userID))
	b.WriteString("\nLogin: ")
	b.WriteString(strings.TrimSpace(login))
	b.WriteString("\nIP: ")
	b.WriteString(strings.TrimSpace(ip))
	b.WriteString("\n\nПользователь подтвердил email и вошел в личный кабинет.")
	postTelegramPlainTextMessage(cfg, b.String(), "account web-user registered")
}
