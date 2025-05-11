package service

import (
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type Service struct {
	apiClient *api.APIClient
}

func NewService(apiClient *api.APIClient) *Service {
	return &Service{
		apiClient: apiClient,
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
