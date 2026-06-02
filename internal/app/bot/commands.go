package bot

import "gopkg.in/telebot.v3"

// botMenuCommands — команды меню Telegram (VPN for Friends и Friends Connect).
func botMenuCommands() []telebot.Command {
	return []telebot.Command{
		{Text: "/start", Description: "Начало работы с ботом"},
		{Text: "/account", Description: "Личный кабинет (NEW)"},
		{Text: "/balance", Description: "Баланс"},
		{Text: "/list", Description: "Список ключей доступа"},
		{Text: "/pricelist", Description: "Новый ключ"},
		{Text: "/help", Description: "Помощь по использованию бота"},
	}
}
