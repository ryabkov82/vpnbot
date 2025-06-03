package bot

import (
	"bytes"
	"errors"
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

	msg := "Для начала работы с Telegram ботом, пожалуйста, зарегистрируйтесь"
	/*
		msg := fmt.Sprintf(
			"Для работы с Telegram ботом укажите _Telegram логин_ в профиле личного кабинета.\n\n"+
				"*Telegram логин*: %s\n\n"+
				"*Кабинет пользователя*: %s",
			username,
			s.config.Cli.URL,
		)
	*/

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

	if c.Message() != nil {
		if err := c.Bot().Delete(c.Message()); err != nil {
			log.Printf("Ошибка удаления сообщения: %v", err)
		}
	}

	/*
		if c.Callback() != nil {
			// Для callback-запросов
			if err := c.Bot().Delete(c.Callback().Message); err != nil {
				log.Printf("Delete callback message error: %v", err)
			}
		}


			return c.Send(
				"Все кнопки удалены",
				&telebot.SendOptions{
					ReplyMarkup: &telebot.ReplyMarkup{
						RemoveKeyboard: true, // Удаляем Reply-клавиатуру
						InlineKeyboard: nil,  // Удаляем инлайн-кнопки
					},
				},
			)
	*/
	/*
		// 1. Создаем Reply-клавиатуру (кнопки под полем ввода)
		replyMarkup := &telebot.ReplyMarkup{
			ResizeKeyboard:  true,
			OneTimeKeyboard: false,
			Selective:       true, // Важно для корректной работы
		}
		btnMenu := replyMarkup.Text("📋 Меню")
		replyMarkup.Reply(replyMarkup.Row(btnMenu))

		err := c.Send("Меню",
			&telebot.SendOptions{
				ParseMode:   "HTML",
				ReplyMarkup: replyMarkup, // Reply-клавиатура
			})

		if err != nil {
			return err
		}
	*/

	msg := "Создавайте и управляйте своими VPN ключами"

	// 2. Создаем инлайн-меню (кнопки внутри сообщения)
	inlineMenu := &telebot.ReplyMarkup{}
	btnBalance := inlineMenu.Data("💰 Баланс", "/balance")
	btnKeys := inlineMenu.Data("🗝 Список VPN ключей", "/list")
	btnHelp := inlineMenu.Data("🗓 Помощь", "/help")
	btnSupport := inlineMenu.URL("🛟 Поддержка", s.config.Telegram.SupportChat)

	inlineMenu.Inline(
		inlineMenu.Row(btnBalance),
		inlineMenu.Row(btnKeys),
		inlineMenu.Row(btnHelp),
		inlineMenu.Row(btnSupport),
	)

	return c.Send(msg, inlineMenu)

}

func (s *Service) handleBalance(c telebot.Context) error {

	if c.Callback() != nil {
		// Для callback-запросов
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	}

	userBalance, err := s.service.GetUserBalance(c.Chat().ID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Println("Ошибка проверки баланса пользователя:", err)
		return c.Send("Ошибка системы, попробуйте позже")
	}

	menu := &telebot.ReplyMarkup{}
	btnPay := menu.WebApp("✚ Пополнить баланс", &telebot.WebApp{
		URL: fmt.Sprintf("%s/shm/v1/public/tg_payments_webapp?format=html&user_id=%s&profile=telegram_bot", s.config.API.BaseURL, userBalance.ID),
	})

	btnPays := menu.Data("☰ История платежей", "/pays")

	btnBack := menu.Data("⇦ Назад", "/menu")

	menu.Inline(
		menu.Row(btnPay),
		menu.Row(btnPays),
		menu.Row(btnBack),
	)

	msg := fmt.Sprintf("💰 *Баланс*: %.2f\n\nНеобходимо оплатить: *%.2f*", userBalance.Balance, userBalance.Forecast)

	return c.Send(
		msg,
		menu,
		telebot.ModeMarkdown,
	)
}

func (s *Service) handleList(c telebot.Context) error {

	if c.Callback() != nil {
		// Для callback-запросов
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	}

	services, err := s.service.GetUserServices(c.Chat().ID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Printf("Ошибка при получении списка услуг: %v", err)
		return c.Send("⚠️ Произошла ошибка при получении списка услуг")
	}

	// Форматируем вывод
	//if len(services) == 0 {
	//	return c.Send("У вас нет активных услуг")
	//}

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

	return c.Send("🗝 Ваши ключи:", menu)
}

func (s *Service) handlePricelist(c telebot.Context) error {

	if c.Callback() != nil {
		// Для callback-запросов
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	} else {
		// если это команда, то проверим, что пользователь зарегистрирован
		user, err := s.service.GetUser(c.Chat().ID)
		if err != nil {
			log.Printf("Не удалось загрузить список услуг: %v", err)
			return c.Send("⚠️ Не удалось загрузить список услуг. Попробуйте позже.")
		}
		if user == nil {
			return s.showRegistrationMenu(c)
		}
	}

	menu := &telebot.ReplyMarkup{}
	btnBack := menu.Data("⇦ Назад", "/menu")

	services, err := s.service.GetServices()

	if err != nil {
		log.Printf("Не удалось загрузить список услуг: %v", err)
		return c.Send("⚠️ Не удалось загрузить список услуг. Попробуйте позже.")
	}

	var rows []telebot.Row
	for _, s := range services {
		// Форматируем цену в зависимости от периода
		//price := formatPrice(s.Cost, s.Period)
		rows = append(rows, menu.Row(
			menu.Data(fmt.Sprintf("🛒 %s - %.2f руб.", s.Name, s.Cost), "/serviceorder", fmt.Sprint(s.ServiceID)),
		))
	}
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	msg := "☷ Выберите услугу для заказа:"
	return c.Send(msg, menu)

}

func (s *Service) handleServiceOrder(c telebot.Context, serviceID string) error {

	_, err := s.service.ServiceOrder(c.Chat().ID, serviceID)

	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Printf("Ошибка при заказе услуги: %v", err)
		return c.Send("⚠️ Произошла ошибка при заказе услуги")
	}

	return s.handleList(c)

}

func (s *Service) handleService(c telebot.Context, serviceID string) error {

	if c.Callback() != nil {
		// Для callback-запросов
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	}

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
		if strings.HasPrefix(us.Category, "vpn-mz-") {

			rows = append(rows, menu.Row(
				menu.WebApp("Показать данные для подключения", &telebot.WebApp{
					URL: fmt.Sprintf("%s?telegram=true", us.KeyMarzban.SubscriptionURL),
				}),
				menu.Data("Показать ссылку подписки", "/show_mz_keys", fmt.Sprint(us.ServiceID)),
			))

		} else {
			rows = append(rows, menu.Row(
				menu.Data("🗝 Скачать ключ", "/download_qr", fmt.Sprint(us.ServiceID)),
				menu.Data("👀 Показать QR код", "/show_qr", fmt.Sprint(us.ServiceID)),
			))
		}
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

	return c.Send(msg, &telebot.SendOptions{
		ParseMode:   telebot.ModeHTML,
		ReplyMarkup: menu,
	})
}

func (s *Service) handleDownloadUserKey(c telebot.Context, serviceID string) error {

	fileBytes, err := s.service.DownloadUserKey(c.Chat().ID, serviceID)
	if err != nil {
		log.Printf("Ошибка загрузки файла ключа: %v", err)
		return c.Send("⚠️ Ошибка загрузки файла ключа")
	}

	file := &telebot.Document{
		File:     telebot.FromReader(bytes.NewReader(fileBytes)),
		FileName: fmt.Sprintf("vpn%s.conf", serviceID), // Укажите нужное имя файла
		MIME:     "text/plain; charset=utf-8",          // Укажите правильный MIME-тип
	}

	return c.Send(file)

}

func (s *Service) handleShowMZ(c telebot.Context, serviceID string) error {

	userKey, err := s.service.GetUserKeyMarzban(c.Chat().ID, serviceID)
	if err != nil {
		log.Printf("Ошибка при получении информации по услуге: %v", err)
		return c.Send("⚠️ Произошла ошибка при получении информации по услуге")
	}

	qrBytes, err := service.GenerateQRCode(userKey.SubscriptionURL)

	if err != nil {
		log.Printf("Ошибка генерации QR-кода: %v", err)
		return c.Send("⚠️ Не удалось создать QR-код")
	}

	// Отправляем как изображение
	photo := &telebot.Photo{
		File:    telebot.FromReader(bytes.NewReader(qrBytes)),
		Caption: fmt.Sprintf("<b>Subscription URL:</b>\n<code>%s</code>", userKey.SubscriptionURL),
	}

	err = c.Send(photo, &telebot.SendOptions{
		ParseMode: telebot.ModeHTML,
	})

	if err != nil {
		return err
	}

	link := userKey.Links[0]

	qrBytes, err = service.GenerateQRCode(userKey.SubscriptionURL)
	if err != nil {
		log.Printf("Ошибка генерации QR-кода: %v", err)
		return c.Send("⚠️ Не удалось создать QR-код")
	}

	caption := ""
	if strings.HasPrefix(link, "ss") {
		caption = fmt.Sprintf("<b>ShadowSocks:</b>\n<code>%s</code>", link)
	} else {
		caption = fmt.Sprintf("<b>VLESS TCP:</b>\n<code>%s</code>", link)
	}

	photo = &telebot.Photo{
		File:    telebot.FromReader(bytes.NewReader(qrBytes)),
		Caption: caption,
	}

	return c.Send(photo, &telebot.SendOptions{
		ParseMode: telebot.ModeHTML,
	})

}

func (s *Service) handleShowQR(c telebot.Context, serviceID string) error {

	qrBytes, err := s.service.GetQRCodeUserKey(c.Chat().ID, serviceID)
	if err != nil {
		log.Printf("Ошибка генерации QR-кода: %v", err)
		return c.Send("⚠️ Не удалось создать QR-код")
	}

	// Отправляем как изображение
	photo := &telebot.Photo{
		File:    telebot.FromReader(bytes.NewReader(qrBytes)),
		Caption: "Ваш QR-код",
	}

	return c.Send(photo)

}

func (s *Service) handleDelete(c telebot.Context, serviceID string) error {

	if c.Callback() != nil {
		// Для callback-запросов
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	}

	// Создаем inline-клавиатуру
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	rows = append(rows, menu.Row(
		menu.Data("🧨 ДА, УДАЛИТЬ! 🔥", "/delete_confirmed", serviceID),
	))

	// Кнопка "Назад"
	rows = append(rows, menu.Row(
		menu.Data("⇦ Назад", "/list", ""),
	))

	menu.Inline(rows...)

	msg := "🤔 <b>Подтвердите удаление услуги. Услугу нельзя будет восстановить!</b>"

	return c.Send(msg, &telebot.SendOptions{
		ParseMode:   telebot.ModeHTML,
		ReplyMarkup: menu,
	})
}

func (s *Service) handleDeleteConfirmed(c telebot.Context, serviceID string) error {

	err := s.service.DeleteUserService(c.Chat().ID, serviceID)
	if err != nil {
		log.Printf("Ошибка при удалении услуги: %v", err)
		return c.Send("⚠️ Ошибка при удалении услуги")
	}

	// 3. Удаляем сообщение с подтверждением
	if err := c.Delete(); err != nil {
		log.Printf("Error deleting confirmation message: %v", err)
	}

	// 4. Открываем список услуг
	return s.handleList(c)

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

func (s *Service) handleHelp(c telebot.Context) error {

	if c.Callback() != nil {
		// Для callback-запросов
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	} else {
		// если это команда, то проверим, что пользователь зарегистрирован
		user, err := s.service.GetUser(c.Chat().ID)
		if err != nil {
			log.Printf("Ошибка получения информации о пользователе: %v", err)
			return c.Send("⚠️ Ошибка получения информации о пользователе. Попробуйте позже.")
		}
		if user == nil {
			return s.showRegistrationMenu(c)
		}
	}

	// Создаем кнопки для inline клавиатуры
	supportBtn := telebot.InlineButton{
		Text: "Чат поддержки",
		URL:  s.config.Telegram.SupportChat,
	}

	backBtn := telebot.InlineButton{
		Text: "⇦ Назад",
		Data: "/menu",
	}

	// Создаем inline клавиатуру
	inlineKeys := [][]telebot.InlineButton{
		{supportBtn},
		{backBtn},
	}

	// Формируем текст с HTML разметкой
	//caption := `1️⃣ Скачайте и установите приложение WireGuard к себе на устройство. Скачать для <a href="https://apps.apple.com/us/app/wireguard/id1441195209">iPhone</a>, <a href="https://play.google.com/store/apps/details?id=com.wireguard.android">Android</a>, <a href="https://apps.apple.com/us/app/wireguard/id1451685025">Mac</a>.
	caption := `1️⃣ В разделе <b>"Список VPN ключей"</b> закажите новый ключ, выбрав подходящий тариф.

2️⃣ После оплаты (пункт меню <b>"Баланс" - "✚ Пополнить баланс"</b>) в том же разделе выберите созданный ключ и нажмите <b>"Показать данные для подключения"</b>.

3️⃣ Следуйте инструкциям в открывшемся окне.
`
	// Отправляем фото с подписью и клавиатурой
	err := c.Send(
		caption,
		//&telebot.Photo{
		//	//	File:    telebot.FromURL("https://media.tenor.com/5KHjsG1Aw1YAAAAi/photos-google-photos.gif"),
		//	Caption: caption,
		//},
		&telebot.SendOptions{
			ParseMode: telebot.ModeHTML, // В v3+ может потребоваться просто "HTML"
			//Protected: true,             // В v3+ protect_content заменен на Protected
			ReplyMarkup: &telebot.ReplyMarkup{
				InlineKeyboard: inlineKeys,
			},
		},
	)

	return err
}

func (s *Service) handlePays(c telebot.Context) error {

	if c.Callback() != nil {
		// Для callback-запросов
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	}

	// Получаем ID пользователя из контекста
	userID := c.Sender().ID

	// Делаем запрос к API для получения платежей
	pays, err := s.service.GetUserPays(userID)
	if err != nil {
		log.Printf("Не удалось получить данные о платежах: %v", err)
		return c.Send("⚠️ Не удалось получить данные о платежах")
	}

	// Создаем inline клавиатуру
	var inlineKeys [][]telebot.InlineButton

	// Добавляем кнопки для каждого платежа
	for _, pay := range pays {
		btn := telebot.InlineButton{
			Text: fmt.Sprintf("Дата: %s, Сумма: %d руб.", pay.Date, pay.Money),
			Data: "/menu", // В v3+ используется Data вместо CallbackData
		}
		inlineKeys = append(inlineKeys, []telebot.InlineButton{btn})
	}

	// Добавляем кнопку "Назад"
	backBtn := telebot.InlineButton{
		Text: "⇦ Назад",
		Data: "/menu",
	}
	inlineKeys = append(inlineKeys, []telebot.InlineButton{backBtn})

	// Отправляем сообщение с клавиатурой
	return c.Send(
		"Платежи",
		&telebot.SendOptions{
			ReplyMarkup: &telebot.ReplyMarkup{
				InlineKeyboard: inlineKeys,
			},
		},
	)

}

func generatePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 12)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func formatPrice(cost int, period int) string {
	if period == 1 {
		return fmt.Sprintf("%d руб./мес", cost)
	} else if period == 12 {
		return fmt.Sprintf("%d руб./год", cost)
	}
	return fmt.Sprintf("%d$/%d мес", cost, period)
}
