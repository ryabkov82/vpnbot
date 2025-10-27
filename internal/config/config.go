package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

type TrialFeature struct {
	Enabled       bool   `json:"enabled"`
	BaseServiceID int    `json:"base_service_id"`
	ButtonText    string `json:"button_text"`
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
		SupportChat   string `json:"support_chat"`
		NewsChannel   string `json:"news_channel"`
	}
	Features Features    `json:"features"`
	Services ServicesCfg `json:"services"`
	Assets   Assets      `json:"assets"`
	Payments Payments    `json:"payments"`
}

func Load() *Config {
	// Получаем абсолютный путь к директории исполняемого файла
	_, filename, _, _ := runtime.Caller(0)
	rootDir := filepath.Dir(filepath.Dir(filepath.Dir(filename)))

	// Возможные расположения конфига
	configPaths := []string{
		filepath.Join(rootDir, "configs", "config.json"), // Основной путь
		filepath.Join(".", "configs", "config.json"),     // Для go run
		"config.json", // Текущая директория
	}

	var configFile string
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			configFile = path
			break
		}
	}

	if configFile == "" {
		panic("конфигурационный файл не найден. Проверьте пути: " + fmt.Sprintf("%v", configPaths))
	}

	file, err := os.Open(configFile)
	if err != nil {
		log.Fatal("Ошибка загрузки конфига:", err)
	}
	defer file.Close()

	config := &Config{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		log.Fatal("Ошибка парсинга конфига:", err)
	}

	// 5. Валидация
	if config.Telegram.Token == "" {
		panic("требуется Telegram токен в конфиге")
	}

	return config
}
