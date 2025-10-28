package service

import (
	"bytes"
	"errors"
	"image/png"
	"strconv"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

var (
	ErrUserNotFound = errors.New("user not found")
)

type Service struct {
	apiClient          *api.APIClient
	trialTakenCache    map[int64]bool
	trialCacheMu       sync.RWMutex
	trialEligibleUntil map[int64]time.Time
	trialMu            sync.RWMutex
}

func NewService(apiClient *api.APIClient) *Service {
	return &Service{
		apiClient:          apiClient,
		trialTakenCache:    make(map[int64]bool),
		trialEligibleUntil: make(map[int64]time.Time),
	}
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
	return s.apiClient.GetUser(chatID)
}

func (s *Service) RegisterUser(user models.UserRegistrationRequest) error {
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

func (s *Service) GetUserService(serviceID string) (*models.UserService, error) {

	return s.apiClient.GetUserService(serviceID)

}

func (s *Service) DownloadUserKey(userID int64, serviceID string) ([]byte, error) {

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}

	return s.apiClient.DownloadUserKey(user.ID, serviceID)
}

func (s *Service) GetQRCodeUserKey(userID int64, serviceID string) ([]byte, error) {

	fileBytes, err := s.DownloadUserKey(userID, serviceID)

	if err != nil {
		return nil, err
	}

	return GenerateQRCode(string(fileBytes))

}

func (s *Service) GetUserKeyMarzban(userID int64, serviceID string) (*models.UserKeyMarzban, error) {

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}

	// Преобразование строки в int
	srvID, err := strconv.Atoi(serviceID)
	if err != nil {
		return nil, err
	}

	return s.apiClient.GetUserKeyMarzban(user.ID, srvID)

}

func (s *Service) DeleteUserService(userID int64, serviceID string) error {

	user, err := s.GetUser(userID)
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
