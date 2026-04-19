package bot

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/service"

	"gopkg.in/telebot.v3"
)

const defaultLogoURL = "https://vpn-for-friends.com/logobot.jpg"

// Service содержит бизнес-логику обработки команд
type Service struct {
	service *service.Service
	config  *config.Config

	serviceBuyMu       sync.Mutex
	serviceBuyInFlight map[string]struct{}
}

func NewService(service *service.Service, cfg *config.Config) *Service {
	return &Service{
		service:            service,
		config:             cfg,
		serviceBuyInFlight: make(map[string]struct{}),
	}
}

func (s *Service) logoPhoto(caption string) *telebot.Photo {
	url := s.config.Assets.LogoURL
	if url == "" {
		url = defaultLogoURL
	}
	return &telebot.Photo{
		File:    telebot.FromURL(url),
		Caption: caption,
	}
}

func (s *Service) handleStart(c telebot.Context) error {
	// 1) Сначала читаем payload из /start <payload> и выдаём "допуск", если он валиден
	payload := ""
	if c.Message() != nil {
		payload = strings.TrimSpace(c.Message().Payload)
	}

	trialCfg := s.config.Features.Trial
	if trialCfg.Enabled && trialCfg.RequireStartParam && payload != "" {
		allowed := false
		for _, p := range trialCfg.AllowedStartParams {
			if payload == p {
				allowed = true
				break
			}
		}
		if allowed {
			ttl := time.Duration(trialCfg.EligibilityTTLHours)
			if ttl <= 0 {
				ttl = 24 // дефолт 24 часа
			}
			// даём допуск до регистрации, чтобы он сохранился на время онбординга
			s.service.SetTrialEligible(c.Chat().ID, time.Now().Add(ttl*time.Hour))
		}
	}

	// 2) Проверяем пользователя
	user, err := s.service.GetUser(c.Chat().ID)
	if err != nil {
		log.Println("Ошибка проверки пользователя:", err)
		return c.Send("Ошибка системы, попробуйте позже")
	}
	if user == nil {
		// регистрация; после неё при показе меню eligibility уже будет учтён
		return s.showRegistrationMenu(c)
	}

	// 3) Показываем главное меню
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

	msg := "Создавайте и управляйте своими ключами доступа"

	// 2. Создаем инлайн-меню (кнопки внутри сообщения)
	inlineMenu := &telebot.ReplyMarkup{}
	btnBalance := inlineMenu.Data("💰 Баланс", "/balance")
	btnKeys := inlineMenu.Data("🗝 Список ключей доступа", "/list")
	btnHelp := inlineMenu.Data("🗓 Помощь", "/help")
	btnSupport := inlineMenu.URL("🛟 Поддержка", s.config.Telegram.SupportChat)

	// Кнопка «Новости», если задана ссылка
	var btnNews *telebot.Btn
	if s.config.Telegram.NewsChannel != "" {
		b := inlineMenu.URL("📣 Новости", s.config.Telegram.NewsChannel)
		btnNews = &b
	}

	// Компоновка клавиатуры
	var rows []telebot.Row
	rows = append(rows, inlineMenu.Row(btnBalance))
	rows = append(rows, inlineMenu.Row(btnKeys))

	if trialRow, ok, err := s.buildTrialRow(c, inlineMenu); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Printf("Ошибка при формировании кнопки теста (menu): %v", err)
		return c.Send("⚠️ Произошла ошибка при проверке тестового периода. Попробуйте позже.")
	} else if ok {
		rows = append(rows, trialRow)
	}

	rows = append(rows, inlineMenu.Row(btnHelp))
	if btnNews != nil {
		rows = append(rows, inlineMenu.Row(*btnNews))
	}
	rows = append(rows, inlineMenu.Row(btnSupport))
	inlineMenu.Inline(rows...)

	return c.Send(s.logoPhoto(msg),
		inlineMenu)
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

	// профиль оплаты из конфига (дефолт — telegram_bot)
	paymentProfile := s.config.Payments.Profile
	if paymentProfile == "" {
		paymentProfile = "telegram_bot"
	}

	menu := &telebot.ReplyMarkup{}
	btnPay := menu.WebApp("✚ Пополнить баланс", &telebot.WebApp{
		URL: fmt.Sprintf(
			"%s/shm/v1/public/tg_payments_webapp?format=html&user_id=%d&profile=%s",
			s.config.API.BaseURL,
			userBalance.ID,
			url.QueryEscape(paymentProfile),
		),
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
		s.logoPhoto(msg),
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

	return c.Send(s.logoPhoto("🗝 Ваши ключи:"),
		menu)
}

func (s *Service) handlePricelist(c telebot.Context) error {
	if c.Callback() != nil {
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	} else {
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

	// ——— Вставляем тестовую кнопку (если нужна) в начале списка ———
	if trialRow, ok, err := s.buildTrialRow(c, menu); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Printf("Ошибка при формировании кнопки теста (pricelist): %v", err)
		return c.Send("⚠️ Произошла ошибка при проверке тестового периода. Попробуйте позже.")
	} else if ok {
		rows = append(rows, trialRow)
	}

	// ——— Далее все прочие услуги ———
	trialID := s.config.Features.Trial.BaseServiceID
	for _, svc := range services {
		// не дублируем тестовую услугу, если мы её уже добавили через хелпер
		if trialID > 0 && svc.ServiceID == trialID {
			continue
		}

		rows = append(rows, menu.Row(
			menu.Data(fmt.Sprintf("🛒 %s - %.2f руб.", svc.Name, svc.Cost),
				"service_preview", fmt.Sprint(svc.ServiceID)),
		))
	}

	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	msg := "☷ Выберите услугу для заказа:"
	return c.Send(s.logoPhoto(msg), menu)
}

func (s *Service) handleServicePreview(c telebot.Context, serviceID string) error {
	if c.Callback() != nil {
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	}

	user, err := s.service.GetUser(c.Chat().ID)
	if err != nil {
		log.Printf("handleServicePreview: %v", err)
		return c.Send("⚠️ Не удалось загрузить данные. Попробуйте позже.")
	}
	if user == nil {
		return s.showRegistrationMenu(c)
	}

	sid, err := strconv.Atoi(serviceID)
	if err != nil {
		return c.Send("⚠️ Некорректная услуга")
	}

	svc, err := s.service.GetServiceByID(sid)
	if err != nil || svc == nil {
		log.Printf("GetServiceByID %s: %v", serviceID, err)
		return c.Send("⚠️ Услуга не найдена")
	}

	preview := models.BuildServicePreview(svc)
	caption := fmt.Sprintf("%s\n\n%s\n\n💰 Цена: %.0f ₽", preview.Title, preview.Description, preview.Cost)

	menu := &telebot.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("Купить", "service_buy", fmt.Sprint(svc.ServiceID)),
			menu.Data("⇦ Назад", "/pricelist"),
		),
	)

	return c.Send(s.logoPhoto(caption), menu)
}

// handleServiceBuy вызывает существующую логику заказа; защищает от повторного нажатия «Купить».
func (s *Service) handleServiceBuy(c telebot.Context, serviceID string) error {
	key := fmt.Sprintf("%d:%s", c.Chat().ID, serviceID)
	s.serviceBuyMu.Lock()
	if _, busy := s.serviceBuyInFlight[key]; busy {
		s.serviceBuyMu.Unlock()
		return nil
	}
	s.serviceBuyInFlight[key] = struct{}{}
	s.serviceBuyMu.Unlock()
	defer func() {
		s.serviceBuyMu.Lock()
		delete(s.serviceBuyInFlight, key)
		s.serviceBuyMu.Unlock()
	}()

	return s.handleServiceOrder(c, serviceID)
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

func (s *Service) handleTrial(c telebot.Context) error {
	if c.Callback() != nil {
		if err := c.Bot().Delete(c.Callback().Message); err != nil {
			log.Printf("Delete callback message error: %v", err)
		}
	}

	// Проверим регистрацию пользователя
	user, err := s.service.GetUser(c.Chat().ID)
	if err != nil {
		log.Printf("Не удалось проверить пользователя для теста: %v", err)
		return c.Send("⚠️ Не удалось выдать тест. Попробуйте позже.")
	}
	if user == nil {
		return s.showRegistrationMenu(c)
	}

	// Настройки тестовой услуги из конфига
	trialCfg := s.config.Features.Trial
	if !trialCfg.Enabled || trialCfg.BaseServiceID <= 0 {
		return c.Send("⚠️ Тестовая услуга временно недоступна")
	}

	// Если требуется старт с параметром — проверяем допуск
	if trialCfg.RequireStartParam && !s.service.IsTrialEligible(c.Chat().ID) {
		return c.Send("ℹ️ Тест доступен по специальной ссылке приглашения. Откройте бота по промо-ссылке и попробуйте снова.")
	}

	// Уже брал тест? (проверка по списаниям)
	hasTrial, err := s.service.UserHasTrialService(c.Chat().ID, trialCfg.BaseServiceID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Printf("Ошибка при проверке тестовой услуги: %v", err)
		return c.Send("⚠️ Не удалось выдать тест. Попробуйте позже.")
	}
	if hasTrial {
		// Узнаем человекочитаемое имя услуги, если возможно
		if svc, e := s.service.GetServiceByID(trialCfg.BaseServiceID); e == nil && svc != nil && svc.Name != "" {
			return c.Send("ℹ️ Услуга '" + svc.Name + "' уже была заказана ранее")
		}
		return c.Send("ℹ️ Тестовая услуга уже была заказана ранее")
	}

	// Найдём тестовую услугу по ID (через сервисный слой; внутри APIClient — filter allow_to_order=1)
	svc, err := s.service.GetServiceByID(trialCfg.BaseServiceID)
	if err != nil || svc == nil {
		log.Printf("Не удалось получить тестовую услугу %d: %v", trialCfg.BaseServiceID, err)
		return c.Send("⚠️ Тестовая услуга временно недоступна")
	}

	// Оформим заказ тестовой услуги
	testServiceID := strconv.Itoa(svc.ServiceID)
	if _, err := s.service.ServiceOrder(c.Chat().ID, testServiceID); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return s.showRegistrationMenu(c)
		}
		log.Printf("Ошибка при выдаче тестовой услуги: %v", err)
		return c.Send("⚠️ Не удалось выдать тестовую услугу")
	}

	// Покажем список услуг после выдачи
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

	return c.Send(s.logoPhoto(msg),
		&telebot.SendOptions{
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

	// Небольшая пауза, чтобы SHM успел обновить состояние
	time.Sleep(2 * time.Second)

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
	caption := `1️⃣ В разделе <b>"Список ключей доступа"</b> закажите новый ключ, выбрав подходящий тариф.

2️⃣ После оплаты (пункт меню <b>"Баланс" - "✚ Пополнить баланс"</b>) в том же разделе выберите созданный ключ и нажмите <b>"Показать данные для подключения"</b>.

3️⃣ Следуйте инструкциям в открывшемся окне.
`
	// Отправляем фото с подписью и клавиатурой
	err := c.Send(
		s.logoPhoto(caption),
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
		s.logoPhoto("Платежи"),
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

// buildTrialRow решает, нужно ли показывать кнопку теста,
// и если да — возвращает готовую строку для Inline-клавиатуры.
// Возвращает (row, ok, err): ok=false, если кнопку показывать не надо.
func (s *Service) buildTrialRow(c telebot.Context, m *telebot.ReplyMarkup) (telebot.Row, bool, error) {
	trialCfg := s.config.Features.Trial
	if !trialCfg.Enabled || trialCfg.BaseServiceID <= 0 {
		return telebot.Row{}, false, nil
	}

	// 1) Требование deeplink-параметра (если включено)
	if trialCfg.RequireStartParam && !s.service.IsTrialEligible(c.Chat().ID) {
		return telebot.Row{}, false, nil
	}

	// 2) Уже брал тест? (кэшируема внутрь UserHasTrialService)
	hasTrial, err := s.service.UserHasTrialService(c.Chat().ID, trialCfg.BaseServiceID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			// наружу, чтобы вызывающая функция могла показать регистрацию
			return telebot.Row{}, false, service.ErrUserNotFound
		}
		return telebot.Row{}, false, fmt.Errorf("check trial: %w", err)
	}
	if hasTrial {
		return telebot.Row{}, false, nil
	}

	// 3) Получаем услугу (с allow_to_order=1 внутри API)
	svc, err := s.service.GetServiceByID(trialCfg.BaseServiceID)
	if err != nil || svc == nil || svc.Name == "" {
		return telebot.Row{}, false, nil // услуги нет/недоступна — тихо не показываем
	}

	// 4) Готовим кнопку
	btn := m.Data(svc.Name, "/trial")
	return m.Row(btn), true, nil
}
