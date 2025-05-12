package bot

import (
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/service"

	"gopkg.in/telebot.v3"
)

// Service содержит бизнес-логику обработки команд
type Service struct {
	service *service.Service
	config  *config.Config
}

func NewService(service *service.Service, cfg *config.Config) *Service {
	return &Service{
		service: service,
		config:  cfg,
	}
}

func (s *Service) handleStart(c telebot.Context) error {

	user, err := s.service.GetUser(c.Chat().ID)
	if err != nil {
		log.Println("Ошибка проверки пользователя:", err)
		return c.Send("Ошибка системы, попробуйте позже")
	}

	if user == nil {
		return s.showRegistrationMenu(c)
	}

	return s.showMainMenu(c)
}

func (s *Service) handleMenu(c telebot.Context) error {
	return s.showMainMenu(c)
}

// showRegistrationMenu показывает меню регистрации
func (s *Service) showRegistrationMenu(c telebot.Context) error {
	menu := &telebot.ReplyMarkup{}
	btnRegister := menu.Data("Регистрация ✍", "/register")

	username := c.Sender().Username
	if username == "" {
		username = "не указан"
	}

	msg := fmt.Sprintf(
		"Для работы с Telegram ботом укажите _Telegram логин_ в профиле личного кабинета.\n\n"+
			"*Telegram логин*: %s\n\n"+
			"*Кабинет пользователя*: %s",
		username,
		s.config.Cli.URL,
	)

	menu.Inline(menu.Row(btnRegister))
	err := c.Send(msg, menu, telebot.ModeMarkdown)
	if err != nil {
		if strings.Contains(err.Error(), "can't parse entities") {
			// Пробуем экранировать проблемные символы
			safeText := strings.ReplaceAll(msg, "*", "\\*")
			safeText = strings.ReplaceAll(safeText, "_", "\\_")
			safeText = strings.ReplaceAll(safeText, "[", "\\[")

			// Отправляем безопасную версию
			return c.Send(safeText, menu, telebot.ModeMarkdown)
		}
		return err
	}
	return nil
}

// showMainMenu показывает основное меню
func (s *Service) showMainMenu(c telebot.Context) error {

	menu := &telebot.ReplyMarkup{}
	btnBalance := menu.Data("💰 Баланс", "/balance")
	btnKeys := menu.Data("🗝 Список VPN ключей", "/list")
	btnSupport := menu.URL("🛟 Поддержка", s.config.Telegram.SupportChat)

	menu.Inline(
		menu.Row(btnBalance),
		menu.Row(btnKeys),
		menu.Row(btnSupport),
	)

	msg := "Создавайте и управляйте своими VPN ключами"
	if c.Callback() != nil {
		err := c.Edit(
			msg,
			menu,
		)
		if err == nil {
			return nil
		}
	}

	return c.Send(msg, menu)
}

func (s *Service) handleBalance(c telebot.Context) error {

	userBalance, err := s.service.GetUserBalance(c.Chat().ID)
	if err != nil {
		log.Println("Ошибка проверки баланса пользователя:", err)
		return c.Send("Ошибка системы, попробуйте позже")
	}

	menu := &telebot.ReplyMarkup{}
	btnPay := menu.WebApp("✚ Пополнить баланс", &telebot.WebApp{
		URL: fmt.Sprintf("%s/shm/v1/public/tg_payments_webapp?format=html&user_id=%s&profile=telegram_test_bot", s.config.API.BaseURL, userBalance.ID),
	})
	btnBack := menu.Data("⇦ Назад", "/menu")

	menu.Inline(
		menu.Row(btnPay),
		menu.Row(btnBack),
	)

	msg := fmt.Sprintf("💰 *Баланс*: %.2f\n\nНеобходимо оплатить: *%.2f*", userBalance.Balance, userBalance.Forecast)
	if c.Callback() != nil {
		err := c.Edit(
			msg,
			menu,
			telebot.ModeMarkdown,
		)
		if err == nil {
			return nil
		}
	}

	return c.Send(
		msg,
		menu,
		telebot.ModeMarkdown,
	)
}

func (s *Service) handleList(c telebot.Context) error {

	services, err := s.service.GetUserServices(c.Chat().ID)
	if err != nil {
		log.Printf("Ошибка при получении списка услуг: %v", err)
		return c.Send("⚠️ Произошла ошибка при получении списка услуг")
	}

	// Форматируем вывод
	if len(services) == 0 {
		return c.Send("У вас нет активных услуг")
	}

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	for _, s := range services {
		var status string
		switch s.Status {
		case "ACTIVE":
			status = "✅"
		case "BLOCK":
			status = "❌"
		default:
			status = "⏳"
		}

		rows = append(rows, menu.Row(
			menu.Data(fmt.Sprintf("%s - %s", status, s.Name), "/service", fmt.Sprint(s.ServiceID)),
		))
	}

	rows = append(rows,
		menu.Row(menu.Data("🛒 Новый ключ", "/pricelist")),
		menu.Row(menu.Data("⇦ Назад", "/menu")),
	)

	menu.Inline(rows...)

	if c.Callback() != nil {
		err := c.Edit("🗝 Ваши ключи:", menu)
		if err == nil {
			return nil
		}
	}

	return c.Send("🗝 Ваши ключи:", menu)
}

func (s *Service) handleService(c telebot.Context, serviceID string) error {

	us, err := s.service.GetUserService(serviceID)
	if err != nil {
		log.Printf("Ошибка при получении информации по услуге: %v", err)
		return c.Send("⚠️ Произошла ошибка при получении информации по услуге")
	}

	if us == nil {
		log.Printf("Услуга не найдена: %s", serviceID)
		return c.Send("⚠️ Услуга не найдена")
	}

	// Определяем иконку и статус
	var icon, status string
	switch us.Status {
	case "ACTIVE":
		icon = "✅"
		status = "Работает"
	case "BLOCK":
		icon = "❌"
		status = "Заблокирована"
	case "NOT PAID":
		icon = "💰"
		status = "Ожидает оплаты"
	default:
		icon = "⏳"
		status = "Обработка"
	}

	// Формируем текст сообщения
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>Ключ</b>: %s %s", icon, us.Name))

	if us.Expire != "" {
		text.WriteString(fmt.Sprintf("\n\n<b>Оплачен до</b>: %s",
			us.Expire))
	}

	text.WriteString(fmt.Sprintf("\n\n<b>Статус</b>: %s", status))

	// Создаем inline-клавиатуру
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	// Первый ряд кнопок (для активного ключа)
	if us.Status == "ACTIVE" {
		rows = append(rows, menu.Row(
			menu.Data("🗝 Скачать ключ", "/download_qr", fmt.Sprint(us.ServiceID)),
			menu.Data("👀 Показать QR код", "/show_qr", fmt.Sprint(us.ServiceID)),
		))
	}

	// Второй ряд (для неоплаченных/заблокированных)
	if us.Status == "NOT PAID" || us.Status == "BLOCK" {
		rows = append(rows, menu.Row(
			menu.Data("💰 Оплатить", "/balance", ""),
		))
	}

	// Третий ряд (удаление для всех кроме PROGRESS)
	if us.Status != "PROGRESS" {
		rows = append(rows, menu.Row(
			menu.Data("❌ Удалить ключ", "/delete", fmt.Sprint(us.ServiceID)),
		))
	}

	// Кнопка "Назад"
	rows = append(rows, menu.Row(
		menu.Data("⇦ Назад", "/list", ""),
	))

	menu.Inline(rows...)

	msg := text.String()

	if c.Callback() != nil {
		err := c.Edit(msg, &telebot.SendOptions{
			ParseMode:   telebot.ModeHTML,
			ReplyMarkup: menu,
		})
		if err == nil {
			return nil
		}
	}

	return c.Send(msg, &telebot.SendOptions{
		ParseMode:   telebot.ModeHTML,
		ReplyMarkup: menu,
	})
}

func (s *Service) handleRegister(c telebot.Context) error {

	user := c.Sender()

	login := fmt.Sprintf("@%d", user.ID)
	userID := fmt.Sprintf("%d", user.ID)

	// Подготовка данных для регистрации
	regData := models.UserRegistrationRequest{
		Login:    login,
		Password: generatePassword(), // Функция генерации пароля
		FullName: fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{
				UserID:       userID,
				Username:     user.Username,
				Login:        user.Username,
				FirstName:    user.FirstName,
				LastName:     user.LastName,
				LanguageCode: user.LanguageCode,
				IsPremium:    user.IsPremium,
				ChatID:       user.ID,
				Profile: map[string]interface{}{
					"chat_id": user.ID,
					"status":  "member",
				},
			},
		},
	}

	err := s.service.RegisterUser(regData)
	if err != nil {
		log.Println("Ошибка регистрации:", err)
		return c.Send("⚠️ Ошибка регистрации. Пожалуйста, попробуйте позже.")
	}

	return s.showMainMenu(c)

}

func generatePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 12)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
