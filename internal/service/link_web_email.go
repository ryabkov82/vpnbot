package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func mergeSettingsJSONToMap(existing json.RawMessage) (map[string]interface{}, error) {
	m := map[string]interface{}{}
	if len(existing) == 0 || string(existing) == "null" {
		return m, nil
	}
	if err := json.Unmarshal(existing, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return m, nil
}

// LinkWebEmailForTelegramUser записывает settings.web.* на существующего Telegram-пользователя SHM,
// выставляет login2=<prefix><hash(email)> без затирания остальных settings.
// Проверяет canonical Telegram login и brand membership активного процесса.
func (s *Service) LinkWebEmailForTelegramUser(userID int, telegramChatID int64, email string, source string) (*models.User, error) {
	normEmail, err := webuser.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	if telegramChatID <= 0 {
		return nil, errors.New("invalid telegram chat id")
	}
	if strings.TrimSpace(source) == "" {
		source = "telegram_link"
	}

	brandID := s.activeBrandID()
	if brandID == "" {
		return nil, ErrActiveBrandIDRequired
	}

	webLogin, err := webuser.WebLoginFromEmailWithPrefix(normEmail, s.webLoginPrefix())
	if err != nil {
		return nil, err
	}

	normKey := strings.ToLower(normEmail)

	uVerify, err := s.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if uVerify == nil {
		return nil, ErrUserNotFound
	}
	if uVerify.Settings.Telegram.ChatID != telegramChatID {
		return nil, ErrTelegramChatMismatch
	}

	canonicalTG := telegramSHMLogin(brandID, telegramChatID)
	if strings.TrimSpace(uVerify.Login) != canonicalTG {
		logIdentityMismatch("telegram_login", uVerify.Login, strings.TrimSpace(uVerify.Settings.BrandID), telegramChatID, uVerify.Settings.Telegram.ChatID)
		return nil, ErrUserIdentityMismatch
	}
	if !userBelongsToBrand(uVerify, brandID, canonicalTG) {
		logIdentityMismatch("brand_id", uVerify.Login, strings.TrimSpace(uVerify.Settings.BrandID), telegramChatID, uVerify.Settings.Telegram.ChatID)
		return nil, ErrUserIdentityMismatch
	}

	byLogin, err := s.apiClient.GetUserByLogin(webLogin)
	if err != nil {
		return nil, err
	}
	if err := webLoginConflictError(byLogin, userID, brandID, webLogin); err != nil {
		return nil, err
	}

	byLogin2, err := s.apiClient.GetUserByLogin2(webLogin)
	if err != nil {
		return nil, err
	}
	if err := webLoginConflictError(byLogin2, userID, brandID, webLogin); err != nil {
		return nil, err
	}

	loginSHM, rawSettings, err := s.apiClient.FetchAdminUserRowRaw(userID)
	if err != nil {
		return nil, err
	}
	if loginSHM == "" && (len(rawSettings) == 0 || string(rawSettings) == "null") {
		return nil, ErrUserNotFound
	}

	settingsObj, err := mergeSettingsJSONToMap(rawSettings)
	if err != nil {
		return nil, err
	}

	var webBlock map[string]interface{}
	switch w := settingsObj["web"].(type) {
	case map[string]interface{}:
		webBlock = w
	default:
		webBlock = map[string]interface{}{}
	}

	prevEmail := ""
	if v, ok := webBlock["email"].(string); ok {
		prevEmail = strings.TrimSpace(strings.ToLower(v))
	}

	if prevEmail == normKey {
		if byLogin2 != nil && byLogin2.ID == userID &&
			strings.TrimSpace(byLogin2.Login2) == strings.TrimSpace(webLogin) {
			return uVerify, nil
		}
	} else if prevEmail != "" {
		return nil, ErrWebEmailAlreadyLinked
	}

	webBlock["email"] = normEmail
	webBlock["source"] = source
	settingsObj["web"] = webBlock
	// Постепенный backfill brand_id (в т.ч. legacy VFF).
	settingsObj["brand_id"] = brandID

	updated, err := s.apiClient.PostAdminUserUpdateSettings(userID, webLogin, settingsObj)
	if err != nil {
		if errors.Is(err, api.ErrLogin2NotPersistedSHM) {
			return nil, ErrWebLogin2NotPersisted
		}
		return nil, err
	}
	if updated == nil {
		return nil, errors.New("link web email: shm update returned empty user")
	}
	if strings.TrimSpace(updated.Login) == "" {
		updated.Login = loginSHM
	}
	if strings.TrimSpace(updated.Settings.Web.Email) == "" {
		updated.Settings.Web.Email = normEmail
		updated.Settings.Web.Source = source
	}
	if strings.TrimSpace(updated.Settings.BrandID) == "" {
		updated.Settings.BrandID = brandID
	}
	return updated, nil
}
