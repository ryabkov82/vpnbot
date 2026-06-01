package web

import (
	"strings"

	"github.com/ryabkov82/vpnbot/internal/models"
)

// telegramLinkFieldsFromUser extracts safe Telegram link fields for the account dashboard API.
func telegramLinkFieldsFromUser(u *models.User) (linked bool, username string, chatID int64) {
	if u == nil {
		return false, "", 0
	}
	tg := u.Settings.Telegram
	if uname := normalizeTelegramUsernameForDisplay(tg.Username, tg.Login); uname != "" {
		return true, uname, 0
	}
	if tg.ChatID > 0 {
		return true, "", tg.ChatID
	}
	return false, "", 0
}

func normalizeTelegramUsernameForDisplay(fields ...string) string {
	for _, raw := range fields {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		s = strings.TrimPrefix(s, "@")
		s = strings.TrimSpace(s)
		if s != "" {
			return "@" + s
		}
	}
	return ""
}
