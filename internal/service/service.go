package service

import (
	"bytes"
	"errors"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

var (
	ErrUserNotFound = errors.New("user not found")
)

type Service struct {
	apiClient          *api.APIClient
	brand              config.BrandConfig
	trialTakenCache    map[int64]bool
	trialCacheMu       sync.RWMutex
	trialEligibleUntil map[int64]time.Time
	trialMu            sync.RWMutex
}

// NewService создаёт use-case слой с активным брендом процесса (web-login prefix и source).
// runtime передаёт BrandConfig уже после Config.Normalize; service layer не синтезирует
// brand defaults — пустые поля остаются пустыми.
func NewService(apiClient *api.APIClient, brand config.BrandConfig) *Service {
	return &Service{
		apiClient:          apiClient,
		brand:              effectiveServiceBrand(brand),
		trialTakenCache:    make(map[int64]bool),
		trialEligibleUntil: make(map[int64]time.Time),
	}
}

// effectiveServiceBrand только нормализует поля (trim и т.п.); defaults не добавляет.
func effectiveServiceBrand(brand config.BrandConfig) config.BrandConfig {
	cfg := &config.Config{Brand: brand}
	return cfg.EffectiveBrand()
}

func (s *Service) webLoginPrefix() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.brand.WebUserLoginPrefix)
}

func (s *Service) webUserSource() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.brand.WebUserSource)
}

// --- внутренние хелперы для кэша (private) ---
func (s *Service) getTrialTakenCached(chatID int64) (bool, bool) {
	s.trialCacheMu.RLock()
	v, ok := s.trialTakenCache[chatID]
	s.trialCacheMu.RUnlock()
	return v, ok
}

func (s *Service) setTrialTakenCached(chatID int64) {
	s.trialCacheMu.Lock()
	s.trialTakenCache[chatID] = true
	s.trialCacheMu.Unlock()
}

func (s *Service) SetTrialEligible(chatID int64, until time.Time) {
	s.trialMu.Lock()
	defer s.trialMu.Unlock()
	s.trialEligibleUntil[chatID] = until
}

func (s *Service) IsTrialEligible(chatID int64) bool {
	s.trialMu.RLock()
	defer s.trialMu.RUnlock()
	until, ok := s.trialEligibleUntil[chatID]
	return ok && time.Now().Before(until)
}

func (s *Service) GetUser(chatID int64) (*models.User, error) {
	if chatID <= 0 {
		return nil, errors.New("invalid telegram chat id")
	}
	brandID := s.activeBrandID()
	if brandID == "" {
		return nil, errors.New("active brand id is required")
	}
	login := telegramSHMLogin(brandID, chatID)
	user, err := s.apiClient.GetUserByLogin(login)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	if user.Settings.Telegram.ChatID != chatID {
		logIdentityMismatch("telegram_chat_id", user.Login, strings.TrimSpace(user.Settings.BrandID), chatID, user.Settings.Telegram.ChatID)
		return nil, ErrUserIdentityMismatch
	}
	if !userBelongsToBrand(user, brandID, login) {
		logIdentityMismatch("brand_id", user.Login, strings.TrimSpace(user.Settings.BrandID), chatID, user.Settings.Telegram.ChatID)
		return nil, ErrUserIdentityMismatch
	}
	return user, nil
}

// GetUserByID — пользователь по числовому shm user_id (веб-кабинет, premium-токены).
func (s *Service) GetUserByID(userID int) (*models.User, error) {
	return s.apiClient.GetUserByID(userID)
}

func (s *Service) RegisterUser(user models.UserRegistrationRequest) error {
	brandID := s.activeBrandID()
	if brandID == "" {
		return errors.New("active brand id is required")
	}
	chatID := user.Settings.Telegram.ChatID
	if chatID <= 0 {
		return errors.New("telegram chat_id must be positive")
	}
	// Канонические login и brand_id всегда задаёт service layer активного процесса.
	user.Login = telegramSHMLogin(brandID, chatID)
	user.Settings.BrandID = brandID
	return s.apiClient.RegisterUser(user)
}

func (s *Service) GetUserBalance(userID int64) (*models.UserBalance, error) {

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	return s.apiClient.GetUserBalance(user.ID)
}

func (s *Service) GetUserServices(userID int64) ([]models.UserService, error) {

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	return s.apiClient.GetUserServices(user.ID)

}

func (s *Service) GetUserByLogin(login string) (*models.User, error) {
	return s.apiClient.GetUserByLogin(login)
}

func (s *Service) GetUserByLogin2(login2 string) (*models.User, error) {
	return s.apiClient.GetUserByLogin2(login2)
}

// GetUserServicesByUserID возвращает услуги по числовому SHM user_id (без привязки к Telegram chat id).
func (s *Service) GetUserServicesByUserID(userID int) ([]models.UserService, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	return s.apiClient.GetUserServices(userID)
}

// GetUserBalanceByUserID — баланс по SHM user_id (личный кабинет без Telegram).
func (s *Service) GetUserBalanceByUserID(userID int) (*models.UserBalance, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	return s.apiClient.GetUserBalance(userID)
}

func (s *Service) DownloadUserKey(telegramChatID int64, serviceID string) ([]byte, error) {
	us, user, err := s.GetOwnedUserServiceByTelegramID(telegramChatID, serviceID)
	if err != nil {
		return nil, err
	}
	_ = us
	return s.apiClient.DownloadUserKey(user.ID, serviceID)
}

func (s *Service) GetQRCodeUserKey(telegramChatID int64, serviceID string) ([]byte, error) {
	fileBytes, err := s.DownloadUserKey(telegramChatID, serviceID)
	if err != nil {
		return nil, err
	}
	return GenerateQRCode(string(fileBytes))
}

func (s *Service) GetUserKeyMarzban(telegramChatID int64, serviceID string) (*models.UserKeyMarzban, error) {
	us, user, err := s.GetOwnedUserServiceByTelegramID(telegramChatID, serviceID)
	if err != nil {
		return nil, err
	}
	if us.KeyMarzban.SubscriptionURL != "" || len(us.KeyMarzban.Links) > 0 {
		k := us.KeyMarzban
		return &k, nil
	}
	return s.apiClient.GetUserKeyMarzban(user.ID, us.ServiceID)
}

func (s *Service) DeleteUserService(telegramChatID int64, serviceID string) error {
	_, user, err := s.GetOwnedUserServiceByTelegramID(telegramChatID, serviceID)
	if err != nil {
		return err
	}
	return s.apiClient.DeleteUserService(user.ID, serviceID)
}

// generateQRCode создает QR-код из текста и возвращает PNG в виде []byte
func GenerateQRCode(text string) ([]byte, error) {
	// Генерируем QR-код с высоким уровнем коррекции ошибок (High)
	qr, err := qrcode.New(text, qrcode.High)
	if err != nil {
		return nil, err
	}

	// Получаем PNG-изображение
	var buf bytes.Buffer
	err = png.Encode(&buf, qr.Image(256)) // 256 - размер в пикселях
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *Service) GetServices() ([]models.Service, error) {

	return s.apiClient.GetServices()

}

func (s *Service) ServiceOrder(userID int64, serviceID string) (*models.UserService, error) {

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, ErrUserNotFound
	}

	srvID, err := strconv.Atoi(serviceID)
	if err != nil {
		return nil, err
	}

	return s.apiClient.ServiceOrder(user.ID, srvID)

}

// ServiceOrderByUserID создаёт заказ услуги по числовому user_id (SHM).
func (s *Service) ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	if serviceID <= 0 {
		return nil, errors.New("invalid service id")
	}
	return s.apiClient.ServiceOrder(userID, serviceID)
}

// DeleteUserServiceByUserID удаляет user_service по числовому user_id (личный кабинет)
// только после централизованной ownership-проверки.
func (s *Service) DeleteUserServiceByUserID(userID int, userServiceID string) error {
	if _, err := s.GetOwnedUserServiceByUserID(userID, userServiceID); err != nil {
		return err
	}
	return s.apiClient.DeleteUserService(userID, strings.TrimSpace(userServiceID))
}

func (s *Service) GetUserPays(userID int64) ([]models.UserPay, error) {

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}

	pays, err := s.apiClient.GetUserPays(user.ID)

	if err != nil {
		return pays, err
	}

	return pays, err
}

// GetUserPaysByUserID — сырой список платежей по SHM user_id (личный кабинет без Telegram chat id).
func (s *Service) GetUserPaysByUserID(userID int) ([]models.UserPay, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	return s.apiClient.GetUserPays(userID)
}

// UserHasTrialService возвращает true, если у пользователя уже было СПИСАНИЕ по тестовой услуге.
// Теперь мы считаем “брал тест” по факту withdraw, а не просто наличию UserService.
func (s *Service) UserHasTrialService(chatID int64, baseServiceID int) (bool, error) {
	// 1️ Проверяем кэш
	if v, ok := s.getTrialTakenCached(chatID); ok && v {
		return true, nil
	}

	// 2️ Проверяем по API
	user, err := s.GetUser(chatID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, ErrUserNotFound
	}

	has, err := s.apiClient.HasUserServiceWithdrawals(user.ID, baseServiceID)
	if err != nil {
		return false, err
	}

	// 3️ Если найдено — добавляем в кэш
	if has {
		s.setTrialTakenCached(chatID)
	}

	return has, nil
}

func (s *Service) GetServiceByID(serviceID int) (*models.Service, error) {
	return s.apiClient.GetServiceByID(serviceID)
}
