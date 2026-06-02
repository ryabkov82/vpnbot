package bot

import (
	"errors"
	"log"

	"github.com/ryabkov82/vpnbot/internal/service"

	"gopkg.in/telebot.v3"
)

const (
	webCabinetCommandButtonLabel = "Открыть личный кабинет"
	webCabinetMenuButtonLabel    = "🌐 Личный кабинет"
)

func accountCommandMessage() string {
	return "🌐 Личный кабинет (NEW)\n\n" +
		"В связи с ограничениями и нестабильной работой Telegram мы добавили web-кабинет — " +
		"альтернативный способ управления VPN-услугами через сайт.\n\n" +
		"В личном кабинете можно смотреть услуги, подключать VPN, пополнять баланс, " +
		"покупать новые тарифы и обращаться в поддержку.\n\n" +
		"Если Telegram будет недоступен, вы сможете управлять услугами через web-кабинет.\n\n" +
		"Нажмите кнопку ниже, чтобы открыть личный кабинет."
}

// accountCommandReply — текст и inline URL-кнопка для /account (без отправки в Telegram).
type accountCommandReply struct {
	Message    string
	ButtonText string
	ButtonURL  string
}

func (s *Service) accountCommandReply(chatID int64, shmUserID int) accountCommandReply {
	return accountCommandReply{
		Message:    accountCommandMessage(),
		ButtonText: webCabinetCommandButtonLabel,
		ButtonURL:  s.telegramWebCabinetURL(chatID, shmUserID),
	}
}

func (s *Service) webCabinetMenuButton(m *telebot.ReplyMarkup, chatID int64, shmUserID int) *telebot.Btn {
	cabinetURL := s.telegramWebCabinetURL(chatID, shmUserID)
	if cabinetURL == "" {
		return nil
	}
	b := m.URL(webCabinetMenuButtonLabel, cabinetURL)
	return &b
}

func (s *Service) handleAccount(c telebot.Context) error {
	if c.Callback() != nil {
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	} else if c.Message() != nil {
		if err := c.Bot().Delete(c.Message()); err != nil {
			log.Printf("Ошибка удаления сообщения: %v", err)
		}
	}

	user, err := s.service.GetUser(c.Chat().ID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Printf("account command: get user %v", err)
		return c.Send("⚠️ Ошибка получения информации о пользователе. Попробуйте позже.")
	}
	if user == nil {
		return s.showRegistrationMenu(c)
	}

	reply := s.accountCommandReply(c.Chat().ID, user.ID)
	menu := &telebot.ReplyMarkup{}
	if reply.ButtonURL != "" {
		btn := menu.URL(reply.ButtonText, reply.ButtonURL)
		menu.Inline(menu.Row(btn))
		return c.Send(reply.Message, menu)
	}
	return c.Send(reply.Message + "\n\n⚠️ Ссылка на кабинет временно недоступна. Попробуйте позже.")
}
