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

func findOrCreateWebUser(reg webUserRegistrar, email string) (*models.User, error) {
	normalizedEmail, err := webuser.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}

	login := webuser.WebLoginFromEmail(normalizedEmail)

	u, err := reg.GetUserByLogin(login)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return u, nil
	}

	password, err := randomWebUserPassword()
	if err != nil {
		return nil, err
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
		return nil, err
	}

	u, err = reg.GetUserByLogin(login)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, fmt.Errorf("web user not found after registration (login=%s)", login)
	}
	return u, nil
}

func randomWebUserPassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// FindOrCreateWebUser находит SHM-пользователя по web-login из email или создаёт нового.
func (s *Service) FindOrCreateWebUser(email string) (*models.User, error) {
	return findOrCreateWebUser(s.apiClient, email)
}
