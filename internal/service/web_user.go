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
	GetUserByLogin2(login2 string) (*models.User, error)
	RegisterUser(user models.UserRegistrationRequest) error
}

func findUserByWebLoginKeys(reg webUserRegistrar, normalizedEmail string) (*models.User, error) {
	webLogin := webuser.WebLoginFromEmail(normalizedEmail)
	u, err := reg.GetUserByLogin(webLogin)
	if err != nil || u != nil {
		return u, err
	}
	return reg.GetUserByLogin2(webLogin)
}

func findOrCreateWebUser(reg webUserRegistrar, email string) (*models.User, bool, error) {
	normalizedEmail, err := webuser.NormalizeEmail(email)
	if err != nil {
		return nil, false, err
	}

	uKnown, err := findUserByWebLoginKeys(reg, normalizedEmail)
	if err != nil {
		return nil, false, err
	}
	if uKnown != nil {
		return uKnown, false, nil
	}

	login := webuser.WebLoginFromEmail(normalizedEmail)

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

	u, err := reg.GetUserByLogin(login)
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

// FindOrCreateWebUser находит SHM-пользователя по login (=web_<hash>) или по login2 (=web_<hash> у telegram-профиля), иначе регистрирует web-only пользователя.
// Второй результат — RegisterUser действительно вызывался в этом запросе.
func (s *Service) FindOrCreateWebUser(email string) (*models.User, bool, error) {
	return findOrCreateWebUser(s.apiClient, email)
}

// FindUserByWebEmail находит shm user только по связке login/login2 = web_<hash(email)> (без фильтров по nested settings.web — SHM на них даёт ISE).
func (s *Service) FindUserByWebEmail(email string) (*models.User, error) {
	normEmail, err := webuser.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return findUserByWebLoginKeys(s.apiClient, normEmail)
}
