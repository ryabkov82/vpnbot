package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
)

const accountTokenTypAccount = "account"

// AccountTokenClaims — magic-link личного кабинета.
type AccountTokenClaims struct {
	Typ    string `json:"typ"`
	Email  string `json:"email"`
	UserID int    `json:"user_id"`
	Login  string `json:"login"`
	Exp    int64  `json:"exp"`
}

var (
	ErrAccountTokenMalformed   = errors.New("malformed account token")
	ErrAccountTokenSignature   = errors.New("invalid account token signature")
	ErrAccountTokenExpired     = errors.New("account token expired")
	ErrAccountTokenType        = errors.New("invalid account token type")
	ErrAccountTokenEmptySecret = errors.New("account token secret is empty")
)

// CreateAccountToken — base64url(JSON).base64url(HMAC-SHA256(JSON, secret)).
func CreateAccountToken(secret string, email string, userID int, login string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrAccountTokenEmptySecret
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if userID <= 0 || strings.TrimSpace(email) == "" || strings.TrimSpace(login) == "" {
		return "", errors.New("invalid account token fields")
	}
	payload := AccountTokenClaims{
		Typ:    accountTokenTypAccount,
		Email:  email,
		UserID: userID,
		Login:  login,
		Exp:    time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := signOrderTokenPayload([]byte(secret), payloadJSON)
	encSig := base64.RawURLEncoding.EncodeToString(sig)
	return encPayload + "." + encSig, nil
}

// ParseAndVerifyAccountToken проверяет подпись и срок токена кабинета.
func ParseAndVerifyAccountToken(secret, token string) (*AccountTokenClaims, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrAccountTokenEmptySecret
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrAccountTokenMalformed
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return nil, ErrAccountTokenMalformed
	}
	encPayload := token[:dot]
	encSig := token[dot+1:]
	payloadJSON, err := base64.RawURLEncoding.DecodeString(encPayload)
	if err != nil {
		return nil, ErrAccountTokenMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return nil, ErrAccountTokenMalformed
	}
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write(payloadJSON)
	expected := m.Sum(nil)
	if len(sig) != len(expected) || !hmac.Equal(sig, expected) {
		return nil, ErrAccountTokenSignature
	}
	var claims AccountTokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrAccountTokenMalformed
	}
	if claims.Typ != accountTokenTypAccount {
		return nil, ErrAccountTokenType
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrAccountTokenExpired
	}
	if claims.UserID <= 0 || strings.TrimSpace(claims.Email) == "" || strings.TrimSpace(claims.Login) == "" {
		return nil, ErrAccountTokenMalformed
	}
	return &claims, nil
}

func accountTokenTTL(cfg *config.Config) time.Duration {
	return webSalesOrderTokenTTL(cfg)
}
