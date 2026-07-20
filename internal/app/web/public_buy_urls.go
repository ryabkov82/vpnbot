package web

import (
	"net/http"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

// publicOrderBaseURL — базовый URL сайта для ссылок в письмах (magic-link /account/session):
// эффективный PublicBaseURL бренда или scheme://Host.
func publicOrderBaseURL(cfg *config.Config, r *http.Request) string {
	if b := cfg.PublicBaseURL(); b != "" {
		return b
	}
	if r != nil && r.Host != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		} else if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		return scheme + "://" + r.Host
	}
	return ""
}
