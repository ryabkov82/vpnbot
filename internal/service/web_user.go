package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/registrationevent"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// ErrWebUserSourceRequired — пустой web source недопустим при регистрации web-user.
var ErrWebUserSourceRequired = errors.New("web user source is required")

type webUserRegistrar interface {
	GetUserByLogin(login string) (*models.User, error)
	GetUserByLogin2(login2 string) (*models.User, error)
	RegisterUser(user models.UserRegistrationRequest) error
}

func findUserByWebLoginKeys(reg webUserRegistrar, normalizedEmail, loginPrefix, brandID string) (*models.User, error) {
	webLogin, err := webuser.WebLoginFromEmailWithPrefix(normalizedEmail, loginPrefix)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(brandID) == "" {
		return nil, ErrActiveBrandIDRequired
	}

	u, err := reg.GetUserByLogin(webLogin)
	if err != nil {
		return nil, err
	}
	if u != nil {
		if err := ensureWebUserMembership(u, brandID, webLogin); err != nil {
			return nil, err
		}
		return u, nil
	}

	u, err = reg.GetUserByLogin2(webLogin)
	if err != nil {
		return nil, err
	}
	if u != nil {
		if err := ensureWebUserMembership(u, brandID, webLogin); err != nil {
			return nil, err
		}
		return u, nil
	}
	return nil, nil
}

func findOrCreateWebUser(reg webUserRegistrar, email, loginPrefix, webSource, brandID string) (*models.User, bool, error) {
	return findOrCreateWebUserCore(reg, email, loginPrefix, webSource, brandID, nil)
}

func findOrCreateWebUserWithAttribution(
	reg webUserRegistrar,
	email, loginPrefix, webSource, brandID string,
	record attribution.Record,
) (*models.User, bool, error) {
	if !record.Valid() {
		return nil, false, ErrAttributionRequired
	}
	return findOrCreateWebUserCore(reg, email, loginPrefix, webSource, brandID, &record)
}

func findOrCreateWebUserCore(
	reg webUserRegistrar,
	email, loginPrefix, webSource, brandID string,
	record *attribution.Record,
) (*models.User, bool, error) {
	normalizedEmail, err := webuser.NormalizeEmail(email)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(loginPrefix) == "" {
		return nil, false, webuser.ErrWebLoginPrefixRequired
	}
	if strings.TrimSpace(webSource) == "" {
		return nil, false, ErrWebUserSourceRequired
	}
	brandID = strings.TrimSpace(brandID)
	if brandID == "" {
		return nil, false, ErrActiveBrandIDRequired
	}

	uKnown, err := findUserByWebLoginKeys(reg, normalizedEmail, loginPrefix, brandID)
	if err != nil {
		return nil, false, err
	}
	if uKnown != nil {
		return uKnown, false, nil
	}

	login, err := webuser.WebLoginFromEmailWithPrefix(normalizedEmail, loginPrefix)
	if err != nil {
		return nil, false, err
	}

	password, err := randomWebUserPassword()
	if err != nil {
		return nil, false, err
	}

	settings := models.UserSettings{
		BrandID: brandID,
		Web: models.WebInfo{
			Email:  normalizedEmail,
			Source: webSource,
		},
	}
	if record != nil {
		cp := *record
		settings.Attribution = &cp
	}

	regReq := models.UserRegistrationRequest{
		Login:    login,
		Password: password,
		FullName: normalizedEmail,
		Settings: settings,
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
	if record != nil {
		if err := registrationevent.Emit(slog.Default(), brandID, u.ID, *record); err != nil {
			slog.Warn("registration event skipped",
				"brand_id", brandID,
				"registration_channel", string(record.FirstTouch.RegistrationChannel),
				"reason", "emit_failed",
			)
		}
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

// FindOrCreateWebUser находит SHM-пользователя по login/login2 = <prefix><hash(email)>
// с проверкой brand membership; иначе регистрирует web-only пользователя с settings.brand_id.
// Второй результат — RegisterUser действительно вызывался в этом запросе.
//
// При записи другого бренда на том же web login возвращает ErrUserIdentityMismatch
// (не not found): новый user не создаётся.
//
// Без attribution (legacy Google OAuth path until a later M8 commit).
func (s *Service) FindOrCreateWebUser(email string) (*models.User, bool, error) {
	return findOrCreateWebUser(s.apiClient, email, s.webLoginPrefix(), s.webUserSource(), s.activeBrandID())
}

// FindOrCreateWebUserWithAttribution is the magic-link signup path: new users get
// settings.attribution from the signed signup-token record. Existing users are
// returned unchanged (created=false); attribution is never updated or backfilled.
func (s *Service) FindOrCreateWebUserWithAttribution(email string, record attribution.Record) (*models.User, bool, error) {
	return findOrCreateWebUserWithAttribution(
		s.apiClient,
		email,
		s.webLoginPrefix(),
		s.webUserSource(),
		s.activeBrandID(),
		record,
	)
}

// FindUserByWebEmail находит shm user только по связке login/login2 = <prefix><hash(email)>
// активного бренда (без фильтров по nested settings.web — SHM на них даёт ISE).
// Чужой brand → ErrUserIdentityMismatch.
func (s *Service) FindUserByWebEmail(email string) (*models.User, error) {
	normEmail, err := webuser.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return findUserByWebLoginKeys(s.apiClient, normEmail, s.webLoginPrefix(), s.activeBrandID())
}
