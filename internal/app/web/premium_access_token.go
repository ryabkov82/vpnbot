package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// PremiumAccessClaims — полезная нагрузка подписанного токена доступа к premium onboarding.
// Либо UserID — Telegram chat id (бот), либо ShmUserID — shm user id (личный кабинет).
type PremiumAccessClaims struct {
	ServiceID int   `json:"service_id"`
	UserID    int64 `json:"user_id,omitempty"`
	ShmUserID int   `json:"shm_user_id,omitempty"`
	Exp       int64 `json:"exp"`
}

var (
	ErrPremiumTokenMalformed   = errors.New("malformed premium access token")
	ErrPremiumTokenSignature   = errors.New("invalid premium access token signature")
	ErrPremiumTokenExpired     = errors.New("premium access token expired")
	ErrPremiumTokenService     = errors.New("premium access token service mismatch")
	ErrPremiumTokenEmptySecret = errors.New("premium link signing secret is empty")
)

func marshalPremiumSignedToken(secret string, claims PremiumAccessClaims) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrPremiumTokenEmptySecret
	}
	if claims.ServiceID <= 0 {
		return "", errors.New("serviceID must be positive")
	}
	hasTG := claims.UserID != 0
	hasSHM := claims.ShmUserID != 0
	if hasTG == hasSHM {
		return "", errors.New("exactly one of telegram user_id or shm_user_id must be set")
	}
	if claims.Exp <= 0 {
		return "", errors.New("exp required")
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := signPremiumPayload([]byte(secret), payloadJSON)
	encSig := base64.RawURLEncoding.EncodeToString(sig)
	return encPayload + "." + encSig, nil
}

// CreatePremiumAccessToken возвращает base64url(JSON).base64url(HMAC-SHA256(JSON, secret)).
// userID — Telegram chat id (как в боте).
func CreatePremiumAccessToken(secret string, userID int64, serviceID int, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if userID == 0 {
		return "", errors.New("telegram user id required")
	}
	return marshalPremiumSignedToken(secret, PremiumAccessClaims{
		ServiceID: serviceID,
		UserID:    userID,
		Exp:       time.Now().Add(ttl).Unix(),
	})
}

// CreatePremiumSHMAccessToken — токен для открытия premium-connect из веб-кабинета (shm user id).
func CreatePremiumSHMAccessToken(secret string, shmUserID int, serviceID int, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if shmUserID <= 0 {
		return "", errors.New("shm user id required")
	}
	return marshalPremiumSignedToken(secret, PremiumAccessClaims{
		ServiceID: serviceID,
		ShmUserID: shmUserID,
		Exp:       time.Now().Add(ttl).Unix(),
	})
}

// ValidatePremiumAccessToken проверяет подпись, срок и совпадение service_id с query.
func ValidatePremiumAccessToken(secret string, token string, serviceID int) (*PremiumAccessClaims, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrPremiumTokenEmptySecret
	}
	if serviceID <= 0 {
		return nil, errors.New("invalid service id")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrPremiumTokenMalformed
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return nil, ErrPremiumTokenMalformed
	}
	encPayload := token[:dot]
	encSig := token[dot+1:]
	payloadJSON, err := base64.RawURLEncoding.DecodeString(encPayload)
	if err != nil {
		return nil, ErrPremiumTokenMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return nil, ErrPremiumTokenMalformed
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payloadJSON)
	expected := mac.Sum(nil)
	if len(sig) != len(expected) || !hmac.Equal(sig, expected) {
		return nil, ErrPremiumTokenSignature
	}
	var claims PremiumAccessClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrPremiumTokenMalformed
	}
	if claims.ServiceID != serviceID {
		return nil, ErrPremiumTokenService
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrPremiumTokenExpired
	}
	hasTG := claims.UserID != 0
	hasSHM := claims.ShmUserID != 0
	if hasTG == hasSHM {
		return nil, ErrPremiumTokenMalformed
	}
	return &claims, nil
}

func signPremiumPayload(secret []byte, payloadJSON []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payloadJSON)
	return mac.Sum(nil)
}
