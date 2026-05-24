package service

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

const webUserSource = "vpn-for-friends.com"

type webUserRegistrar interface {
	GetUserByLogin(login string) (*models.User, error)
	RegisterUser(user models.UserRegistrationRequest) error
}

func findOrCreateWebUser(reg webUserRegistrar, email string) (*models.User, bool, error) {
	normalizedEmail, err := webuser.NormalizeEmail(email)
	if err != nil {
		return nil, false, err
	}

	login := webuser.WebLoginFromEmail(normalizedEmail)

	u, err := reg.GetUserByLogin(login)
	if err != nil {
		return nil, false, err
	}
	if u != nil {
		return u, false, nil
	}

	password, err := randomWebUserPassword()
	if err != nil {
		return nil, false, err
	}

	regReq := models.UserRegistrationRequest{
		Login:    login,
		Password: password,
		FullName: normalizedEmail,
		Settings: models.UserSettings{
			Web: models.WebInfo{
				Email:  normalizedEmail,
				Source: webUserSource,
			},
		},
	}

	if err := reg.RegisterUser(regReq); err != nil {
		return nil, false, err
	}

	u, err = reg.GetUserByLogin(login)
	if err != nil {
		return nil, false, err
	}
	if u == nil {
		return nil, false, fmt.Errorf("web user not found after registration (login=%s)", login)
	}
	return u, true, nil
}

func randomWebUserPassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// FindOrCreateWebUser находит SHM-пользователя по web-login из email или регистрирует нового.
// Второй результат — RegisterUser действительно вызывался в этом запросе.
func (s *Service) FindOrCreateWebUser(email string) (*models.User, bool, error) {
	return findOrCreateWebUser(s.apiClient, email)
}
