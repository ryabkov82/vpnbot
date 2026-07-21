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

// ErrWebLoginPrefixRequired — пустой prefix в parameterized helper недопустим.
var ErrWebLoginPrefixRequired = errors.New("web login prefix is required")

// defaultWebLoginPrefix — только для явной VFF-compatibility функции WebLoginFromEmail.
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
// normalized = strings.ToLower(strings.TrimSpace(email)).
// Пустой prefix после TrimSpace — ошибка (без fallback на "web_").
func WebLoginFromEmailWithPrefix(email, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", ErrWebLoginPrefixRequired
	}
	norm := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(norm))
	h := hex.EncodeToString(sum[:])
	return prefix + h[:16], nil
}

// WebLoginFromEmail — явная VFF-compatibility функция: всегда использует prefix "web_".
// Не является fallback для parameterized API.
func WebLoginFromEmail(email string) string {
	login, err := WebLoginFromEmailWithPrefix(email, defaultWebLoginPrefix)
	if err != nil {
		panic("explicit default prefix must be valid")
	}
	return login
}
