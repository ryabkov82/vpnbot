package remnawave

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const defaultHTTPTimeout = 5 * time.Second

const (
	defaultTopNodesLimit     = 10
	bandwidthQueryDateLayout = "2006-01-02"
)

// Client вызывает Remnawave HTTP API (без сторонних SDK).
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient возвращает клиент или nil, если baseURL или token пустые.
func NewClient(baseURL, token string) *Client {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	tok := strings.TrimSpace(token)
	if base == "" || tok == "" {
		return nil
	}
	return &Client{
		BaseURL: base,
		Token:   tok,
		HTTP: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

func truncateBody(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
}

// fetchGET выполняет GET и возвращает тело и HTTP-код; err только при сетевой/I/O ошибке.
func (c *Client) fetchGET(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *Client) doGET(ctx context.Context, path string) ([]byte, int, error) {
	body, code, err := c.fetchGET(ctx, path)
	if err != nil {
		return body, code, err
	}
	if code < 200 || code > 299 {
		msg := truncateBody(strings.TrimSpace(string(body)), 200)
		return body, code, fmt.Errorf("remnawave HTTP %d: %s", code, msg)
	}
	return body, code, nil
}

// doPOSTJSON выполняет POST с JSON-телом; err при коде вне 2xx — как у doGET.
func (c *Client) doPOSTJSON(ctx context.Context, path string, payload any) ([]byte, int, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := truncateBody(strings.TrimSpace(string(body)), 200)
		return body, resp.StatusCode, fmt.Errorf("remnawave HTTP %d: %s", resp.StatusCode, msg)
	}
	return body, resp.StatusCode, nil
}

// User — минимальные поля пользователя Remnawave.
type User struct {
	UUID     string
	Username string
}

// GetUserByUsername выполняет GET /api/users/by-username/{username}.
func (c *Client) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	if c == nil {
		return nil, fmt.Errorf("remnawave: nil client")
	}
	path := "/api/users/by-username/" + url.PathEscape(username)
	body, _, err := c.doGET(ctx, path)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("remnawave user json: %w", err)
	}

	respObj, _ := root["response"].(map[string]any)
	if respObj == nil {
		return nil, fmt.Errorf("remnawave user: missing response object")
	}

	uuid := stringField(respObj, "uuid")
	if uuid == "" {
		if u, ok := respObj["user"].(map[string]any); ok {
			uuid = stringField(u, "uuid")
		}
	}
	if strings.TrimSpace(uuid) == "" {
		return nil, fmt.Errorf("remnawave user: empty uuid")
	}

	return &User{UUID: strings.TrimSpace(uuid), Username: username}, nil
}

// UserBandwidthStats — использованный трафик.
type UserBandwidthStats struct {
	UsedBytes int64
}

// GetUserBandwidthStats выполняет GET /api/bandwidth-stats/users/{uuid} (OpenAPI 2.7.4: topNodesLimit, start/end как date YYYY-MM-DD UTC).
func (c *Client) GetUserBandwidthStats(ctx context.Context, uuid string, start, end time.Time) (*UserBandwidthStats, error) {
	if c == nil {
		return nil, fmt.Errorf("remnawave: nil client")
	}
	if !end.After(start) {
		return nil, fmt.Errorf("remnawave: invalid usage range")
	}
	id := url.PathEscape(strings.TrimSpace(uuid))
	q := url.Values{}
	q.Set("topNodesLimit", strconv.Itoa(defaultTopNodesLimit))
	q.Set("start", start.UTC().Format(bandwidthQueryDateLayout))
	q.Set("end", end.UTC().Format(bandwidthQueryDateLayout))
	path := "/api/bandwidth-stats/users/" + id + "?" + q.Encode()

	body, _, err := c.doGET(ctx, path)
	if err != nil {
		return nil, err
	}
	used, err := parseBandwidthUsed(body)
	if err != nil {
		return nil, err
	}
	return &UserBandwidthStats{UsedBytes: used}, nil
}

// Subscription — подписка Remnawave по username (только URL для шифрования Happ).
type Subscription struct {
	SubscriptionURL string
}

// GetSubscriptionByUsername выполняет GET /api/subscriptions/by-username/{username}.
func (c *Client) GetSubscriptionByUsername(ctx context.Context, username string) (*Subscription, error) {
	if c == nil {
		return nil, fmt.Errorf("remnawave: nil client")
	}
	path := "/api/subscriptions/by-username/" + url.PathEscape(username)
	body, _, err := c.doGET(ctx, path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("remnawave subscription json: %w", err)
	}
	respObj, ok := root["response"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("remnawave subscription: missing response")
	}
	u := strings.TrimSpace(stringField(respObj, "subscriptionUrl"))
	if u == "" {
		return nil, fmt.Errorf("remnawave subscription: empty subscriptionUrl")
	}
	return &Subscription{SubscriptionURL: u}, nil
}

const happCryptLinkPrefix = "happ://crypt"

// EncryptHappLink вызывает POST /api/system/tools/happ/encrypt и возвращает response.encryptedLink.
func (c *Client) EncryptHappLink(ctx context.Context, linkToEncrypt string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("remnawave: nil client")
	}
	if strings.TrimSpace(linkToEncrypt) == "" {
		return "", fmt.Errorf("remnawave encrypt: empty linkToEncrypt")
	}
	body, _, err := c.doPOSTJSON(ctx, "/api/system/tools/happ/encrypt", map[string]string{
		"linkToEncrypt": linkToEncrypt,
	})
	if err != nil {
		return "", err
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return "", fmt.Errorf("remnawave encrypt json: %w", err)
	}
	respObj, ok := root["response"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("remnawave encrypt: missing response")
	}
	enc := strings.TrimSpace(stringField(respObj, "encryptedLink"))
	if enc == "" {
		return "", fmt.Errorf("remnawave encrypt: empty encryptedLink")
	}
	if !strings.HasPrefix(enc, happCryptLinkPrefix) {
		return "", fmt.Errorf("remnawave encrypt: invalid happ link prefix")
	}
	return enc, nil
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

// jsonNumberToInt64 приводит OpenAPI number (float64 в encoding/json) к int64.
func jsonNumberToInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case json.Number:
		f, err := x.Float64()
		if err == nil {
			return int64(f), true
		}
		n, err := x.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

// sumSeriesOrTopNodesTotals суммирует поле total у объектов в массиве (series или topNodes).
func sumSeriesOrTopNodesTotals(items []any) (int64, bool) {
	var sum int64
	var found bool
	for _, el := range items {
		m, ok := el.(map[string]any)
		if !ok {
			continue
		}
		tv, ok := m["total"]
		if !ok {
			continue
		}
		n, ok := jsonNumberToInt64(tv)
		if !ok {
			continue
		}
		sum += n
		found = true
	}
	return sum, found
}

// parseBandwidthUsed разбирает GetStatsUserUsageResponseDto: сумма response.series[].total,
// при отсутствии данных в series — сумма response.topNodes[].total (ограничено topNodesLimit в API, только fallback).
func parseBandwidthUsed(body []byte) (int64, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return 0, fmt.Errorf("remnawave bandwidth json: %w", err)
	}

	resp, ok := root["response"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("remnawave bandwidth: empty usage response")
	}

	series, hasSeries := resp["series"].([]any)
	if hasSeries && len(series) > 0 {
		if n, ok := sumSeriesOrTopNodesTotals(series); ok {
			return n, nil
		}
		return 0, fmt.Errorf("remnawave bandwidth: empty usage response")
	}

	// Fallback: topNodes ограничен topNodesLimit на стороне API; используем только если series отсутствует или пустой.
	if topNodes, ok := resp["topNodes"].([]any); ok && len(topNodes) > 0 {
		if n, ok := sumSeriesOrTopNodesTotals(topNodes); ok {
			return n, nil
		}
	}

	return 0, fmt.Errorf("remnawave bandwidth: empty usage response")
}
