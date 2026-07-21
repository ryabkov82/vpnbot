package shmmigrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	pathAuth             = "/shm/user/auth.cgi"
	pathAdminUser        = "/shm/v1/admin/user"
	pathAdminUserService = "/shm/v1/admin/user/service"
)

// Config — минимальный SHM API config для миграции.
type Config struct {
	API struct {
		BaseURL string `json:"base_url"`
		Login   string `json:"api_login"`
		Pass    string `json:"api_pass"`
		Timeout int    `json:"timeout_seconds"`
	} `json:"api"`
}

// LiveUser — live-снимок пользователя для миграции.
type LiveUser struct {
	UserID   int             `json:"user_id"`
	Login    string          `json:"login"`
	Login2   string          `json:"login2"`
	Balance  float64         `json:"balance"`
	Bonus    float64         `json:"bonus"`
	Credit   float64         `json:"credit"`
	Settings json.RawMessage `json:"settings"`
}

// UserService — минимальные поля user_service для preflight.
type UserService struct {
	UserID   int    `json:"user_id"`
	Category string `json:"category"`
	Status   string `json:"status"`
}

// Client — migration SHM client (auth + GET user/service + POST user update).
type Client struct {
	baseURL    string
	login      string
	password   string
	httpClient *http.Client
}

func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}
	timeout := time.Duration(cfg.API.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.API.BaseURL), "/"),
		login:    cfg.API.Login,
		password: cfg.API.Pass,
		httpClient: &http.Client{
			Timeout:   timeout,
			Jar:       jar,
			Transport: &migrateTransport{base: http.DefaultTransport},
		},
	}, nil
}

type migrateTransport struct {
	base http.RoundTripper
}

func (t *migrateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("migration SHM transport: nil request")
	}
	method := req.Method
	path := req.URL.EscapedPath()
	if path == "" {
		path = req.URL.Path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	switch method {
	case http.MethodPost:
		if path == pathAuth || path == pathAdminUser {
			return t.base.RoundTrip(req)
		}
		return nil, fmt.Errorf("migration SHM transport blocked %s %s", method, path)
	case http.MethodGet:
		if path == pathAdminUser || path == pathAdminUserService {
			return t.base.RoundTrip(req)
		}
		return nil, fmt.Errorf("migration SHM transport blocked %s %s", method, path)
	default:
		return nil, fmt.Errorf("migration SHM transport blocked %s %s", method, path)
	}
}

func (c *Client) Authenticate(ctx context.Context) error {
	payload := map[string]string{"login": c.login, "password": c.password}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+pathAuth, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read auth: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed: HTTP %d", resp.StatusCode)
	}
	var authResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &authResp); err != nil {
		return fmt.Errorf("decode auth: %w", err)
	}
	if strings.TrimSpace(authResp.SessionID) == "" {
		return fmt.Errorf("auth failed: empty session")
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	c.httpClient.Jar.SetCookies(u, []*http.Cookie{{
		Name:    "session_id",
		Value:   authResp.SessionID,
		Path:    "/",
		Expires: time.Now().Add(24 * time.Hour),
	}})
	return nil
}

func (c *Client) getFiltered(ctx context.Context, path string, filter map[string]any, dest any) error {
	fb, err := json.Marshal(filter)
	if err != nil {
		return err
	}
	full := c.baseURL + path + "?filter=" + url.QueryEscape(string(fb))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("read GET %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("decode GET %s: %w", path, err)
	}
	return nil
}

// GetUserByID возвращает пользователя по user_id или nil, если не найден.
func (c *Client) GetUserByID(ctx context.Context, userID int) (*LiveUser, error) {
	var page struct {
		Data []LiveUser `json:"data"`
	}
	if err := c.getFiltered(ctx, pathAdminUser, map[string]any{"user_id": userID}, &page); err != nil {
		return nil, err
	}
	for i := range page.Data {
		if page.Data[i].UserID == userID {
			u := page.Data[i]
			return &u, nil
		}
	}
	return nil, nil
}

// GetUserByLogin возвращает пользователя с точным login или nil.
func (c *Client) GetUserByLogin(ctx context.Context, login string) (*LiveUser, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, nil
	}
	var page struct {
		Data []LiveUser `json:"data"`
	}
	if err := c.getFiltered(ctx, pathAdminUser, map[string]any{"login": login}, &page); err != nil {
		return nil, err
	}
	for i := range page.Data {
		if page.Data[i].Login == login {
			u := page.Data[i]
			return &u, nil
		}
	}
	return nil, nil
}

// GetUserServices загружает услуги пользователя без category filter.
func (c *Client) GetUserServices(ctx context.Context, userID int) ([]UserService, error) {
	var page struct {
		Data []UserService `json:"data"`
	}
	if err := c.getFiltered(ctx, pathAdminUserService, map[string]any{"user_id": userID}, &page); err != nil {
		return nil, err
	}
	out := make([]UserService, 0, len(page.Data))
	for _, s := range page.Data {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

// UpdateUser выполняет POST /shm/v1/admin/user (update, не create).
func (c *Client) UpdateUser(ctx context.Context, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+pathAdminUser, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", pathAdminUser, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: HTTP %d", pathAdminUser, resp.StatusCode)
	}
	return nil
}

// HTTPClient — для тестов.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }
