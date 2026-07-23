package config

import (
	"fmt"
	"strings"
)

// FormatSafeBrandSummary — безопасная сводка для configcheck (без секретов).
func FormatSafeBrandSummary(cfg *Config) string {
	b := EffectiveBrandOf(cfg)
	hosts := strings.Join(b.AllowedHosts, ",")
	var sb strings.Builder
	sb.WriteString("config valid\n")
	sb.WriteString(fmt.Sprintf("brand.id=%s\n", b.ID))
	sb.WriteString(fmt.Sprintf("brand.name=%s\n", b.Name))
	sb.WriteString(fmt.Sprintf("brand.public_base_url=%s\n", b.PublicBaseURL))
	sb.WriteString(fmt.Sprintf("brand.service_category=%s\n", b.ServiceCategory))
	sb.WriteString(fmt.Sprintf("brand.allowed_hosts=%s\n", hosts))
	sb.WriteString(fmt.Sprintf("brand.web_user_login_prefix=%s\n", b.WebUserLoginPrefix))
	sb.WriteString(fmt.Sprintf("brand.web_user_source=%s\n", b.WebUserSource))
	sb.WriteString(fmt.Sprintf("brand.payment_profile=%s\n", b.PaymentProfile))
	sb.WriteString(fmt.Sprintf("brand.yookassa_pay_system=%s\n", b.YooKassaPaySystem))
	return sb.String()
}

// FormatActiveBrandLogLine — одна безопасная строка для startup-лога процесса.
func FormatActiveBrandLogLine(cfg *Config) string {
	b := EffectiveBrandOf(cfg)
	hosts := strings.Join(b.AllowedHosts, ",")
	return fmt.Sprintf(
		"active brand: id=%s name=%q public_base_url=%s service_category=%s allowed_hosts=%s",
		b.ID, b.Name, b.PublicBaseURL, b.ServiceCategory, hosts,
	)
}

// EffectiveBrandOf — nil-safe обёртка над EffectiveBrand.
func EffectiveBrandOf(cfg *Config) BrandConfig {
	if cfg == nil {
		return (*Config)(nil).EffectiveBrand()
	}
	return cfg.EffectiveBrand()
}
