package main

import (
	"log"
	"strings"
	"time"

	"gopkg.in/telebot.v3"

	"github.com/ryabkov82/vpnbot/internal/app/bot"
	"github.com/ryabkov82/vpnbot/internal/app/web"
	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/remnawave"
	"github.com/ryabkov82/vpnbot/internal/service"
)

func main() {
	cfg := config.Load()

	log.Print(config.FormatActiveBrandLogLine(cfg))
	log.Print("telegram bot configured")
	log.Printf("API endpoint: %s", cfg.API.BaseURL)

	apiClient := api.NewAPIClient(cfg)

	if err := apiClient.Authenticate(); err != nil {
		log.Fatalf("Ошибка аутентификации в API: %v", err)
	}

	svc := service.NewService(apiClient, cfg.EffectiveBrand())
	botService := bot.NewService(svc, cfg)
	botHandler := bot.NewBotHandler(botService)

	settings := telebot.Settings{
		Token: cfg.Telegram.Token,
	}

	if cfg.Env == "development" {
		settings.Poller = &telebot.LongPoller{Timeout: 5 * time.Second}
	} else {
		webhookURL := cfg.WebhookURL
		settings.Poller = &telebot.Webhook{
			Listen: ":" + cfg.Port,
			Endpoint: &telebot.WebhookEndpoint{
				PublicURL: webhookURL,
			},
		}
	}

	b, err := telebot.NewBot(settings)
	if err != nil {
		panic(err)
	}

	if err := botHandler.SetBotCommands(b); err != nil {
		panic(err)
	}
	botHandler.RegisterHandlers(b)

	go apiClient.StartSessionRefresher()

	var rwClient *remnawave.Client
	if strings.TrimSpace(cfg.RemnawaveAPIURL) != "" && strings.TrimSpace(cfg.RemnawaveAPIToken) != "" {
		rwClient = remnawave.NewClient(cfg.RemnawaveAPIURL, cfg.RemnawaveAPIToken)
	}
	web.Start(cfg, svc, rwClient)

	log.Println("Бот запущен и готов к работе...")
	b.Start()
}
