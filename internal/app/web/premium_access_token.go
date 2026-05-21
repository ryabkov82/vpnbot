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
// UserID — Telegram user id (как в боте: c.Chat().ID).
type PremiumAccessClaims struct {
	ServiceID int   `json:"service_id"`
	UserID    int64 `json:"user_id"`
	Exp       int64 `json:"exp"`
}

var (
	ErrPremiumTokenMalformed   = errors.New("malformed premium access token")
	ErrPremiumTokenSignature   = errors.New("invalid premium access token signature")
	ErrPremiumTokenExpired     = errors.New("premium access token expired")
	ErrPremiumTokenService     = errors.New("premium access token service mismatch")
	ErrPremiumTokenEmptySecret = errors.New("premium link signing secret is empty")
)

// CreatePremiumAccessToken возвращает base64url(JSON).base64url(HMAC-SHA256(JSON, secret)).
func CreatePremiumAccessToken(secret string, userID int64, serviceID int, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrPremiumTokenEmptySecret
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if serviceID <= 0 {
		return "", errors.New("serviceID must be positive")
	}
	payload := PremiumAccessClaims{
		ServiceID: serviceID,
		UserID:    userID,
		Exp:       time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := signPremiumPayload([]byte(secret), payloadJSON)
	encSig := base64.RawURLEncoding.EncodeToString(sig)
	return encPayload + "." + encSig, nil
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
	return &claims, nil
}

func signPremiumPayload(secret []byte, payloadJSON []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payloadJSON)
	return mac.Sum(nil)
}
