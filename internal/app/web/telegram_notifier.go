package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
)

// publicLead — нормализованные поля заявки для логов и уведомлений.
type publicLead struct {
	ServiceID int
	Email     string
	Contact   string
}

// leadTelegramAPIBase — корень Bot API (подменяется в тестах).
var leadTelegramAPIBase = "https://api.telegram.org"

// leadTelegramHTTPPost выполняет HTTP-запрос к Telegram (подменяется в тестах).
var leadTelegramHTTPPost = defaultLeadTelegramHTTPPost

func defaultLeadTelegramHTTPPost(req *http.Request) (*http.Response, error) {
	c := &http.Client{Timeout: 12 * time.Second}
	return c.Do(req)
}

func resolveLeadNotificationChatID(cfg *config.Config) int64 {
	if cfg == nil {
		return 0
	}
	if cfg.Telegram.LeadsChatID != 0 {
		return cfg.Telegram.LeadsChatID
	}
	return cfg.Telegram.SupportChatID
}

func buildLeadTelegramMessage(lead publicLead, serviceName, ip string) string {
	contact := strings.TrimSpace(lead.Contact)
	if contact == "" {
		contact = "—"
	}
	var b strings.Builder
	b.WriteString("🆕 Заявка с сайта VPN for Friends\n\n")
	b.WriteString("Тариф: ")
	b.WriteString(serviceName)
	b.WriteString("\nservice_id: ")
	b.WriteString(strconv.Itoa(lead.ServiceID))
	b.WriteString("\nEmail: ")
	b.WriteString(lead.Email)
	b.WriteString("\nКонтакт: ")
	b.WriteString(contact)
	b.WriteString("\nIP: ")
	b.WriteString(ip)
	return b.String()
}

// postTelegramPlainTextMessage шлёт plain text в Telegram Bot API (без parse_mode).
// logPrefix — префикс для slog.Warn; токен в логи не пишется.
func postTelegramPlainTextMessage(cfg *config.Config, text string, logPrefix string) {
	if logPrefix == "" {
		logPrefix = "telegram"
	}
	chatID := resolveLeadNotificationChatID(cfg)
	if chatID == 0 {
		return
	}
	token := strings.TrimSpace(cfg.Telegram.Token)
	if token == "" {
		slog.Warn(logPrefix + ": skip, empty bot token")
		return
	}

	body, err := json.Marshal(map[string]any{
		"chat_id": chatID,
		"text":    text,
	})
	if err != nil {
		slog.Warn(logPrefix+": marshal body", "err", err)
		return
	}

	u := leadTelegramAPIBase + "/bot" + token + "/sendMessage"
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		slog.Warn(logPrefix+": build request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := leadTelegramHTTPPost(req)
	if err != nil {
		slog.Warn(logPrefix+": http request failed", "err", err)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		slog.Warn(logPrefix+": read response", "err", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn(logPrefix+": non-200 response", "status", resp.StatusCode)
		return
	}
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		slog.Warn(logPrefix+": decode response", "err", err)
		return
	}
	if !parsed.OK {
		slog.Warn(logPrefix+": api returned error", "description", parsed.Description)
	}
}

// sendLeadTelegramNotification шлёт уведомление в Telegram через Bot API.
// Ошибки не пробрасываются наружу; токен в логи не пишется.
func sendLeadTelegramNotification(cfg *config.Config, lead publicLead, serviceName, ip string) {
	text := buildLeadTelegramMessage(lead, serviceName, ip)
	postTelegramPlainTextMessage(cfg, text, "lead telegram")
}
