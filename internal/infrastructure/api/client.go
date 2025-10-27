package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type APIClient struct {
	ServerURL  string
	SessionID  string
	sessionMu  sync.Mutex
	HTTPClient *http.Client
	config     *config.Config
}

func NewAPIClient(cfg *config.Config) *APIClient {
	jar, _ := cookiejar.New(nil)
	return &APIClient{
		ServerURL: cfg.API.BaseURL,
		HTTPClient: &http.Client{
			Timeout: time.Duration(cfg.API.Timeout) * time.Second,
			Jar:     jar,
		},
		config: cfg,
	}
}

func (c *APIClient) Authenticate() error {
	authData := map[string]string{
		"login":    c.config.API.APILogin,
		"password": c.config.API.APIPass,
	}

	jsonData, err := json.Marshal(authData)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Post(
		fmt.Sprintf("%s/shm/user/auth.cgi", c.ServerURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var authResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return err
	}

	c.sessionMu.Lock()
	c.SessionID = authResp.SessionID
	c.sessionMu.Unlock()

	// Устанавливаем cookie в jar
	url, _ := url.Parse(c.ServerURL)
	cookie := &http.Cookie{
		Name:    "session_id",
		Value:   c.SessionID,
		Path:    "/",
		Expires: time.Now().Add(24 * time.Hour),
	}
	c.HTTPClient.Jar.SetCookies(url, []*http.Cookie{cookie})

	return nil
}

func (c *APIClient) GetUser(chatID int64) (*models.User, error) {

	login := fmt.Sprintf("@%d", chatID)
	// Подготовка данных
	filter := map[string]interface{}{
		"login": login,
	}

	// Сериализация и кодирование
	jsonBytes, _ := json.Marshal(filter)
	encoded := url.QueryEscape(string(jsonBytes))

	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/shm/v1/admin/user?filter=%s", c.ServerURL, encoded),
		nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	type UserResponse struct {
		Data []models.User `json:"data"`
	}

	var users UserResponse
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}

	for _, user := range users.Data {
		if user.Settings.Telegram.ChatID == chatID {
			return &user, nil
		}
	}

	return nil, nil
}

func (c *APIClient) RegisterUser(user models.UserRegistrationRequest) error {
	jsonData, err := json.Marshal(user)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("%s/shm/v1/admin/user", c.ServerURL),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	/*

		// Если сессия устарела (401 Unauthorized или 403 Forbidden)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			log.Println("Сессия устарела, пробуем обновить SessionID...")
			if err := c.Authenticate(); err != nil {
				return fmt.Errorf("не удалось обновить сессию: %v", err)
			}
			// Повторяем запрос с новым SessionID
			return c.RegisterUser(user)
		}


			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			fmt.Printf(string(body))
	*/

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API вернул статус %d", resp.StatusCode)
	}

	return nil
}

func (c *APIClient) StartSessionRefresher() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := c.Authenticate(); err != nil {
			log.Printf("Ошибка обновления SessionID: %v", err)
			continue
		}
		log.Println("SessionID успешно обновлен")
	}
}

func (c *APIClient) GetUserBalance(userID int) (*models.UserBalance, error) {

	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/shm/v1/template/getUserBalance?format=json&uid=%d", c.ServerURL, userID),
		nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userBalance models.UserBalance
	if err := json.Unmarshal(body, &userBalance); err != nil {
		return nil, err
	}

	return &userBalance, nil

}

func (c *APIClient) GetUserServices(userID int) ([]models.UserService, error) {

	// Собираем filter как JSON:
	// {"user_id": <id>, "category": "<cat>"} — category добавляем только если задана
	f := map[string]any{
		"user_id": userID,
	}
	category := c.config.Services.Category
	if category != "" {
		f["category"] = category
	}

	fb, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("marshal filter: %w", err)
	}

	fullURL := fmt.Sprintf("%s/shm/v1/admin/user/service?filter=%s",
		c.ServerURL, url.QueryEscape(string(fb)),
	)

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	// Выполняем GET-запрос
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Парсим ответ
	type ServiceResponse struct {
		Data []models.UserService `json:"data"`
	}

	var result ServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil

}

func (c *APIClient) GetUserService(serviceID string) (*models.UserService, error) {

	// Собираем filter как JSON:
	// {"user_service_id": <serviceID>, "category": "<category>"} — category добавляем, если задана
	f := map[string]any{
		"user_service_id": serviceID,
	}
	fb, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("marshal filter: %w", err)
	}

	fullURL := fmt.Sprintf("%s/shm/v1/admin/user/service?filter=%s",
		c.ServerURL, url.QueryEscape(string(fb)),
	)

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	// Выполняем GET-запрос
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Парсим ответ
	type ServiceResponse struct {
		Data []models.UserService `json:"data"`
	}

	var result ServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) > 0 {
		us := result.Data[0]
		if strings.HasPrefix(us.Category, "vpn-mz-") && us.Status == "ACTIVE" {
			userKey, err := c.GetUserKeyMarzban(us.UserID, us.ServiceID)
			if err != nil {
				return nil, err
			}
			us.KeyMarzban = *userKey
		}
		return &us, nil
	}

	return nil, nil

}

func (c *APIClient) GetUserKeyMarzban(userID int, serviceID int) (*models.UserKeyMarzban, error) {

	// Формируем URL для запроса
	url := fmt.Sprintf("%s/shm/v1/storage/manage/vpn_mrzb_%d?user_id=%d", c.ServerURL, serviceID, userID)

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return nil, err
	}

	// Выполняем GET-запрос
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Парсим ответ

	var result models.UserKeyMarzban
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *APIClient) DownloadUserKey(userID int, serviceID string) ([]byte, error) {

	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/shm/v1/template/uploadDocumentFromStorage?uid=%d&name=vpn%s", c.ServerURL, userID, serviceID),
		nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (c *APIClient) DeleteUserService(userID int, serviceID string) error {

	req, err := http.NewRequest(
		"DELETE",
		fmt.Sprintf("%s/shm/v1/admin/user/service?user_id=%d&user_service_id=%s", c.ServerURL, userID, serviceID),
		nil)

	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Проверяем статус код
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *APIClient) GetServiceByID(serviceID int) (*models.Service, error) {
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/shm/v1/admin/service?service_id=%d&limit=1", c.ServerURL, serviceID),
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	type SvcResp struct {
		Data []models.Service `json:"data"`
	}
	var out SvcResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("service %d not found", serviceID)
	}
	return &out.Data[0], nil
}

func (c *APIClient) GetServices() ([]models.Service, error) {

	// Собираем filter как JSON
	// {"allow_to_order":1, "category":"..."}   // category добавляем только если задана
	f := map[string]any{
		"allow_to_order": 1,
	}
	category := c.config.Services.Category
	if category != "" {
		f["category"] = category
	}
	fb, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("marshal filter: %w", err)
	}

	u := fmt.Sprintf("%s/shm/v1/admin/service?filter=%s",
		c.ServerURL, url.QueryEscape(string(fb)),
	)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	type ServiceResponse struct {
		Data []models.Service `json:"data"`
	}
	var result ServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	sort.Slice(result.Data, func(i, j int) bool {
		return result.Data[i].Period < result.Data[j].Period
	})

	return result.Data, nil
}

func (c *APIClient) ServiceOrder(userID int, serviceID int) (*models.UserService, error) {

	/*
		svc, err := c.GetServiceByID(serviceID)
		if err != nil {
			return nil, err
		}

		months := int(svc.Period)
		if months <= 0 {
			months = 1
		}

		body := map[string]any{
			"user_id":             userID,
			"service_id":          serviceID,
			"check_exists_unpaid": 1,
			"cost":                svc.Cost, // обязательно
			"months":              months,   // обязателен для срока
			"settings":            nil,      // если нужно — передавайте свои
		}
	*/

	// Подготовка данных
	body := map[string]interface{}{
		"service_id":          serviceID,
		"user_id":             userID,
		"check_exists_unpaid": 1,
	}

	// Сериализация и кодирование
	jsonData, _ := json.Marshal(body)

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("%s/shm/v1/admin/service/order", c.ServerURL),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, err
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API вернул статус %d", resp.StatusCode)
	}

	// Парсим ответ
	type ServiceResponse struct {
		Data []models.UserService `json:"data"`
	}

	var result ServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) > 0 {
		return &result.Data[0], nil
	}

	return nil, nil

}

func (c *APIClient) GetUserPays(userID int) ([]models.UserPay, error) {

	// Формируем URL для запроса
	filter := fmt.Sprintf(`{"user_id": %d}`, userID)
	url := fmt.Sprintf("%s/shm/v1/admin/user/pay?filter=%s", c.ServerURL, url.QueryEscape(filter))

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status: %d", resp.StatusCode)
	}

	// Парсим ответ
	type PaysResponse struct {
		Data []models.UserPay `json:"data"`
	}

	var result PaysResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

// HasUserServiceWithdrawals возвращает true, если у пользователя есть хотя бы одно списание по услуге.
func (c *APIClient) HasUserServiceWithdrawals(userID int, serviceID int) (bool, error) {
	// Собираем filter={"user_id":19,"service_id":8} как query string
	filter := struct {
		UserID    int `json:"user_id"`
		ServiceID int `json:"service_id"`
	}{
		UserID: userID, ServiceID: serviceID,
	}
	fb, err := json.Marshal(filter)
	if err != nil {
		return false, fmt.Errorf("marshal filter: %w", err)
	}

	q := url.Values{}
	q.Set("filter", string(fb))

	// Пример: /shm/v1/admin/user/service/withdraw?filter={...}
	endpoint := "/shm/v1/admin/user/service/withdraw"
	fullURL := c.ServerURL + endpoint + "?" + q.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)

	if err != nil {
		return false, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("API returned status: %d", resp.StatusCode)
	}

	// Парсим ответ
	type WithdrawalsResponse struct {
		Data []models.WithdrawItem `json:"data"`
	}

	var result WithdrawalsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode: %w", err)
	}

	return len(result.Data) > 0, nil
}
