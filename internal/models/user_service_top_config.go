package models

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrEmptyUserServiceTopConfig — пустая строка user_service.config.
var ErrEmptyUserServiceTopConfig = errors.New("empty user service config")

// UserServiceTopConfigRemnawave — фрагмент user_service.config.remnawave.
type UserServiceTopConfigRemnawave struct {
	InternalSquadName    string `json:"internal_squad_name"`
	TrafficLimitBytes    int64  `json:"traffic_limit_bytes"`
	TrafficLimitStrategy string `json:"traffic_limit_strategy"`
	HWIDDeviceLimit      int    `json:"hwid_device_limit"`
}

// UserServiceTopConfig — верхний user_service.config (сырой JSON из API).
type UserServiceTopConfig struct {
	Remnawave UserServiceTopConfigRemnawave `json:"remnawave"`
}

// ParseUserServiceTopConfig разбирает JSON из поля config экземпляра user service.
func ParseUserServiceTopConfig(raw string) (UserServiceTopConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return UserServiceTopConfig{}, ErrEmptyUserServiceTopConfig
	}
	var cfg UserServiceTopConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return UserServiceTopConfig{}, err
	}
	return cfg, nil
}

// ParseTopConfig делегирует в ParseUserServiceTopConfig.
func (us *UserService) ParseTopConfig() (UserServiceTopConfig, error) {
	return ParseUserServiceTopConfig(us.ConfigRaw)
}

// UserServiceTopConfigIsPremium возвращает true, если internal_squad_name совпадает с premium_squad_name из конфига.
func UserServiceTopConfigIsPremium(cfg UserServiceTopConfig, premiumSquadName string) bool {
	if strings.TrimSpace(premiumSquadName) == "" {
		return false
	}
	return strings.TrimSpace(cfg.Remnawave.InternalSquadName) == strings.TrimSpace(premiumSquadName)
}

// IsPremiumAntiBlockUserService проверяет, что услуга относится к premium AntiBlock
// (совпадение remnawave.internal_squad_name с premium_squad_name из конфига бота).
func IsPremiumAntiBlockUserService(us *UserService, premiumSquadName string) bool {
	if us == nil {
		return false
	}
	parsed, err := us.ParseTopConfig()
	if err != nil {
		return false
	}
	return UserServiceTopConfigIsPremium(parsed, premiumSquadName)
}
