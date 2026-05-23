package web

import (
	"net/http"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

// publicOrderBaseURL — базовый URL сайта для ссылок в письмах (magic-link /account/session): WebSales.public_base_url или scheme://Host.
func publicOrderBaseURL(cfg *config.Config, r *http.Request) string {
	if cfg != nil {
		b := strings.TrimRight(strings.TrimSpace(cfg.WebSales.PublicBaseURL), "/")
		if b != "" {
			return b
		}
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
