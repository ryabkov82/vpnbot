package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	appService "github.com/ryabkov82/vpnbot/internal/service"
)

// authenticateWebAccount проверяет account token (включая brand) и повторно
// валидирует SHM-пользователя для активного бренда.
func authenticateWebAccount(cfg *config.Config, app accountWebApp, rawToken string) (*AccountTokenClaims, *models.User, error) {
	if cfg == nil || app == nil {
		return nil, nil, ErrAccountTokenMalformed
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, nil, ErrAccountTokenMalformed
	}
	secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
	claims, err := ParseAndVerifyAccountToken(secret, cfgBrandID(cfg), rawToken)
	if err != nil {
		return nil, nil, err
	}
	user, err := app.ValidateWebAccountUser(claims.UserID, claims.Login, claims.Email)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, appService.ErrUserNotFound
	}
	return claims, user, nil
}

func writeAccountAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appService.ErrUserIdentityMismatch),
		errors.Is(err, ErrAccountTokenBrand),
		errors.Is(err, ErrAccountTokenSignature),
		errors.Is(err, ErrAccountTokenExpired),
		errors.Is(err, ErrAccountTokenType),
		errors.Is(err, ErrAccountTokenMalformed),
		errors.Is(err, ErrAccountTokenEmptySecret),
		errors.Is(err, appService.ErrUserNotFound):
		writeJSONError(w, http.StatusUnauthorized, "invalid_token")
	default:
		slog.Error("account auth", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error")
	}
}
