package models

type User struct {
	ID       int          `json:"user_id"`
	Login    string       `json:"login"`
	Balance  float64      `json:"balance"`
	Settings UserSettings `json:"settings"`
}

type UserBalance struct {
	ID       string  `json:"user_id"`
	Balance  float64 `json:"balance"`
	Forecast float64 `json:"forecast"`
}

type UserRegistrationRequest struct {
	Login    string       `json:"login"`
	Password string       `json:"password"`
	FullName string       `json:"full_name"`
	Settings UserSettings `json:"settings"`
}

type UserSettings struct {
	Telegram TelegramInfo `json:"telegram"`
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
