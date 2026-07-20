package webuser

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
)

// ErrInvalidEmail возвращается из NormalizeEmail при неверном адресе.
var ErrInvalidEmail = errors.New("invalid email")

const defaultWebLoginPrefix = "web_"

// NormalizeEmail: trim, lower-case для адресной части, проверка через net/mail.ParseAddress.
func NormalizeEmail(email string) (string, error) {
	s := strings.TrimSpace(email)
	if s == "" {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return "", ErrInvalidEmail
	}
	if strings.TrimSpace(addr.Name) != "" {
		return "", ErrInvalidEmail
	}
	return strings.ToLower(addr.Address), nil
}

// WebLoginFromEmailWithPrefix строит стабильный login: <prefix> + первые 16 hex SHA256(normalized).
// normalized = strings.ToLower(strings.TrimSpace(email)). Пустой prefix трактуется как "web_".
func WebLoginFromEmailWithPrefix(email, prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = defaultWebLoginPrefix
	}
	norm := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(norm))
	h := hex.EncodeToString(sum[:])
	return prefix + h[:16]
}

// WebLoginFromEmail строит стабильный login: web_ + первые 16 hex-символов SHA256(normalized).
// Сохранена для совместимости; эквивалентна WebLoginFromEmailWithPrefix(email, "web_").
func WebLoginFromEmail(email string) string {
	return WebLoginFromEmailWithPrefix(email, defaultWebLoginPrefix)
}
