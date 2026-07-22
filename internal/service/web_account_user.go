package service

import (
	"errors"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// ValidateWebAccountUser повторно проверяет SHM-пользователя для account token claims
// активного бренда. Не заменяет GetUserByID: предназначен только для web account flow.
func (s *Service) ValidateWebAccountUser(userID int, tokenLogin, tokenEmail string) (*models.User, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	brandID := s.activeBrandID()
	if brandID == "" {
		return nil, ErrActiveBrandIDRequired
	}
	tokenLogin = strings.TrimSpace(tokenLogin)
	if tokenLogin == "" {
		return nil, ErrUserIdentityMismatch
	}
	normEmail, err := webuser.NormalizeEmail(tokenEmail)
	if err != nil {
		return nil, err
	}

	u, err := s.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	if strings.TrimSpace(u.Login) != tokenLogin {
		logIdentityMismatch("token_login", u.Login, strings.TrimSpace(u.Settings.BrandID), 0, 0)
		return nil, ErrUserIdentityMismatch
	}

	canonical, err := webuser.WebLoginFromEmailWithPrefix(normEmail, s.webLoginPrefix())
	if err != nil {
		return nil, err
	}
	if err := ensureWebUserMembership(u, brandID, canonical); err != nil {
		return nil, err
	}

	storedEmail := strings.TrimSpace(u.Settings.Web.Email)
	if storedEmail != "" {
		storedNorm, nerr := webuser.NormalizeEmail(storedEmail)
		if nerr != nil || storedNorm != normEmail {
			logIdentityMismatch("web_email", u.Login, strings.TrimSpace(u.Settings.BrandID), 0, 0)
			return nil, ErrUserIdentityMismatch
		}
	}
	return u, nil
}
