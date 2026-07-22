package service

import (
	"errors"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// ErrActiveBrandIDRequired — web identity operations требуют явный brand.id процесса.
var ErrActiveBrandIDRequired = errors.New("active brand id is required")

// userHasCanonicalWebLogin — login или login2 совпадает с каноническим web login.
func userHasCanonicalWebLogin(user *models.User, canonicalWebLogin string) bool {
	if user == nil {
		return false
	}
	canonicalWebLogin = strings.TrimSpace(canonicalWebLogin)
	if canonicalWebLogin == "" {
		return false
	}
	return strings.TrimSpace(user.Login) == canonicalWebLogin ||
		strings.TrimSpace(user.Login2) == canonicalWebLogin
}

// webUserBelongsToBrand проверяет web membership активного бренда.
//
// Требует совпадение login или login2 с canonicalWebLogin, затем:
//   - VFF: brand_id == "vff" либо legacy (пустой brand_id);
//   - остальные бренды: только точное brand_id == activeBrandID (без VFF fallback).
//
// Явный чужой brand_id никогда не принадлежит, даже при совпавшем web login.
func webUserBelongsToBrand(user *models.User, activeBrandID, canonicalWebLogin string) bool {
	if !userHasCanonicalWebLogin(user, canonicalWebLogin) {
		return false
	}
	activeBrandID = strings.TrimSpace(activeBrandID)
	if activeBrandID == "" {
		return false
	}
	stored := strings.TrimSpace(user.Settings.BrandID)
	if activeBrandID == brandIDVFF {
		if stored == brandIDVFF {
			return true
		}
		return stored == ""
	}
	return stored == activeBrandID
}

func ensureWebUserMembership(user *models.User, activeBrandID, canonicalWebLogin string) error {
	if webUserBelongsToBrand(user, activeBrandID, canonicalWebLogin) {
		return nil
	}
	login := ""
	brandStored := ""
	if user != nil {
		login = user.Login
		brandStored = strings.TrimSpace(user.Settings.BrandID)
	}
	logIdentityMismatch("web_brand_id", login, brandStored, 0, 0)
	return ErrUserIdentityMismatch
}

// canonicalWebLoginFromEmail строит web login активного prefix для нормализованного email.
func canonicalWebLoginFromEmail(normalizedEmail, loginPrefix string) (string, error) {
	return webuser.WebLoginFromEmailWithPrefix(normalizedEmail, loginPrefix)
}

// webLoginConflictError классифицирует занятость web login/login2 при linking.
// same userID → nil; same brand other user → ErrWebEmailUsedByOtherAccount;
// other brand / mismatch → ErrUserIdentityMismatch.
func webLoginConflictError(found *models.User, selfUserID int, activeBrandID, canonicalWebLogin string) error {
	if found == nil || found.ID == selfUserID {
		return nil
	}
	if webUserBelongsToBrand(found, activeBrandID, canonicalWebLogin) {
		return ErrWebEmailUsedByOtherAccount
	}
	logIdentityMismatch("web_login_conflict_other_brand", found.Login, strings.TrimSpace(found.Settings.BrandID), 0, 0)
	return ErrUserIdentityMismatch
}
