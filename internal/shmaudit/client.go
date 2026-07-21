package shmaudit

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
	pathAdminService     = "/shm/v1/admin/service"
	pathAdminWithdraw    = "/shm/v1/admin/user/service/withdraw"
	pathAdminPay         = "/shm/v1/admin/user/pay"
)

var allowedGET = map[string]struct{}{
	pathAdminUser:        {},
	pathAdminUserService: {},
	pathAdminService:     {},
	pathAdminWithdraw:    {},
	pathAdminPay:         {},
}

// maxPages — hard limit страниц на один endpoint.
const maxPages = 10000

// Client — read-only SHM Admin API client для аудита.
type Client struct {
	baseURL    string
	login      string
	password   string
	httpClient *http.Client
	delay      time.Duration
}

// NewClient создаёт audit client с cookie jar и read-only transport.
func NewClient(cfg *Config, delay time.Duration) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}
	timeout := time.Duration(cfg.API.Timeout) * time.Second
	transport := &readOnlyTransport{base: http.DefaultTransport}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.API.BaseURL), "/"),
		login:    cfg.API.Login,
		password: cfg.API.Pass,
		httpClient: &http.Client{
			Timeout:   timeout,
			Jar:       jar,
			Transport: transport,
		},
		delay: delay,
	}, nil
}

// readOnlyTransport блокирует любые write-запросы и GET вне whitelist
// до фактической отправки в сеть.
type readOnlyTransport struct {
	base http.RoundTripper
}

func (t *readOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("read-only SHM transport: nil request")
	}
	method := req.Method
	path := req.URL.EscapedPath()
	if path == "" {
		path = req.URL.Path
	}
	// Нормализуем: без trailing slash, кроме корня.
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}

	switch method {
	case http.MethodPost:
		if path == pathAuth {
			return t.base.RoundTrip(req)
		}
		return nil, fmt.Errorf("read-only SHM transport blocked %s %s", method, path)
	case http.MethodGet:
		if _, ok := allowedGET[path]; ok {
			return t.base.RoundTrip(req)
		}
		return nil, fmt.Errorf("read-only SHM transport blocked %s %s", method, path)
	default:
		return nil, fmt.Errorf("read-only SHM transport blocked %s %s", method, path)
	}
}

// Authenticate выполняет POST /shm/user/auth.cgi и сохраняет session cookie в jar.
func (c *Client) Authenticate(ctx context.Context) error {
	payload := map[string]string{
		"login":    c.login,
		"password": c.password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal auth payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+pathAuth, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read auth response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed: HTTP %d", resp.StatusCode)
	}
	var authResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return fmt.Errorf("decode auth response: %w", err)
	}
	if strings.TrimSpace(authResp.SessionID) == "" {
		return fmt.Errorf("auth failed: empty session")
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	c.httpClient.Jar.SetCookies(u, []*http.Cookie{{
		Name:    "session_id",
		Value:   authResp.SessionID,
		Path:    "/",
		Expires: time.Now().Add(24 * time.Hour),
	}})
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, limit, offset int, dest any) error {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", fmt.Sprintf("%d", offset))
	full := c.baseURL + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return fmt.Errorf("create GET %s: %w", path, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("read GET %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode GET %s: %w", path, err)
	}
	return nil
}

// FetchAll загружает все страницы endpoint'а последовательно.
func FetchAll[T any](ctx context.Context, c *Client, path string, pageSize int, logf func(string, ...any)) ([]T, error) {
	if pageSize < 1 || pageSize > 1000 {
		return nil, fmt.Errorf("page-size must be in range 1..1000")
	}
	var all []T
	offset := 0
	var lastFingerprint string
	for pageNum := 0; pageNum < maxPages; pageNum++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pageNum > 0 && c.delay > 0 {
			timer := time.NewTimer(c.delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		if logf != nil {
			logf("loading %s page offset=%d", path, offset)
		}
		var page Page[T]
		if err := c.getJSON(ctx, path, pageSize, offset, &page); err != nil {
			return nil, err
		}
		n := len(page.Data)
		items := int(page.Items)
		limit := int(page.Limit)
		pageOffset := int(page.Offset)
		fp := pageFingerprint(page.Data, pageOffset, limit, items)
		if pageNum > 0 && fp == lastFingerprint {
			return nil, fmt.Errorf("pagination stuck: repeated page at %s offset=%d", path, offset)
		}
		lastFingerprint = fp

		if n == 0 {
			return all, nil
		}
		all = append(all, page.Data...)
		if items > 0 && offset+n >= items {
			return all, nil
		}
		nextOffset := offset + n
		if nextOffset <= offset {
			return nil, fmt.Errorf("pagination stuck: no progress at %s offset=%d", path, offset)
		}
		offset = nextOffset
	}
	return nil, fmt.Errorf("pagination exceeded hard page limit (%d) for %s", maxPages, path)
}

func pageFingerprint[T any](data []T, offset, limit, items int) string {
	raw, err := json.Marshal(struct {
		Data   []T `json:"data"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
		Items  int `json:"items"`
	}{Data: data, Offset: offset, Limit: limit, Items: items})
	if err != nil {
		return fmt.Sprintf("%d:%d:%d:%d", offset, limit, items, len(data))
	}
	return string(raw)
}

func (c *Client) FetchUsers(ctx context.Context, pageSize int, logf func(string, ...any)) ([]AuditUser, error) {
	return FetchAll[AuditUser](ctx, c, pathAdminUser, pageSize, logf)
}

func (c *Client) FetchUserServices(ctx context.Context, pageSize int, logf func(string, ...any)) ([]AuditUserService, error) {
	return FetchAll[AuditUserService](ctx, c, pathAdminUserService, pageSize, logf)
}

func (c *Client) FetchServices(ctx context.Context, pageSize int, logf func(string, ...any)) ([]AuditService, error) {
	return FetchAll[AuditService](ctx, c, pathAdminService, pageSize, logf)
}

func (c *Client) FetchWithdrawals(ctx context.Context, pageSize int, logf func(string, ...any)) ([]AuditWithdraw, error) {
	return FetchAll[AuditWithdraw](ctx, c, pathAdminWithdraw, pageSize, logf)
}

func (c *Client) FetchPayments(ctx context.Context, pageSize int, logf func(string, ...any)) ([]AuditPay, error) {
	return FetchAll[AuditPay](ctx, c, pathAdminPay, pageSize, logf)
}

// HTTPClient экспортирует http.Client только для тестов transport.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}
