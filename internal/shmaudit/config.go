package shmaudit

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultTimeoutSeconds = 30

// Config — минимальная audit-конфигурация (только SHM API).
type Config struct {
	API struct {
		BaseURL string `json:"base_url"`
		Login   string `json:"api_login"`
		Pass    string `json:"api_pass"`
		Timeout int    `json:"timeout_seconds"`
	} `json:"api"`
}

// Options — параметры CLI-запуска аудита.
type Options struct {
	ConfigPath   string
	OutputDir    string
	FCCategory   string
	VFFCategory  string
	PageSize     int
	RequestDelay time.Duration
}

// LoadConfig читает минимальный audit-конфиг из JSON-файла.
// Не использует config.Load / config.LoadFromFile.
func LoadConfig(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("config path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.API.BaseURL = strings.TrimSpace(cfg.API.BaseURL)
	cfg.API.Login = strings.TrimSpace(cfg.API.Login)
	// api_pass не TrimSpace — пароль может содержать пробелы по краям намеренно,
	// но пустая строка после обычной проверки запрещена.
	if cfg.API.BaseURL == "" {
		return nil, fmt.Errorf("api.base_url is required")
	}
	if _, err := url.ParseRequestURI(cfg.API.BaseURL); err != nil {
		return nil, fmt.Errorf("api.base_url is invalid")
	}
	if cfg.API.Login == "" {
		return nil, fmt.Errorf("api.api_login is required")
	}
	if cfg.API.Pass == "" {
		return nil, fmt.Errorf("api.api_pass is required")
	}
	if cfg.API.Timeout <= 0 {
		cfg.API.Timeout = defaultTimeoutSeconds
	}
	return &cfg, nil
}

// ValidateOptions проверяет CLI-параметры аудита.
func ValidateOptions(opt Options) error {
	if strings.TrimSpace(opt.ConfigPath) == "" {
		return fmt.Errorf("--config is required")
	}
	if strings.TrimSpace(opt.OutputDir) == "" {
		return fmt.Errorf("--output is required")
	}
	fc := strings.TrimSpace(opt.FCCategory)
	vff := strings.TrimSpace(opt.VFFCategory)
	if fc == "" {
		return fmt.Errorf("--fc-category is required")
	}
	if vff == "" {
		return fmt.Errorf("--vff-category is required")
	}
	if fc == vff {
		return fmt.Errorf("--fc-category and --vff-category must differ")
	}
	if opt.PageSize < 1 || opt.PageSize > 1000 {
		return fmt.Errorf("--page-size must be in range 1..1000")
	}
	if opt.RequestDelay < 0 {
		return fmt.Errorf("--request-delay must be >= 0")
	}
	return nil
}

// BaseURLHost возвращает host из base_url без userinfo/query.
func BaseURLHost(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
