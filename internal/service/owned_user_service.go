package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

// ErrUserServiceUnavailable — услуга не существует, чужая, с другим ID или вне категории бренда.
// Причины намеренно не различаются.
var ErrUserServiceUnavailable = api.ErrUserServiceUnavailable

func parseOwnedUserServiceID(userServiceID string) (int, error) {
	s := strings.TrimSpace(userServiceID)
	if s == "" {
		return 0, fmt.Errorf("invalid service id")
	}
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid service id")
	}
	return id, nil
}

func (s *Service) expectedServiceCategory() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.brand.ServiceCategory)
}

// ownedUserServiceMatches — повторная локальная проверка ownership/category (defense-in-depth).
func ownedUserServiceMatches(us *models.UserService, userID, userServiceID int, expectedCategory string) bool {
	if us == nil {
		return false
	}
	if us.UserID != userID || us.ServiceID != userServiceID {
		return false
	}
	return models.ServiceCategoryAllowed(expectedCategory, us.Category)
}

// GetOwnedUserServiceByUserID возвращает user_service только если она принадлежит userID
// и категории активного бренда. Иначе ErrUserServiceUnavailable.
func (s *Service) GetOwnedUserServiceByUserID(userID int, userServiceID string) (*models.UserService, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	usID, err := parseOwnedUserServiceID(userServiceID)
	if err != nil {
		return nil, err
	}

	us, err := s.apiClient.GetUserServiceByUserID(userID, strconv.Itoa(usID))
	if err != nil {
		if errors.Is(err, api.ErrUserServiceUnavailable) {
			return nil, ErrUserServiceUnavailable
		}
		return nil, err
	}
	if !ownedUserServiceMatches(us, userID, usID, s.expectedServiceCategory()) {
		return nil, ErrUserServiceUnavailable
	}
	return us, nil
}

// GetOwnedUserServiceByTelegramID находит SHM-пользователя по Telegram chat id и возвращает
// принадлежащую ему услугу. Отсутствие пользователя → ErrUserNotFound.
func (s *Service) GetOwnedUserServiceByTelegramID(telegramChatID int64, userServiceID string) (*models.UserService, *models.User, error) {
	user, err := s.GetUser(telegramChatID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, ErrUserNotFound
	}
	us, err := s.GetOwnedUserServiceByUserID(user.ID, userServiceID)
	if err != nil {
		return nil, nil, err
	}
	return us, user, nil
}
