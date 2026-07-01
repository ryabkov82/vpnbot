package models

import "encoding/json"

type User struct {
	ID       int          `json:"user_id"`
	Login    string       `json:"login"`
	Login2   string       `json:"login2,omitempty"`
	Balance  float64      `json:"balance"`
	Settings UserSettings `json:"settings"`
}

type UserBalance struct {
	ID       int     `json:"user_id"`
	Balance  float64 `json:"balance"`
	Forecast float64 `json:"forecast"`
}

type UserService struct {
	Name          string `json:"name"`
	UserID        int    `json:"user_id"`
	Cost          string `json:"cost"`
	Status        string `json:"status"`
	Expire        string `json:"expire"`
	Period        string `json:"period"`
	ServiceID     int    `json:"user_service_id"`
	BaseServiceID int    `json:"service_id"`
	Category      string `json:"category"`
	ConfigRaw     string `json:"config"`
	KeyMarzban    UserKeyMarzban
}

type UserRegistrationRequest struct {
	Login    string       `json:"login"`
	Password string       `json:"password"`
	FullName string       `json:"full_name"`
	Settings UserSettings `json:"settings"`
}

type UserSettings struct {
	Telegram TelegramInfo `json:"telegram"`
	Web      WebInfo      `json:"web,omitempty"`
}

// WebInfo — метаданные web-пользователя (SHM settings.web).
type WebInfo struct {
	Email  string `json:"email"`
	Source string `json:"source"`
}

type TelegramInfo struct {
	UserID       string                 `json:"user_id"`
	Username     string                 `json:"username"`
	Login        string                 `json:"login"`
	FirstName    string                 `json:"first_name"`
	LastName     string                 `json:"last_name"`
	LanguageCode string                 `json:"language_code"`
	IsPremium    bool                   `json:"is_premium"`
	ChatID       int64                  `json:"chat_id"`
	Profile      map[string]interface{} `json:"telegram_bot"`
}

// ServiceBotConfig — service.config.remnawave.bot (SHM API).
type ServiceBotConfig struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ServiceRemnawaveConfig — service.config.remnawave.
type ServiceRemnawaveConfig struct {
	InternalSquadName string           `json:"internal_squad_name"`
	Bot               ServiceBotConfig `json:"bot"`
}

// ServicePricingConfig — service.config.pricing (SHM international catalog).
type ServicePricingConfig struct {
	PublicCode               string `json:"public_code"`
	InternationalEnabled     bool   `json:"international_enabled"`
	InternationalCurrency    string `json:"international_currency"`
	InternationalAmountCents int64  `json:"international_amount_cents"`
}

// ServiceDisplayLocaleConfig — localized title/description for catalog UI.
type ServiceDisplayLocaleConfig struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ServiceDisplayConfig — service.config.display (SHM localized catalog copy).
type ServiceDisplayConfig struct {
	RU ServiceDisplayLocaleConfig `json:"ru"`
	EN ServiceDisplayLocaleConfig `json:"en"`
}

// ServiceConfig — service.config из ответа /shm/v1/admin/service.
type ServiceConfig struct {
	Remnawave ServiceRemnawaveConfig `json:"remnawave"`
	Pricing   ServicePricingConfig   `json:"pricing"`
	Display   ServiceDisplayConfig   `json:"display"`
}

type Service struct {
	ServiceID    int            `json:"service_id"`
	Name         string         `json:"name"`
	Descr        string         `json:"descr"`
	Cost         float64        `json:"cost"`
	Period       float32        `json:"period"`
	AllowToOrder int            `json:"allow_to_order"`
	Config       *ServiceConfig `json:"config"`
}

// UserPay — запись user/pay (SHM). money может быть дробным и отрицательным.
type UserPay struct {
	ID          int             `json:"id"`
	UserID      int             `json:"user_id"`
	Date        string          `json:"date"`
	Money       float64         `json:"money"`
	PaySystemID string          `json:"pay_system_id"`
	UniqKey     string          `json:"uniq_key"`
	Comment     json.RawMessage `json:"comment,omitempty"`
}

type UserKeyMarzban struct {
	SubscriptionURL string   `json:"subscription_url"`
	Links           []string `json:"links"`
}

type WithdrawItem struct {
	Bonus         float64 `json:"bonus"`
	Cost          float64 `json:"cost"`
	CreateDate    string  `json:"create_date"`
	Discount      float64 `json:"discount"`
	EndDate       string  `json:"end_date"`
	Months        float64 `json:"months"`
	Name          string  `json:"name"`
	Qnt           int     `json:"qnt"`
	ServiceID     int     `json:"service_id"`
	Total         float64 `json:"total"`
	UserID        int64   `json:"user_id"`
	UserServiceID int64   `json:"user_service_id"`
	WithdrawDate  string  `json:"withdraw_date"`
	WithdrawID    int64   `json:"withdraw_id"`
}
