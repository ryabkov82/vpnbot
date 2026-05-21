package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
)

const (
	orderTokenTypStart = "start"
	orderTokenTypOrder = "order"
)

// OrderStartClaims — полезная нагрузка start-токена (до SHM-заказа).
type OrderStartClaims struct {
	Typ       string `json:"typ"`
	Email     string `json:"email"`
	ServiceID int    `json:"service_id"`
	Nonce     string `json:"nonce"`
	Exp       int64  `json:"exp"`
}

// OrderTokenClaims — полезная нагрузка после создания заказа в SHM.
type OrderTokenClaims struct {
	Typ           string  `json:"typ"`
	Email         string  `json:"email"`
	ServiceID     int     `json:"service_id"`
	UserID        int     `json:"user_id"`
	UserServiceID int     `json:"user_service_id"`
	Amount        float64 `json:"amount"`
	Exp           int64   `json:"exp"`
}

var (
	ErrOrderTokenMalformed   = errors.New("malformed order token")
	ErrOrderTokenSignature   = errors.New("invalid order token signature")
	ErrOrderTokenExpired     = errors.New("order token expired")
	ErrOrderTokenType        = errors.New("invalid order token type")
	ErrOrderTokenEmptySecret = errors.New("order token secret is empty")
)

func webSalesOrderTokenTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.WebSales.OrderTokenTTLHours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(cfg.WebSales.OrderTokenTTLHours) * time.Hour
}

func signOrderTokenPayload(secret []byte, payloadJSON []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payloadJSON)
	return mac.Sum(nil)
}

func randomOrderNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateOrderStartToken возвращает base64url(JSON).base64url(HMAC-SHA256(JSON, secret)).
func CreateOrderStartToken(secret, email string, serviceID int, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrOrderTokenEmptySecret
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if serviceID <= 0 {
		return "", errors.New("service_id must be positive")
	}
	nonce, err := randomOrderNonce()
	if err != nil {
		return "", err
	}
	payload := OrderStartClaims{
		Typ:       orderTokenTypStart,
		Email:     email,
		ServiceID: serviceID,
		Nonce:     nonce,
		Exp:       time.Now().Add(ttl).Unix(),
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

// ParseAndVerifyOrderStartToken проверяет подпись и срок start-токена.
func ParseAndVerifyOrderStartToken(secret, token string) (*OrderStartClaims, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrOrderTokenEmptySecret
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrOrderTokenMalformed
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return nil, ErrOrderTokenMalformed
	}
	encPayload := token[:dot]
	encSig := token[dot+1:]
	payloadJSON, err := base64.RawURLEncoding.DecodeString(encPayload)
	if err != nil {
		return nil, ErrOrderTokenMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return nil, ErrOrderTokenMalformed
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payloadJSON)
	expected := mac.Sum(nil)
	if len(sig) != len(expected) || !hmac.Equal(sig, expected) {
		return nil, ErrOrderTokenSignature
	}
	var claims OrderStartClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrOrderTokenMalformed
	}
	if claims.Typ != orderTokenTypStart {
		return nil, ErrOrderTokenType
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrOrderTokenExpired
	}
	if claims.ServiceID <= 0 || strings.TrimSpace(claims.Email) == "" || strings.TrimSpace(claims.Nonce) == "" {
		return nil, ErrOrderTokenMalformed
	}
	return &claims, nil
}

// CreateOrderToken подписывает токен после создания заказа.
func CreateOrderToken(secret string, email string, serviceID, userID, userServiceID int, amount float64, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrOrderTokenEmptySecret
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if serviceID <= 0 || userID <= 0 || userServiceID <= 0 || amount <= 0 {
		return "", errors.New("invalid order token fields")
	}
	payload := OrderTokenClaims{
		Typ:           orderTokenTypOrder,
		Email:         email,
		ServiceID:     serviceID,
		UserID:        userID,
		UserServiceID: userServiceID,
		Amount:        amount,
		Exp:           time.Now().Add(ttl).Unix(),
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

// ParseAndVerifyOrderToken проверяет подпись и срок order-токена.
func ParseAndVerifyOrderToken(secret, token string) (*OrderTokenClaims, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrOrderTokenEmptySecret
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrOrderTokenMalformed
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return nil, ErrOrderTokenMalformed
	}
	encPayload := token[:dot]
	encSig := token[dot+1:]
	payloadJSON, err := base64.RawURLEncoding.DecodeString(encPayload)
	if err != nil {
		return nil, ErrOrderTokenMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return nil, ErrOrderTokenMalformed
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payloadJSON)
	expected := mac.Sum(nil)
	if len(sig) != len(expected) || !hmac.Equal(sig, expected) {
		return nil, ErrOrderTokenSignature
	}
	var claims OrderTokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrOrderTokenMalformed
	}
	if claims.Typ != orderTokenTypOrder {
		return nil, ErrOrderTokenType
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrOrderTokenExpired
	}
	if claims.ServiceID <= 0 || claims.UserID <= 0 || claims.UserServiceID <= 0 || claims.Amount <= 0 {
		return nil, ErrOrderTokenMalformed
	}
	return &claims, nil
}
