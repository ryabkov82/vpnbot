package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type TrialFeature struct {
	Enabled             bool     `json:"enabled"`
	BaseServiceID       int      `json:"base_service_id"`
	RequireStartParam   bool     `json:"require_start_param"`
	AllowedStartParams  []string `json:"allowed_start_params"`
	EligibilityTTLHours int      `json:"eligibility_ttl_hours"`
}

type Features struct {
	Trial TrialFeature `json:"trial"`
}

type ServicesCfg struct {
	Category string `json:"category"`
}

type Assets struct {
	LogoURL string `json:"logo_url"`
}

type Payments struct {
	Profile string `json:"profile"`
}

// Конфигурация
type Config struct {
	Env        string `json:"app_env"`
	WebhookURL string `json:"webhook_url"`
	Port       string `json:"port"`
	WebPort    string `json:"web_port"`
	API        struct {
		BaseURL  string `json:"base_url"`
		APILogin string `json:"api_login"`
		APIPass  string `json:"api_pass"`
		Timeout  int    `json:"timeout_seconds"`
	} `json:"api"`
	Cli struct {
		URL string `json:"url"`
	} `json:"cli"`
	Telegram struct {
		Token         string `json:"token"`
		SupportChatID int64  `json:"support_chat_id"`
		LeadsChatID   int64  `json:"leads_chat_id"`
		SupportChat   string `json:"support_chat"`
		NewsChannel   string `json:"news_channel"`
	}
	Features Features    `json:"features"`
	Services ServicesCfg `json:"services"`
	Assets   Assets      `json:"assets"`
	Payments Payments    `json:"payments"`

	Admin struct {
		Token string `json:"token"`
	} `json:"admin"`

	PremiumSquadName         string `json:"premium_squad_name"`
	PremiumConnectBaseURL    string `json:"premium_connect_base_url"`
	PremiumLinkSigningSecret string `json:"premium_link_signing_secret"`

	// WebSales: секрет подписи ссылок /account/session и TTL; публичный URL сайта для писем.
	// enabled — сохранено в JSON для совместимости (раньше включало удалённый email-first /buy order flow).
	WebSales struct {
		Enabled            bool   `json:"enabled"`
		OrderTokenSecret   string `json:"order_token_secret"`
		OrderTokenTTLHours int    `json:"order_token_ttl_hours"`
		PublicBaseURL      string `json:"public_base_url"`
		// TTL токена «Личный кабинет» из Telegram (/account/link?token=...)
		TelegramLinkTokenTTLMinutes int `json:"telegram_link_token_ttl_minutes"`
		// TTL письма подтверждения привязки email (account_link_email)
		LinkConfirmEmailTTLMinutes int `json:"link_confirm_email_ttl_minutes"`
	} `json:"web_sales"`

	// WebAccount — вход в личный кабинет (OAuth и т.п.), без секретов по умолчанию.
	// Client secret задаётся только в production-config, не должен попадать в репозиторий.
	WebAccount struct {
		GoogleEnabled      bool   `json:"google_enabled"`
		GoogleClientID     string `json:"google_client_id"`
		GoogleClientSecret string `json:"google_client_secret"`
		GoogleRedirectURL  string `json:"google_redirect_url"`
	} `json:"web_account"`

	Email struct {
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     int    `json:"smtp_port"`
		SMTPUsername string `json:"smtp_username"`
		SMTPPassword string `json:"smtp_password"`
		FromEmail    string `json:"from_email"`
		FromName     string `json:"from_name"`
		Enabled      bool   `json:"enabled"`
	} `json:"email"`

	RemnawaveAPIURL   string `json:"remnawave_api_url"`
	RemnawaveAPIToken string `json:"remnawave_api_token"`

	// Brand — один активный бренд процесса. Секция обязательна: runtime требует
	// явного brand (см. Config.Normalize). Legacy-конфиг без brand невалиден для
	// запуска и поддерживается только как вход для renderer-миграции.
	Brand BrandConfig `json:"brand"`
}

const envConfigPath = "VPNBOT_CONFIG"

// Load загружает конфигурацию процесса: VPNBOT_CONFIG или стандартный поиск путей.
// При ошибке завершает процесс (совместимость с существующим поведением).
func Load() *Config {
	cfg, err := loadConfig()
	if err != nil {
		panic(err.Error())
	}
	return cfg
}

func loadConfig() (*Config, error) {
	if p := strings.TrimSpace(os.Getenv(envConfigPath)); p != "" {
		return LoadFromFile(p)
	}

	_, filename, _, _ := runtime.Caller(0)
	rootDir := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
	configPaths := []string{
		filepath.Join(rootDir, "configs", "config.json"),
		filepath.Join(".", "configs", "config.json"),
		"config.json",
	}

	var configFile string
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			configFile = path
			break
		}
	}
	if configFile == "" {
		return nil, fmt.Errorf("конфигурационный файл не найден. Проверьте пути: %v", configPaths)
	}
	return LoadFromFile(configFile)
}

// LoadFromFile читает JSON-конфиг из указанного пути, затем обязательно вызывает
// Normalize() (строгая валидация явного brand) до любых внешних side effects.
func LoadFromFile(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("путь к конфигурации пуст")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть конфигурацию %q: %w", path, err)
	}
	defer file.Close()

	cfg := &Config{}
	dec := json.NewDecoder(file)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("ошибка парсинга конфигурации %q: %w", path, err)
	}
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Telegram.Token) == "" {
		return nil, fmt.Errorf("требуется Telegram токен в конфиге")
	}
	return cfg, nil
}
