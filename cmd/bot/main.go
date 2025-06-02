package main

import (
	"log"
	"time"

	"gopkg.in/telebot.v3"

	"github.com/ryabkov82/vpnbot/internal/app/bot"
	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/service"
)

func main() {
	// Загрузка конфигурации
	cfg := config.Load()

	// Использование параметров
	botToken := cfg.Telegram.Token
	apiURL := cfg.API.BaseURL

	log.Printf("Запуск бота с токеном: %s", botToken)
	log.Printf("API endpoint: %s", apiURL)

	// 2. Создание API клиентов

	apiClient := api.NewAPIClient(cfg)

	// 3. Получение session_id (аутентификация в API)
	if err := apiClient.Authenticate(); err != nil {
		log.Fatalf("Ошибка аутентификации в API: %v", err)
	}

	// 4. Инициализация UseCase
	service := service.NewService(apiClient)

	// 5. Создание сервиса бота
	botService := bot.NewService(service, cfg)

	// 5. Инициализация Handler
	botHandler := bot.NewBotHandler(botService)

	// 6. Настройка Telegram бота
	settings := telebot.Settings{
		Token: botToken,
	}

	// Режим разработки (LongPoller)
	if cfg.Env == "development" {
		settings.Poller = &telebot.LongPoller{Timeout: 5 * time.Second}
	} else {
		// Продуктовый режим (Webhook)
		webhookURL := cfg.WebhookURL // Например, "https://yourdomain.com/bot-webhook"
		settings.Poller = &telebot.Webhook{
			Listen: ":" + cfg.Port, // Порт сервера (например, "8080")
			Endpoint: &telebot.WebhookEndpoint{
				PublicURL: webhookURL,
			},
		}
	}

	b, err := telebot.NewBot(settings)
	if err != nil {
		panic(err)
	}

	// Установка команд меню
	if err := botHandler.SetBotCommands(b); err != nil {
		panic(err)
	}
	// 7. Регистрация обработчиков
	botHandler.RegisterHandlers(b)

	// 8. Запуск периодического обновления session_id
	go apiClient.StartSessionRefresher()

	log.Println("Бот запущен и готов к работе...")
	b.Start()

}
