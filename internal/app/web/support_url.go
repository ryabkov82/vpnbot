package web

import (
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

// envTelegramSupportURL переопределяет telegram.support_chat для ссылки «Поддержка»
// в web-кабинете (полный URL или @username — см. normalizeSupportCandidate).
const (
	envTelegramSupportURL       = "TELEGRAM_SUPPORT_URL"
	envTelegramSupportURLLegacy = "SUPPORT_TELEGRAM_URL"
)

var telegramUsernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{5,32}$`)

// WebCabinetResolvedSupportURL безопасный href для блока поддержки или "" если не задано.
// Приоритет: TELEGRAM_SUPPORT_URL (если не пустая), затем SUPPORT_TELEGRAM_URL, затем конфиг telegram.support_chat.
func WebCabinetResolvedSupportURL(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if ev := supportURLOverrideFromEnv(); ev != "" {
		if u, ok := normalizeSupportCandidate(ev); ok {
			return u
		}
	}
	raw := strings.TrimSpace(cfg.Telegram.SupportChat)
	if raw == "" {
		return ""
	}
	if u, ok := normalizeSupportCandidate(raw); ok {
		return u
	}
	return ""
}

// supportURLOverrideFromEnv: TELEGRAM_SUPPORT_URL, иначе SUPPORT_TELEGRAM_URL.
func supportURLOverrideFromEnv() string {
	if ev := strings.TrimSpace(os.Getenv(envTelegramSupportURL)); ev != "" {
		return ev
	}
	return strings.TrimSpace(os.Getenv(envTelegramSupportURLLegacy))
}

func normalizeSupportCandidate(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	lower := strings.ToLower(s)
	if dangerousURLPrefix(lower) {
		return "", false
	}

	unameOnly := bareUsernameCandidate(s)
	if unameOnly != "" && telegramUsernameRe.MatchString(unameOnly) {
		return "https://t.me/" + unameOnly, true
	}

	work := strings.TrimSpace(s)
	wl := strings.ToLower(work)

	if strings.Contains(work, "://") {
		// leave as-is for parsing
	} else {
		switch {
		case strings.HasPrefix(wl, "//t.me/") || strings.HasPrefix(wl, "//telegram.me/"):
			work = "https:" + work
		case strings.HasPrefix(wl, "t.me/") || strings.HasPrefix(wl, "telegram.me/"):
			work = "https://" + work
		}
	}

	u, err := url.Parse(work)
	if err != nil || u.Scheme == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)

	switch scheme {
	case "tg":
		return strings.TrimRight(work, "\t "), true
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
		return strings.TrimRight(work, "\t "), true
	default:
		return "", false
	}
}

func bareUsernameCandidate(s string) string {
	st := strings.TrimSpace(s)
	for strings.HasPrefix(st, "@") {
		st = strings.TrimPrefix(st, "@")
	}
	if st == "" || strings.ContainsAny(st, "/:?#&=") || strings.Contains(st, " ") {
		return ""
	}
	return st
}

func dangerousURLPrefix(lower string) bool {
	switch {
	case strings.HasPrefix(lower, "javascript:"):
		return true
	case strings.HasPrefix(lower, "data:"):
		return true
	case strings.HasPrefix(lower, "vbscript:"):
		return true
	default:
		return false
	}
}
