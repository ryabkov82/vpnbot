package web

import (
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestTelegramLinkFieldsFromUser_Username(t *testing.T) {
	linked, uname, chatID := telegramLinkFieldsFromUser(&models.User{
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{
				Username: "  @friend_user  ",
				ChatID:   4242,
			},
		},
	})
	if !linked || uname != "@friend_user" || chatID != 0 {
		t.Fatalf("got linked=%v uname=%q chatID=%d", linked, uname, chatID)
	}
}

func TestTelegramLinkFieldsFromUser_LoginFallback(t *testing.T) {
	linked, uname, chatID := telegramLinkFieldsFromUser(&models.User{
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{
				Login:  "@telegram27",
				ChatID: 9001,
			},
		},
	})
	if !linked || uname != "@telegram27" || chatID != 0 {
		t.Fatalf("got linked=%v uname=%q chatID=%d", linked, uname, chatID)
	}
}

func TestTelegramLinkFieldsFromUser_ChatIDOnly(t *testing.T) {
	linked, uname, chatID := telegramLinkFieldsFromUser(&models.User{
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{ChatID: 707070},
		},
	})
	if !linked || uname != "" || chatID != 707070 {
		t.Fatalf("got linked=%v uname=%q chatID=%d", linked, uname, chatID)
	}
}

func TestTelegramLinkFieldsFromUser_NotLinked(t *testing.T) {
	linked, uname, chatID := telegramLinkFieldsFromUser(&models.User{
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{},
			Web:      models.WebInfo{Email: "solo@example.com"},
		},
	})
	if linked || uname != "" || chatID != 0 {
		t.Fatalf("got linked=%v uname=%q chatID=%d", linked, uname, chatID)
	}
}
