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

// WebLoginFromEmail строит стабильный login: web_ + первые 16 hex-символов SHA256(normalized).
// normalized = strings.ToLower(strings.TrimSpace(email)).
func WebLoginFromEmail(email string) string {
	norm := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(norm))
	h := hex.EncodeToString(sum[:])
	return "web_" + h[:16]
}
