package service

import (
	"bytes"
	"image/png"
	"strconv"
	"sync"

	"github.com/skip2/go-qrcode"

	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type Service struct {
	apiClient *api.APIClient
	paysCache map[int64][]models.UserPay // userID -> pays
	cacheMux  sync.Mutex
}

func NewService(apiClient *api.APIClient) *Service {
	return &Service{
		apiClient: apiClient,
		paysCache: make(map[int64][]models.UserPay),
	}
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

	return s.apiClient.GetUserBalance(user.ID)
}

func (s *Service) GetUserServices(userID int64) ([]models.UserService, error) {

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
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

	return generateQRCode(string(fileBytes))

}

func (s *Service) DeleteUserService(userID int64, serviceID string) error {

	user, err := s.GetUser(userID)
	if err != nil {
		return err
	}

	return s.apiClient.DeleteUserService(user.ID, serviceID)

}

// generateQRCode создает QR-код из текста и возвращает PNG в виде []byte
func generateQRCode(text string) ([]byte, error) {
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

	srvID, err := strconv.Atoi(serviceID)
	if err != nil {
		return nil, err
	}

	return s.apiClient.ServiceOrder(user.ID, srvID)

}

func (s *Service) GetUserPays(userID int64) ([]models.UserPay, error) {

	s.cacheMux.Lock()
	defer s.cacheMux.Unlock()

	if cached, exists := s.paysCache[userID]; exists {
		return cached, nil
	}

	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}

	pays, err := s.apiClient.GetUserPays(user.ID)

	if err != nil {
		return pays, err
	}

	s.paysCache[userID] = pays

	return pays, err
}
