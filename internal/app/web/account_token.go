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

const (
	accountTokenTypAccount      = "account"
	accountTokenTypSignup       = "account_signup"
	accountTokenTypTelegramLink = "account_telegram_link"
	accountTokenTypLinkEmail    = "account_link_email"
)

// AccountTokenClaims — magic-link личного кабинета.
type AccountTokenClaims struct {
	Typ     string `json:"typ"`
	BrandID string `json:"brand_id"`
	Email   string `json:"email"`
	UserID  int    `json:"user_id"`
	Login   string `json:"login"`
	Exp     int64  `json:"exp"`
}

// AccountSignupTokenClaims — одноразовый magic-link до создания shm user (нет user_id).
type AccountSignupTokenClaims struct {
	Typ     string `json:"typ"`
	BrandID string `json:"brand_id"`
	Email   string `json:"email"`
	Login   string `json:"login"`
	Exp     int64  `json:"exp"`
}

// AccountTelegramLinkClaims — короткий токен из бота до привязки web-email.
type AccountTelegramLinkClaims struct {
	Typ            string `json:"typ"`
	BrandID        string `json:"brand_id"`
	ShmUserID      int    `json:"shm_user_id"`
	TelegramChatID int64  `json:"telegram_chat_id"`
	Exp            int64  `json:"exp"`
}

// AccountLinkEmailClaims — переход из письма после ввода email на странице /account/link.
type AccountLinkEmailClaims struct {
	Typ            string `json:"typ"`
	BrandID        string `json:"brand_id"`
	ShmUserID      int    `json:"shm_user_id"`
	TelegramChatID int64  `json:"telegram_chat_id"`
	Email          string `json:"email"`
	Exp            int64  `json:"exp"`
}

var (
	ErrAccountTokenMalformed   = errors.New("malformed account token")
	ErrAccountTokenSignature   = errors.New("invalid account token signature")
	ErrAccountTokenExpired     = errors.New("account token expired")
	ErrAccountTokenType        = errors.New("invalid account token type")
	ErrAccountTokenEmptySecret = errors.New("account token secret is empty")
	ErrAccountTokenBrand       = errors.New("invalid account token brand")
)

func requireAccountTokenBrandID(brandID string) (string, error) {
	brandID = strings.TrimSpace(brandID)
	if brandID == "" {
		return "", ErrAccountTokenBrand
	}
	return brandID, nil
}

func matchAccountTokenBrand(got, expected string) error {
	expected = strings.TrimSpace(expected)
	got = strings.TrimSpace(got)
	if expected == "" || got == "" || got != expected {
		return ErrAccountTokenBrand
	}
	return nil
}

func cfgBrandID(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.EffectiveBrand().ID)
}

func verifyAccountMagicTokenPayload(secret, token string) ([]byte, error) {
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
	return payloadJSON, nil
}

func signAndEncodeAccountPayload(secret string, payloadJSON []byte) (string, error) {
	encPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := signOrderTokenPayload([]byte(secret), payloadJSON)
	encSig := base64.RawURLEncoding.EncodeToString(sig)
	return encPayload + "." + encSig, nil
}

// CreateAccountToken — base64url(JSON).base64url(HMAC-SHA256(JSON, secret)).
func CreateAccountToken(secret, brandID, email string, userID int, login string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrAccountTokenEmptySecret
	}
	brandID, err := requireAccountTokenBrandID(brandID)
	if err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if userID <= 0 || strings.TrimSpace(email) == "" || strings.TrimSpace(login) == "" {
		return "", errors.New("invalid account token fields")
	}
	payload := AccountTokenClaims{
		Typ:     accountTokenTypAccount,
		BrandID: brandID,
		Email:   email,
		UserID:  userID,
		Login:   login,
		Exp:     time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return signAndEncodeAccountPayload(secret, payloadJSON)
}

// CreateAccountSignupToken — onboarding magic-link перед созданием web user в SHM.
func CreateAccountSignupToken(secret, brandID, email, login string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrAccountTokenEmptySecret
	}
	brandID, err := requireAccountTokenBrandID(brandID)
	if err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	if strings.TrimSpace(email) == "" || strings.TrimSpace(login) == "" {
		return "", errors.New("invalid signup token fields")
	}
	payload := AccountSignupTokenClaims{
		Typ:     accountTokenTypSignup,
		BrandID: brandID,
		Email:   strings.TrimSpace(email),
		Login:   strings.TrimSpace(login),
		Exp:     time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return signAndEncodeAccountPayload(secret, payloadJSON)
}

func accountTelegramLinkTTL(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.WebSales.TelegramLinkTokenTTLMinutes > 0 {
		return time.Duration(cfg.WebSales.TelegramLinkTokenTTLMinutes) * time.Minute
	}
	return 30 * time.Minute
}

func accountLinkEmailMagicTTL(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.WebSales.LinkConfirmEmailTTLMinutes > 0 {
		return time.Duration(cfg.WebSales.LinkConfirmEmailTTLMinutes) * time.Minute
	}
	return 60 * time.Minute
}

// CreateAccountTelegramLinkToken — короткая ссылка из Telegram («Личный кабинет»).
func CreateAccountTelegramLinkToken(secret, brandID string, userID int, chatID int64, cfg *config.Config) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrAccountTokenEmptySecret
	}
	brandID, err := requireAccountTokenBrandID(brandID)
	if err != nil {
		return "", err
	}
	if userID <= 0 || chatID <= 0 {
		return "", errors.New("invalid telegram link payload")
	}
	ttl := accountTelegramLinkTTL(cfg)
	payload := AccountTelegramLinkClaims{
		Typ:            accountTokenTypTelegramLink,
		BrandID:        brandID,
		ShmUserID:      userID,
		TelegramChatID: chatID,
		Exp:            time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return signAndEncodeAccountPayload(secret, payloadJSON)
}

// VerifyAccountTelegramLinkToken проверяет токен привязки из бота.
func VerifyAccountTelegramLinkToken(secret, expectedBrandID, token string) (*AccountTelegramLinkClaims, error) {
	payloadJSON, err := verifyAccountMagicTokenPayload(secret, token)
	if err != nil {
		return nil, err
	}
	var claims AccountTelegramLinkClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrAccountTokenMalformed
	}
	if claims.Typ != accountTokenTypTelegramLink {
		return nil, ErrAccountTokenType
	}
	if err := matchAccountTokenBrand(claims.BrandID, expectedBrandID); err != nil {
		return nil, err
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrAccountTokenExpired
	}
	if claims.ShmUserID <= 0 || claims.TelegramChatID <= 0 {
		return nil, ErrAccountTokenMalformed
	}
	return &claims, nil
}

// CreateAccountLinkEmailToken — продолжение flow после запроса письма с /account/link.
func CreateAccountLinkEmailToken(secret, brandID string, shmUserID int, chatID int64, normEmail string, cfg *config.Config) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrAccountTokenEmptySecret
	}
	brandID, err := requireAccountTokenBrandID(brandID)
	if err != nil {
		return "", err
	}
	normEmail = strings.TrimSpace(normEmail)
	if shmUserID <= 0 || chatID <= 0 || normEmail == "" {
		return "", errors.New("invalid link-email token fields")
	}
	ttl := accountLinkEmailMagicTTL(cfg)
	payload := AccountLinkEmailClaims{
		Typ:            accountTokenTypLinkEmail,
		BrandID:        brandID,
		ShmUserID:      shmUserID,
		TelegramChatID: chatID,
		Email:          normEmail,
		Exp:            time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return signAndEncodeAccountPayload(secret, payloadJSON)
}

// VerifyAccountLinkEmailToken проверяет одноразовую ссылку из письма привязки.
func VerifyAccountLinkEmailToken(secret, expectedBrandID, token string) (*AccountLinkEmailClaims, error) {
	payloadJSON, err := verifyAccountMagicTokenPayload(secret, token)
	if err != nil {
		return nil, err
	}
	var claims AccountLinkEmailClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrAccountTokenMalformed
	}
	if claims.Typ != accountTokenTypLinkEmail {
		return nil, ErrAccountTokenType
	}
	if err := matchAccountTokenBrand(claims.BrandID, expectedBrandID); err != nil {
		return nil, err
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrAccountTokenExpired
	}
	if claims.ShmUserID <= 0 || claims.TelegramChatID <= 0 || strings.TrimSpace(claims.Email) == "" {
		return nil, ErrAccountTokenMalformed
	}
	return &claims, nil
}

// ParseAndVerifyAccountToken проверяет подпись, бренд и срок токена кабинета.
func ParseAndVerifyAccountToken(secret, expectedBrandID, token string) (*AccountTokenClaims, error) {
	payloadJSON, err := verifyAccountMagicTokenPayload(secret, token)
	if err != nil {
		return nil, err
	}
	var claims AccountTokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrAccountTokenMalformed
	}
	if claims.Typ != accountTokenTypAccount {
		return nil, ErrAccountTokenType
	}
	if err := matchAccountTokenBrand(claims.BrandID, expectedBrandID); err != nil {
		return nil, err
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrAccountTokenExpired
	}
	if claims.UserID <= 0 || strings.TrimSpace(claims.Email) == "" || strings.TrimSpace(claims.Login) == "" {
		return nil, ErrAccountTokenMalformed
	}
	return &claims, nil
}

// ParseAndVerifyAccountSignupToken проверяет onboarding-токен (без user_id).
func ParseAndVerifyAccountSignupToken(secret, expectedBrandID, token string) (*AccountSignupTokenClaims, error) {
	payloadJSON, err := verifyAccountMagicTokenPayload(secret, token)
	if err != nil {
		return nil, err
	}
	var claims AccountSignupTokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrAccountTokenMalformed
	}
	if claims.Typ != accountTokenTypSignup {
		return nil, ErrAccountTokenType
	}
	if err := matchAccountTokenBrand(claims.BrandID, expectedBrandID); err != nil {
		return nil, err
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrAccountTokenExpired
	}
	if strings.TrimSpace(claims.Email) == "" || strings.TrimSpace(claims.Login) == "" {
		return nil, ErrAccountTokenMalformed
	}
	return &claims, nil
}

func webSalesOrderTokenTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.WebSales.OrderTokenTTLHours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(cfg.WebSales.OrderTokenTTLHours) * time.Hour
}

func signOrderTokenPayload(secret []byte, payloadJSON []byte) []byte {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write(payloadJSON)
	return m.Sum(nil)
}

func accountTokenTTL(cfg *config.Config) time.Duration {
	return webSalesOrderTokenTTL(cfg)
}

// webSalesTokenFlowAvailable сообщает, настроены ли подписанные ссылки для веб-кабинета (WebSales.order_token_secret).
func webSalesTokenFlowAvailable(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.WebSales.OrderTokenSecret) != ""
}
