package bot

import (
	"strings"

	"gopkg.in/telebot.v3"
)

type BotHandler struct {
	service *Service
}

func NewBotHandler(service *Service) *BotHandler {
	return &BotHandler{service: service}
}

// RegisterHandlers связывает обработчики с роутером бота
func (h *BotHandler) RegisterHandlers(bot *telebot.Bot) {
	// Команды
	bot.Handle("/start", h.handleStart)
	bot.Handle("/register", h.handleRegister)
	// Callback-кнопки
	bot.Handle(telebot.OnCallback, h.handleCallbacks)
	/*

		// Текстовые сообщения
		bot.Handle(telebot.OnText, h.handleText)


		// Другие события
		bot.Handle(telebot.OnPhoto, h.handlePhotoUpload)
	*/
}

func (h *BotHandler) handleMenu(c telebot.Context) error {
	return h.service.handleMenu(c)
}

func (h *BotHandler) handleStart(c telebot.Context) error {
	return h.service.handleStart(c)
}

func (h *BotHandler) handleRegister(c telebot.Context) error {
	return h.service.handleRegister(c)
}

func (h *BotHandler) handleBalance(c telebot.Context) error {
	return h.service.handleBalance(c)
}

func (h *BotHandler) handleList(c telebot.Context) error {
	return h.service.handleList(c)
}

func (h *BotHandler) handleCallbacks(c telebot.Context) error {
	data := c.Callback().Data
	args := strings.Fields(data)
	cmd := args[0]

	switch cmd {
	case "/register":
		return h.handleRegister(c)
	case "/balance":
		return h.handleBalance(c)
	case "/menu":
		return h.handleMenu(c)
	case "/list":
		return h.handleList(c)
	/*
		case "/download_qr":
			return handleDownloadQR(c, args[1:])
		//case "/delete_confirmed":
		//	return handleDeleteConfirmed(c, args[1:])
		// ... другие обработчики
	*/
	default:
		return c.Respond(&telebot.CallbackResponse{
			Text: "Неизвестная команда",
		})
	}
}
