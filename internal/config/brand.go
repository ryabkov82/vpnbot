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
}

const (
	defaultBrandID                 = "vff"
	defaultBrandName               = "VPN for Friends"
	defaultBrandHost               = "connect.vpn-for-friends.com"
	defaultBrandLandingURL         = "https://vpn-for-friends.com"
	defaultBrandWebUserLoginPrefix = "web_"
	defaultBrandWebUserSource      = "vpn-for-friends.com"
)

var brandIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// EffectiveBrand возвращает активный бренд процесса.
// Nil-safe: (*Config)(nil) и частично заполненные тестовые конфиги не паникуют.
//
// Если Brand.ID пуст — синтезируется VFF из legacy-полей и defaults.
// Если Brand.ID заполнен — brand.* является единственным источником истины
// (legacy Services.Category / WebSales.PublicBaseURL / Payments.Profile не подмешиваются).
func (c *Config) EffectiveBrand() BrandConfig {
	if c == nil {
		return synthesizeVFFBrand("", "", "")
	}
	if strings.TrimSpace(c.Brand.ID) == "" {
		return synthesizeVFFBrand(c.WebSales.PublicBaseURL, c.Services.Category, c.Payments.Profile)
	}
	return normalizeBrandFields(c.Brand)
}

// ServiceCategory — эффективная категория услуг активного бренда (nil-safe).
func (c *Config) ServiceCategory() string {
	return strings.TrimSpace(c.EffectiveBrand().ServiceCategory)
}

// PublicBaseURL — эффективный публичный base URL активного бренда (nil-safe).
func (c *Config) PublicBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(c.EffectiveBrand().PublicBaseURL), "/")
}

// PaymentProfile — эффективный платёжный профиль активного бренда (nil-safe).
func (c *Config) PaymentProfile() string {
	return strings.TrimSpace(c.EffectiveBrand().PaymentProfile)
}

// WebUserLoginPrefix — префикс web-login активного бренда (nil-safe).
func (c *Config) WebUserLoginPrefix() string {
	p := strings.TrimSpace(c.EffectiveBrand().WebUserLoginPrefix)
	if p == "" {
		return defaultBrandWebUserLoginPrefix
	}
	return p
}

// WebUserSource — settings.web.source активного бренда (nil-safe).
func (c *Config) WebUserSource() string {
	s := strings.TrimSpace(c.EffectiveBrand().WebUserSource)
	if s == "" {
		return defaultBrandWebUserSource
	}
	return s
}

func synthesizeVFFBrand(publicBaseURL, serviceCategory, paymentProfile string) BrandConfig {
	return normalizeBrandFields(BrandConfig{
		ID:                 defaultBrandID,
		Name:               defaultBrandName,
		AllowedHosts:       []string{defaultBrandHost},
		PublicBaseURL:      strings.TrimSpace(publicBaseURL),
		LandingURL:         defaultBrandLandingURL,
		ServiceCategory:    strings.TrimSpace(serviceCategory),
		WebUserLoginPrefix: defaultBrandWebUserLoginPrefix,
		WebUserSource:      defaultBrandWebUserSource,
		PaymentProfile:     strings.TrimSpace(paymentProfile),
	})
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

// Normalize строит эффективный бренд и валидирует конфигурацию после чтения JSON.
// Для legacy (Brand.ID пуст) синтезирует VFF без новых production-breaking требований.
// Для явного brand — строгая валидация brand.* без подмешивания legacy-полей.
func (c *Config) Normalize() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(c.Brand.ID) == "" {
		c.Brand = synthesizeVFFBrand(c.WebSales.PublicBaseURL, c.Services.Category, c.Payments.Profile)
		return nil
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
