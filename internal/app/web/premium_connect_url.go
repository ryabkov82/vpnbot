package web

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
)

var ErrPremiumConnectNotConfigured = errors.New("premium connect not configured")

// PremiumConnectSignedLinkTTL совпадает с Telegram-потоком подключения через бота (24 часа).
const PremiumConnectSignedLinkTTL = 24 * time.Hour

func appendPremiumAccessQuery(rawBase string, userServiceInstanceID int, accessToken string) (string, error) {
	base := strings.TrimSpace(rawBase)
	if base == "" {
		return "", ErrPremiumConnectNotConfigured
	}
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("empty access token")
	}
	if userServiceInstanceID <= 0 {
		return "", errors.New("invalid user service instance id")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("service_id", strconv.Itoa(userServiceInstanceID))
	q.Set("access_token", accessToken)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// BuildPremiumConnectURLForTelegram — подписанная ссылка для Telegram (UserID в токене = chat id).
func BuildPremiumConnectURLForTelegram(cfg *config.Config, telegramChatID int64, userServiceInstanceID int) (string, error) {
	if cfg == nil {
		return "", ErrPremiumConnectNotConfigured
	}
	secret := strings.TrimSpace(cfg.PremiumLinkSigningSecret)
	if secret == "" {
		return "", ErrPremiumConnectNotConfigured
	}
	tok, err := CreatePremiumAccessToken(secret, telegramChatID, userServiceInstanceID, PremiumConnectSignedLinkTTL)
	if err != nil {
		return "", err
	}
	return appendPremiumAccessQuery(cfg.PremiumConnectBaseURL, userServiceInstanceID, tok)
}

// BuildPremiumConnectURLForWebAccount — подписанная ссылка из личного кабинета (shm user id в токене).
func BuildPremiumConnectURLForWebAccount(cfg *config.Config, shmUserID int, userServiceInstanceID int) (string, error) {
	if cfg == nil {
		return "", ErrPremiumConnectNotConfigured
	}
	secret := strings.TrimSpace(cfg.PremiumLinkSigningSecret)
	if secret == "" {
		return "", ErrPremiumConnectNotConfigured
	}
	tok, err := CreatePremiumSHMAccessToken(secret, shmUserID, userServiceInstanceID, PremiumConnectSignedLinkTTL)
	if err != nil {
		return "", err
	}
	return appendPremiumAccessQuery(cfg.PremiumConnectBaseURL, userServiceInstanceID, tok)
}
