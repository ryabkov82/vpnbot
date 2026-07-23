package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// BrandConfig — один активный бренд процесса (один процесс = один BrandConfig).
type BrandConfig struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	AllowedHosts       []string `json:"allowed_hosts"`
	PublicBaseURL      string   `json:"public_base_url"`
	LandingURL         string   `json:"landing_url"`
	ServiceCategory    string   `json:"service_category"`
	WebUserLoginPrefix string   `json:"web_user_login_prefix"`
	WebUserSource      string   `json:"web_user_source"`
	PaymentProfile     string   `json:"payment_profile"`
	// YooKassaPaySystem — имя ключа в SHM config.pay_systems для web/Telegram YooKassa overlay (ps=).
	// Не путать с PaymentProfile (Telegram WebApp auth profile).
	YooKassaPaySystem string `json:"yookassa_pay_system"`
}

var brandIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// shmPaySystemKeyPattern — безопасное имя ключа SHM pay_systems / query ps.
var shmPaySystemKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// EffectiveBrand возвращает активный бренд процесса.
// Nil-safe: (*Config)(nil) и частично заполненные тестовые конфиги не паникуют.
//
// Runtime требует явного brand: пустой Brand НЕ превращается в VFF, а возвращается
// как есть (нормализованным). Корректность обеспечивается обязательным вызовом
// Normalize() при загрузке конфигурации, который отклоняет отсутствие brand.id.
// Legacy Services.Category / WebSales.PublicBaseURL / Payments.Profile здесь не читаются.
func (c *Config) EffectiveBrand() BrandConfig {
	if c == nil {
		return BrandConfig{}
	}
	return normalizeBrandFields(c.Brand)
}

// ServiceCategory — категория услуг активного бренда (nil-safe, только explicit brand).
func (c *Config) ServiceCategory() string {
	return strings.TrimSpace(c.EffectiveBrand().ServiceCategory)
}

// PublicBaseURL — публичный base URL активного бренда (nil-safe, только explicit brand).
func (c *Config) PublicBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(c.EffectiveBrand().PublicBaseURL), "/")
}

// PaymentProfile — Telegram WebApp profile активного бренда (nil-safe, только explicit brand).
func (c *Config) PaymentProfile() string {
	return strings.TrimSpace(c.EffectiveBrand().PaymentProfile)
}

// YooKassaPaySystem — ключ SHM pay_systems для YooKassa (query ps=), nil-safe.
func (c *Config) YooKassaPaySystem() string {
	return strings.TrimSpace(c.EffectiveBrand().YooKassaPaySystem)
}

// WebUserLoginPrefix — префикс web-login активного бренда (nil-safe, только explicit brand).
func (c *Config) WebUserLoginPrefix() string {
	return strings.TrimSpace(c.EffectiveBrand().WebUserLoginPrefix)
}

// WebUserSource — settings.web.source активного бренда (nil-safe, только explicit brand).
func (c *Config) WebUserSource() string {
	return strings.TrimSpace(c.EffectiveBrand().WebUserSource)
}

func normalizeBrandFields(b BrandConfig) BrandConfig {
	b.ID = strings.TrimSpace(b.ID)
	b.Name = strings.TrimSpace(b.Name)
	b.PublicBaseURL = strings.TrimRight(strings.TrimSpace(b.PublicBaseURL), "/")
	b.LandingURL = strings.TrimRight(strings.TrimSpace(b.LandingURL), "/")
	b.ServiceCategory = strings.TrimSpace(b.ServiceCategory)
	b.WebUserLoginPrefix = strings.TrimSpace(b.WebUserLoginPrefix)
	b.WebUserSource = strings.TrimSpace(b.WebUserSource)
	b.PaymentProfile = strings.TrimSpace(b.PaymentProfile)
	b.YooKassaPaySystem = strings.TrimSpace(b.YooKassaPaySystem)
	b.AllowedHosts = normalizeAllowedHosts(b.AllowedHosts)
	return b
}

func normalizeAllowedHosts(hosts []string) []string {
	seen := make(map[string]struct{}, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		n, ok := normalizeHostEntry(h)
		if !ok {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// normalizeHostEntry — trim, lowercase, одна DNS trailing dot; строгий DNS hostname.
// Возвращает ok=false для пустых и невалидных значений (scheme, port, path, wildcard и т.п.).
func normalizeHostEntry(raw string) (string, bool) {
	h := strings.TrimSpace(raw)
	if h == "" {
		return "", false
	}
	// Пробельные символы внутри значения запрещены (leading/trailing уже сняты TrimSpace).
	if strings.ContainsAny(h, " \t\n\r") {
		return "", false
	}
	lower := strings.ToLower(h)
	if strings.Contains(lower, "://") || strings.ContainsAny(lower, "/?#:@*") {
		return "", false
	}
	// Разрешена ровно одна завершающая DNS-точка; TrimRight('.', '.') недопустим.
	if strings.HasSuffix(lower, ".") {
		lower = strings.TrimSuffix(lower, ".")
		if lower == "" || strings.HasSuffix(lower, ".") {
			return "", false
		}
	}
	if !validHostname(lower) {
		return "", false
	}
	return lower, true
}

// validHostname проверяет DNS hostname (и single-label localhost) без порта и IP-литералов.
func validHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) == 0 {
		return false
	}
	for _, label := range labels {
		if !validHostnameLabel(label) {
			return false
		}
	}
	return true
}

func validHostnameLabel(label string) bool {
	n := len(label)
	if n < 1 || n > 63 {
		return false
	}
	for i := 0; i < n; i++ {
		c := label[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if i == 0 || i == n-1 {
			if !isAlphaNum {
				return false
			}
			continue
		}
		if !isAlphaNum && c != '-' {
			return false
		}
	}
	return true
}

// Normalize нормализует и строго валидирует явный brand после чтения JSON.
// Runtime требует полностью заданной секции brand: отсутствие brand.id —
// ошибка конфигурации (legacy-поля services/web_sales/payments не подмешиваются).
// Legacy-конфиг без brand поддерживается только как вход для renderer-миграции,
// но невалиден для запуска процесса.
func (c *Config) Normalize() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(c.Brand.ID) == "" {
		return fmt.Errorf("brand.id is required")
	}
	if err := validateExplicitBrandRawHosts(c.Brand.AllowedHosts); err != nil {
		return err
	}
	c.Brand = normalizeBrandFields(c.Brand)
	return validateExplicitBrand(c.Brand)
}

func validateExplicitBrand(b BrandConfig) error {
	if b.ID == "" {
		return fmt.Errorf("brand.id is required")
	}
	if !brandIDPattern.MatchString(b.ID) {
		return fmt.Errorf("brand.id %q is invalid: must match %s", b.ID, brandIDPattern.String())
	}
	if b.Name == "" {
		return fmt.Errorf("brand.name is required")
	}
	if len(b.AllowedHosts) == 0 {
		return fmt.Errorf("brand.allowed_hosts must contain at least one host")
	}
	for _, raw := range b.AllowedHosts {
		// после normalizeAllowedHosts сюда попадают только валидные; перепроверим исходный контракт
		if _, ok := normalizeHostEntry(raw); !ok {
			return fmt.Errorf("brand.allowed_hosts contains invalid host %q", raw)
		}
	}
	if err := validateAbsoluteHTTPURL("brand.public_base_url", b.PublicBaseURL); err != nil {
		return err
	}
	if err := validateAbsoluteHTTPURL("brand.landing_url", b.LandingURL); err != nil {
		return err
	}
	if b.ServiceCategory == "" {
		return fmt.Errorf("brand.service_category is required")
	}
	if b.WebUserLoginPrefix == "" {
		return fmt.Errorf("brand.web_user_login_prefix is required")
	}
	if b.WebUserSource == "" {
		return fmt.Errorf("brand.web_user_source is required")
	}
	if b.PaymentProfile == "" {
		return fmt.Errorf("brand.payment_profile is required")
	}
	if b.YooKassaPaySystem == "" {
		return fmt.Errorf("brand.yookassa_pay_system is required")
	}
	if !shmPaySystemKeyPattern.MatchString(b.YooKassaPaySystem) {
		return fmt.Errorf("brand.yookassa_pay_system %q is invalid: must match %s", b.YooKassaPaySystem, shmPaySystemKeyPattern.String())
	}
	return nil
}

func validateAbsoluteHTTPURL(field, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s is required", field)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", field, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must be an absolute http/https URL", field)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must be an absolute http/https URL", field)
	}
	return nil
}

// validateExplicitBrandRawHosts проверяет hosts до нормализации.
// Любой невалидный или пустой элемент отклоняет всю explicit-конфигурацию (молча не отбрасываем).
func validateExplicitBrandRawHosts(hosts []string) error {
	if len(hosts) == 0 {
		return fmt.Errorf("brand.allowed_hosts must contain at least one host")
	}
	for _, h := range hosts {
		if strings.TrimSpace(h) == "" {
			return fmt.Errorf("brand.allowed_hosts contains invalid host %q", h)
		}
		if _, ok := normalizeHostEntry(h); !ok {
			return fmt.Errorf("brand.allowed_hosts contains invalid host %q", h)
		}
	}
	return nil
}
