package shmaudit

import (
	"encoding/json"
	"time"
)

// Page — универсальный envelope пагинации SHM Admin API.
type Page[T any] struct {
	Data   []T `json:"data"`
	Items  int `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Status int `json:"status"`
}

// AuditUser — пользователь для аудита (отдельная модель от runtime User).
type AuditUser struct {
	UserID    int             `json:"user_id"`
	Login     string          `json:"login"`
	Login2    string          `json:"login2"`
	Balance   float64         `json:"balance"`
	Bonus     float64         `json:"bonus"`
	Credit    float64         `json:"credit"`
	Created   string          `json:"created"`
	LastLogin string          `json:"last_login"`
	Settings  json.RawMessage `json:"settings"`
}

// AuditUserService — пользовательская услуга.
type AuditUserService struct {
	UserID        int    `json:"user_id"`
	UserServiceID int    `json:"user_service_id"`
	ServiceID     int    `json:"service_id"`
	Category      string `json:"category"`
	Status        string `json:"status"`
	Created       string `json:"created"`
	Expire        string `json:"expire"`
}

// AuditService — запись каталога услуг.
type AuditService struct {
	ServiceID int    `json:"service_id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Deleted   int    `json:"deleted"`
}

// AuditWithdraw — списание по услуге.
type AuditWithdraw struct {
	WithdrawID    int64   `json:"withdraw_id"`
	UserID        int     `json:"user_id"`
	UserServiceID int64   `json:"user_service_id"`
	ServiceID     int     `json:"service_id"`
	WithdrawDate  string  `json:"withdraw_date"`
	CreateDate    string  `json:"create_date"`
	Total         float64 `json:"total"`
}

// AuditPay — платёж (comment намеренно не сохраняется).
type AuditPay struct {
	ID          int     `json:"id"`
	UserID      int     `json:"user_id"`
	Date        string  `json:"date"`
	Money       float64 `json:"money"`
	PaySystemID string  `json:"pay_system_id"`
}

// TelegramHints — безопасное извлечение telegram-полей из settings.
type TelegramHints struct {
	Present     bool
	ChatID      int64
	ChatIDValid bool
	Username    string
	ChatIDRawOK bool // удалось корректно разобрать как положительный int64
}

// SettingsHints — безопасное извлечение полей из settings.
type SettingsHints struct {
	BrandID            string
	BrandIDPresent     bool
	Telegram           TelegramHints
	TelegramKeyPresent bool
}

const (
	ClassFCOnly    = "fc_only"
	ClassVFFOnly   = "vff_only"
	ClassShared    = "shared"
	ClassEmpty     = "empty"
	ClassAmbiguous = "ambiguous"
)

const (
	ActionRenameFC         = "rename login to @fc_<chat_id>; set settings.brand_id=fc"
	ActionDoNotMigrate     = "do_not_migrate"
	ActionManualReview     = "manual_review"
	ActionDoNotMigrateAuto = "do_not_migrate_automatically"
)

// AuditRecord — одна запись отчёта по legacy-кандидату.
type AuditRecord struct {
	Classification string `json:"classification"`

	UserID           int    `json:"user_id"`
	Login            string `json:"login"`
	ProposedLogin    string `json:"proposed_login,omitempty"`
	TelegramChatID   int64  `json:"telegram_chat_id,omitempty"`
	TelegramUsername string `json:"telegram_username,omitempty"`
	Created          string `json:"created,omitempty"`
	LastLogin        string `json:"last_login,omitempty"`
	BrandID          string `json:"brand_id,omitempty"`

	Balance float64 `json:"balance"`
	Bonus   float64 `json:"bonus"`
	Credit  float64 `json:"credit"`

	Login2Present bool `json:"login2_present"`

	ServiceCategories    []string `json:"service_categories"`
	ServiceStatuses      []string `json:"service_statuses"`
	ServiceCount         int      `json:"service_count"`
	WithdrawalCategories []string `json:"withdrawal_categories"`
	WithdrawalCount      int      `json:"withdrawal_count"`
	PaymentCount         int      `json:"payment_count"`
	PaySystemIDs         []string `json:"pay_system_ids"`

	OtherCategories      []string `json:"other_categories"`
	UnresolvedServiceIDs []int    `json:"unresolved_service_ids"`

	TargetLoginExists bool `json:"target_login_exists"`
	TargetLoginUserID int  `json:"target_login_user_id,omitempty"`

	ProposedAction string   `json:"proposed_action"`
	Reasons        []string `json:"reasons"`

	EvidenceHash string `json:"evidence_hash"`
}

// FetchedCounts — счётчики полностью загруженных сущностей.
type FetchedCounts struct {
	Users        int `json:"users"`
	UserServices int `json:"user_services"`
	Services     int `json:"services"`
	Withdrawals  int `json:"withdrawals"`
	Payments     int `json:"payments"`
}

// ClassificationCounts — сводка по классам.
type ClassificationCounts struct {
	FCOnly    int `json:"fc_only"`
	VFFOnly   int `json:"vff_only"`
	Shared    int `json:"shared"`
	Empty     int `json:"empty"`
	Ambiguous int `json:"ambiguous"`
}

// Summary — summary.json.
type Summary struct {
	GeneratedAt         time.Time            `json:"generated_at"`
	Complete            bool                 `json:"complete"`
	BaseURLHost         string               `json:"base_url_host"`
	FCCategory          string               `json:"fc_category"`
	VFFCategory         string               `json:"vff_category"`
	PageSize            int                  `json:"page_size"`
	Fetched             FetchedCounts        `json:"fetched"`
	LegacyTelegramUsers int                  `json:"legacy_telegram_users"`
	Classifications     ClassificationCounts `json:"classifications"`
}

// Dataset — полный набор данных после успешной загрузки всех endpoint'ов.
type Dataset struct {
	Users        []AuditUser
	UserServices []AuditUserService
	Services     []AuditService
	Withdrawals  []AuditWithdraw
	Payments     []AuditPay
}
