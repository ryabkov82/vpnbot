package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/models"
)

const brandIDVFF = "vff"

// ErrUserIdentityMismatch — запись по каноническому login найдена, но telegram.chat_id
// или settings.brand_id не соответствуют активному бренду/запросу.
// Нельзя трактовать как обычный not found: иначе последующая регистрация упрётся в занятый login.
var ErrUserIdentityMismatch = errors.New("user identity mismatch")

// telegramSHMLogin — канонический SHM login для Telegram-пользователя активного бренда.
// vff → @<chatID>; любой другой brand ID → @<brandID>_<chatID>.
func telegramSHMLogin(brandID string, chatID int64) string {
	brandID = strings.TrimSpace(brandID)
	if brandID == "" || brandID == brandIDVFF {
		return fmt.Sprintf("@%d", chatID)
	}
	return fmt.Sprintf("@%s_%d", brandID, chatID)
}

func (s *Service) activeBrandID() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.brand.ID)
}

// userBelongsToBrand проверяет принадлежность найденной SHM-записи активному бренду.
// VFF: brand_id == "vff" или legacy (пустой brand_id + login == @<chatID>).
// Остальные бренды: только точное совпадение settings.brand_id.
func userBelongsToBrand(user *models.User, activeBrandID, canonicalLogin string) bool {
	if user == nil {
		return false
	}
	activeBrandID = strings.TrimSpace(activeBrandID)
	stored := strings.TrimSpace(user.Settings.BrandID)
	if activeBrandID == brandIDVFF {
		if stored == brandIDVFF {
			return true
		}
		return stored == "" && user.Login == canonicalLogin
	}
	return stored == activeBrandID
}

func logIdentityMismatch(reason, login, brandID string, wantChatID, gotChatID int64) {
	slog.Warn("shm user identity mismatch",
		"reason", reason,
		"login", login,
		"brand_id", brandID,
		"want_chat_id", wantChatID,
		"got_chat_id", gotChatID,
	)
}
