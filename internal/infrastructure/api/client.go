package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
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

// GetUserByID возвращает пользователя по shm user_id (фильтр admin/user).
func (c *APIClient) GetUserByID(userID int) (*models.User, error) {
	if userID <= 0 {
		return nil, nil
	}
	filter := map[string]interface{}{
		"user_id": userID,
	}
	jsonBytes, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var users struct {
		Data []models.User `json:"data"`
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}

	for i := range users.Data {
		if users.Data[i].ID == userID {
			return &users.Data[i], nil
		}
	}
	return nil, nil
}

func (c *APIClient) GetUserByLogin(login string) (*models.User, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, nil
	}

	filter := map[string]interface{}{
		"login": login,
	}
	jsonBytes, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}
	encoded := url.QueryEscape(string(jsonBytes))

	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/shm/v1/admin/user?filter=%s", c.ServerURL, encoded),
		nil,
	)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var users struct {
		Data []models.User `json:"data"`
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}

	for i := range users.Data {
		if users.Data[i].Login == login {
			return &users.Data[i], nil
		}
	}
	return nil, nil
}

func (c *APIClient) GetUserByLogin2(login2 string) (*models.User, error) {
	login2 = strings.TrimSpace(login2)
	if login2 == "" {
		return nil, nil
	}

	filter := map[string]interface{}{
		"login2": login2,
	}
	jsonBytes, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}
	encoded := url.QueryEscape(string(jsonBytes))

	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf("%s/shm/v1/admin/user?filter=%s", c.ServerURL, encoded),
		nil,
	)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var users struct {
		Data []models.User `json:"data"`
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}

	want := login2
	for i := range users.Data {
		if strings.TrimSpace(users.Data[i].Login2) == want {
			return &users.Data[i], nil
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

// FetchAdminUserRowRaw — строка shm admin/user для user_id с settings как JSON без потери неизвестных полей.
func (c *APIClient) FetchAdminUserRowRaw(userID int) (login string, settingsRaw json.RawMessage, err error) {
	if userID <= 0 {
		return "", nil, fmt.Errorf("invalid user id")
	}
	filter := map[string]interface{}{
		"user_id": userID,
	}
	jsonBytes, err := json.Marshal(filter)
	if err != nil {
		return "", nil, err
	}
	encoded := url.QueryEscape(string(jsonBytes))
	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf("%s/shm/v1/admin/user?filter=%s", c.ServerURL, encoded),
		nil)
	if err != nil {
		return "", nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("get admin user raw: HTTP %d", resp.StatusCode)
	}

	var envelope struct {
		Data []struct {
			UserID   int             `json:"user_id"`
			Login    string          `json:"login"`
			Settings json.RawMessage `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", nil, fmt.Errorf("decode admin user envelope: %w", err)
	}
	for _, row := range envelope.Data {
		if row.UserID != userID {
			continue
		}
		return row.Login, row.Settings, nil
	}
	return "", nil, nil
}

func parseAdminUserUpdateBody(userID int, respBody []byte) (*models.User, bool) {
	var out struct {
		Data []models.User `json:"data"`
	}
	if len(respBody) == 0 || json.Unmarshal(respBody, &out) != nil || len(out.Data) == 0 {
		return nil, false
	}
	for i := range out.Data {
		if out.Data[i].ID == userID {
			u := out.Data[i]
			return &u, true
		}
	}
	u := out.Data[0]
	return &u, true
}

// verifyPersistedLogin2 делает один GET по login2 с тем же HTTPClient (уже задаёт timeout из конфигурации).
func (c *APIClient) verifyPersistedLogin2(userID int, login2Want string) (*models.User, int64, error) {
	want := strings.TrimSpace(login2Want)
	if want == "" {
		return nil, 0, fmt.Errorf("empty login2 for verification")
	}
	t0 := time.Now()
	u, err := c.GetUserByLogin2(want)
	durMs := time.Since(t0).Milliseconds()
	if err != nil {
		slog.Error("shm admin user verify login2", "stage", "shm_get_by_login2", "user_id", userID, "duration_ms", durMs, "err", err)
		return nil, durMs, err
	}
	if u == nil {
		slog.Info("shm admin user verify login2", "user_id", userID, "duration_ms", durMs, "ok", false)
		return nil, durMs, fmt.Errorf("login2 verification: shm returned no row: %w", ErrLogin2NotPersistedSHM)
	}
	if u.ID != userID {
		slog.Info("shm admin user verify login2", "user_id", userID, "duration_ms", durMs, "ok", false)
		return nil, durMs, fmt.Errorf("login2 maps user_id=%d want=%d: %w", u.ID, userID, ErrLogin2NotPersistedSHM)
	}
	got := strings.TrimSpace(u.Login2)
	if got != want {
		slog.Info("shm admin user verify login2", "user_id", userID, "duration_ms", durMs, "ok", false)
		return nil, durMs, fmt.Errorf("persisted login2 mismatch: %w", ErrLogin2NotPersistedSHM)
	}
	slog.Info("shm admin user verify login2", "user_id", userID, "duration_ms", durMs, "ok", true)
	return u, durMs, nil
}

func (c *APIClient) adminUserUpdate(method string, userID int, raw []byte) (respBody []byte, statusCode int, durationMs int64, err error) {
	endpoint := "/shm/v1/admin/user"
	fullURL := fmt.Sprintf("%s%s", c.ServerURL, endpoint)
	req, err := http.NewRequest(method, fullURL, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	t0 := time.Now()
	resp, err := c.HTTPClient.Do(req)
	durationMs = time.Since(t0).Milliseconds()
	if err != nil {
		slog.Error("shm admin user update",
			"stage", "transport", "method", method, "user_id", userID, "duration_ms", durationMs, "err", err)
		return nil, 0, durationMs, err
	}
	defer resp.Body.Close()
	respBody, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if rerr != nil {
		return nil, resp.StatusCode, durationMs, rerr
	}
	return respBody, resp.StatusCode, durationMs, nil
}

// PostAdminUserUpdateSettings выполняет POST /shm/v1/admin/user и при наличии login2 обязательно проверяет,
// что второй логин реально сохранился (GET по login2). Если после POST связка отсутствует — пробуем один PUT с тем же телом (некоторые билды SHM принимают login2 только там).
func (c *APIClient) PostAdminUserUpdateSettings(userID int, login2 string, settingsObj map[string]interface{}) (*models.User, error) {
	if userID <= 0 || settingsObj == nil {
		return nil, fmt.Errorf("invalid update user settings")
	}
	payload := map[string]interface{}{
		"user_id":  userID,
		"settings": settingsObj,
	}
	login2Trim := strings.TrimSpace(login2)
	if login2Trim != "" {
		payload["login2"] = login2Trim
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, status, durMs, err := c.adminUserUpdate(http.MethodPost, userID, raw)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		slog.Error("shm admin user update", "stage", "post_http_status", "user_id", userID, "status_code", status, "duration_ms", durMs, "body_bytes", len(respBody))
		return nil, fmt.Errorf("post admin user settings: HTTP %d (duration_ms=%d)", status, durMs)
	}

	parsedFromBody, haveParsed := parseAdminUserUpdateBody(userID, respBody)

	tryVerify := login2Trim != ""
	if tryVerify {
		persisted, _, vErr := c.verifyPersistedLogin2(userID, login2Trim)
		if vErr == nil {
			return persisted, nil
		}
		if !errors.Is(vErr, ErrLogin2NotPersistedSHM) {
			return nil, vErr
		}

		slog.Warn("shm admin user: login2 not visible after POST, retry PUT", "user_id", userID)

		respBodyPut, statusPut, durPut, errPut := c.adminUserUpdate(http.MethodPut, userID, raw)
		if errPut != nil {
			return nil, errPut
		}
		if statusPut != http.StatusOK {
			slog.Error("shm admin user update", "stage", "put_http_status", "user_id", userID, "status_code", statusPut, "duration_ms", durPut, "body_bytes", len(respBodyPut))
			return nil, fmt.Errorf("put admin user settings: HTTP %d (duration_ms=%d)", statusPut, durPut)
		}
		_, _ = parseAdminUserUpdateBody(userID, respBodyPut) // проверку делаем по login2, не по телу ответа PUT

		persisted, _, vErr = c.verifyPersistedLogin2(userID, login2Trim)
		if vErr != nil {
			return nil, vErr
		}
		return persisted, nil
	}

	if haveParsed {
		return parsedFromBody, nil
	}
	u := &models.User{ID: userID}
	if login2Trim != "" {
		u.Login2 = login2Trim
	}
	return u, nil
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
	if category := c.expectedServiceCategory(); category != "" {
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

	expectedCategory := c.expectedServiceCategory()
	out := make([]models.UserService, 0, len(result.Data))
	for i := range result.Data {
		us := result.Data[i]
		if us.UserID != userID {
			slog.Error("GetUserServices: SHM returned row with unexpected user_id",
				"requested_user_id", userID, "row_user_id", us.UserID, "user_service_id", us.ServiceID)
			continue
		}
		if !models.ServiceCategoryAllowed(expectedCategory, us.Category) {
			slog.Error("GetUserServices: SHM returned row outside active category",
				"requested_user_id", userID, "user_service_id", us.ServiceID, "category", us.Category)
			continue
		}
		out = append(out, us)
	}
	return out, nil

}

func parsePositiveUserServiceID(userServiceID string) (int, error) {
	s := strings.TrimSpace(userServiceID)
	if s == "" {
		return 0, fmt.Errorf("invalid user service id")
	}
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid user service id")
	}
	return id, nil
}

// GetUserServiceByUserID загружает user_service только в контексте владельца.
// Несуществующая, чужая или внекатегорийная услуга → ErrUserServiceUnavailable (без различия причин).
func (c *APIClient) GetUserServiceByUserID(userID int, userServiceID string) (*models.UserService, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	usID, err := parsePositiveUserServiceID(userServiceID)
	if err != nil {
		return nil, err
	}
	userServiceIDTrimmed := strings.TrimSpace(userServiceID)

	// Фильтр: user_id + user_service_id (строка, как в прежнем GetUserService) + category при наличии.
	f := map[string]any{
		"user_id":         userID,
		"user_service_id": userServiceIDTrimmed,
	}
	expectedCategory := c.expectedServiceCategory()
	if expectedCategory != "" {
		f["category"] = expectedCategory
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

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	type ServiceResponse struct {
		Data []models.UserService `json:"data"`
	}
	var result ServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, ErrUserServiceUnavailable
	}

	us := result.Data[0]
	// Локальные проверки до Marzban/storage: не доверяем фильтру SHM.
	if us.UserID != userID || us.ServiceID != usID || !models.ServiceCategoryAllowed(expectedCategory, us.Category) {
		return nil, ErrUserServiceUnavailable
	}

	// Префикс vpn-mz- — технический тип (Marzban), не авторизация.
	if strings.HasPrefix(us.Category, "vpn-mz-") && us.Status == "ACTIVE" {
		userKey, err := c.GetUserKeyMarzban(us.UserID, us.ServiceID)
		if err != nil {
			return nil, err
		}
		us.KeyMarzban = *userKey
	}
	return &us, nil
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

// expectedServiceCategory — разрешённая категория услуг из эффективного бренда (пустая строка = без ограничения).
func (c *APIClient) expectedServiceCategory() string {
	if c == nil || c.config == nil {
		return ""
	}
	return c.config.ServiceCategory()
}

// internal/infrastructure/api/client.go
func (c *APIClient) GetServiceByID(serviceID int) (*models.Service, error) {
	// Формируем filter: {"service_id": <id>, "allow_to_order": 1, "category": <cat>}
	// category добавляем только если задана в конфиге.
	f := map[string]any{
		"service_id":     serviceID,
		"allow_to_order": 1,
	}
	expectedCategory := c.expectedServiceCategory()
	if expectedCategory != "" {
		f["category"] = expectedCategory
	}
	fb, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("marshal filter: %w", err)
	}

	u := fmt.Sprintf(
		"%s/shm/v1/admin/service?&filter=%s",
		c.ServerURL,
		url.QueryEscape(string(fb)),
	)

	req, err := http.NewRequest(http.MethodGet, u, nil)
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
		return nil, fmt.Errorf("service %d not found: %w", serviceID, ErrServiceNotFound)
	}
	svc := out.Data[0]
	// Локальная проверка категории: не доверяем только фильтру SHM.
	// Услуга другой категории неотличима от отсутствующей.
	if !models.ServiceCategoryAllowed(expectedCategory, svc.Category) {
		return nil, fmt.Errorf("service %d not found: %w", serviceID, ErrServiceNotFound)
	}
	return &svc, nil
}

func (c *APIClient) GetServices() ([]models.Service, error) {

	// Собираем filter как JSON
	// {"allow_to_order":1, "category":"..."}   // category добавляем только если задана
	f := map[string]any{
		"allow_to_order": 1,
	}
	if category := c.expectedServiceCategory(); category != "" {
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
	filterBytes, err := json.Marshal(map[string]any{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("marshal user pays filter: %w", err)
	}

	q := url.Values{}
	q.Set("filter", string(filterBytes))

	fullURL := c.ServerURL + "/shm/v1/admin/user/pay?" + q.Encode()
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user pays: API status %d", resp.StatusCode)
	}

	type paysResponse struct {
		Data []models.UserPay `json:"data"`
	}

	var result paysResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode user pays: %w", err)
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
