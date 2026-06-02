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
	// Обработчик кнопки "📋 Меню"
	bot.Handle("/menu", h.handleMenu)
	bot.Handle("/help", h.handleHelp)
	bot.Handle("/list", h.handleList)
	bot.Handle("/pricelist", h.handlePricelist)
	bot.Handle("/balance", h.handleBalance)
	bot.Handle("/account", h.handleAccount)
	// Callback-кнопки
	bot.Handle(telebot.OnCallback, h.handleCallbacks)
	/*

		// Текстовые сообщения
		bot.Handle(telebot.OnText, h.handleText)


		// Другие события
		bot.Handle(telebot.OnPhoto, h.handlePhotoUpload)
	*/
}

// Устанавливаем список команд для бота
func (h *BotHandler) SetBotCommands(bot *telebot.Bot) error {
	return bot.SetCommands(botMenuCommands())
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

func (h *BotHandler) handleService(c telebot.Context, serviceID string) error {
	return h.service.handleService(c, serviceID)
}

func (h *BotHandler) handleDownloadUserKey(c telebot.Context, serviceID string) error {
	return h.service.handleDownloadUserKey(c, serviceID)
}

func (h *BotHandler) handleShowQR(c telebot.Context, serviceID string) error {
	return h.service.handleShowQR(c, serviceID)
}

func (h *BotHandler) handleDelete(c telebot.Context, serviceID string) error {
	return h.service.handleDelete(c, serviceID)
}

func (h *BotHandler) handleDeleteConfirmed(c telebot.Context, serviceID string) error {
	return h.service.handleDeleteConfirmed(c, serviceID)
}

func (h *BotHandler) handlePricelist(c telebot.Context) error {
	return h.service.handlePricelist(c)
}

func (h *BotHandler) handleServiceOrder(c telebot.Context, serviceID string) error {
	return h.service.handleServiceOrder(c, serviceID)
}

func (h *BotHandler) handleServicePreview(c telebot.Context, serviceID string) error {
	return h.service.handleServicePreview(c, serviceID)
}

func (h *BotHandler) handleServiceBuy(c telebot.Context, serviceID string) error {
	return h.service.handleServiceBuy(c, serviceID)
}

func (h *BotHandler) handleHelp(c telebot.Context) error {
	return h.service.handleHelp(c)
}

func (h *BotHandler) handleAccount(c telebot.Context) error {
	return h.service.handleAccount(c)
}

func (h *BotHandler) handlePays(c telebot.Context) error {
	return h.service.handlePays(c)
}

func (h *BotHandler) handleShowMZ(c telebot.Context, serviceID string) error {
	return h.service.handleShowMZ(c, serviceID)
}

func (h *BotHandler) handleCallbacks(c telebot.Context) error {

	// 1. Всегда отвечаем на callback
	if err := c.Respond(); err != nil {
		return err
	}

	callbackData := c.Callback().Data

	// Убираем \f (если есть) и разбиваем по |
	cleanData := strings.TrimPrefix(callbackData, "\f")
	parts := strings.Split(cleanData, "|")

	cmd := parts[0]

	switch cmd {
	case "/register":
		return h.handleRegister(c)
	case "/balance":
		return h.handleBalance(c)
	case "/menu":
		return h.handleMenu(c)
	case "/list":
		return h.handleList(c)
	case "/trial":
		return h.service.handleTrial(c)
	case "/service":
		serviceIDStr := parts[1]
		return h.handleService(c, serviceIDStr)
	case "/download_qr":
		serviceIDStr := parts[1]
		return h.handleDownloadUserKey(c, serviceIDStr)
	case "/show_qr":
		serviceIDStr := parts[1]
		return h.handleShowQR(c, serviceIDStr)
	case "/delete":
		serviceIDStr := parts[1]
		return h.handleDelete(c, serviceIDStr)
	case "/delete_confirmed":
		serviceIDStr := parts[1]
		return h.handleDeleteConfirmed(c, serviceIDStr)
	case "/pricelist":
		return h.handlePricelist(c)
	case "/serviceorder":
		// Старые inline-кнопки: ведём на preview, а не на мгновенный заказ
		if len(parts) < 2 {
			return nil
		}
		return h.handleServicePreview(c, parts[1])
	case "service_preview":
		if len(parts) < 2 {
			return nil
		}
		return h.handleServicePreview(c, parts[1])
	case "service_buy":
		if len(parts) < 2 {
			return nil
		}
		return h.handleServiceBuy(c, parts[1])
	case "/help":
		return h.handleHelp(c)
	case "/pays":
		return h.handlePays(c)
	case "/show_mz_keys":
		serviceIDStr := parts[1]
		return h.handleShowMZ(c, serviceIDStr)
	default:
		return c.Respond(&telebot.CallbackResponse{
			Text: "Неизвестная команда",
		})
	}
}
