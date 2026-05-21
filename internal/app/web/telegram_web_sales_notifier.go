package web

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
)

// webSalesTelegramSend отправляет plain text в тот же чат, что и lead-уведомления.
// Подменяется в тестах.
var webSalesTelegramSend = postTelegramPlainTextMessage

func webSalesActiveNotifyTTL(cfg *config.Config) time.Duration {
	ttl := webSalesOrderTokenTTL(cfg)
	if ttl < 48*time.Hour {
		return 48 * time.Hour
	}
	return ttl
}

// webSalesOrderActiveNotified — защита от повторных Telegram при опросе /order/status.
var webSalesOrderActiveNotified = &webSalesActiveOnceStore{}

type webSalesActiveOnceStore struct {
	mu sync.Mutex
	m  map[int]time.Time // user_service_id -> deadline
}

func (s *webSalesActiveOnceStore) tryMarkFirst(userServiceID int, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[int]time.Time)
	}
	now := time.Now()
	for id, until := range s.m {
		if now.After(until) {
			delete(s.m, id)
		}
	}
	if _, ok := s.m[userServiceID]; ok {
		return false
	}
	s.m[userServiceID] = now.Add(ttl)
	return true
}

func resetWebSalesOrderActiveNotifiedForTest() {
	webSalesOrderActiveNotified.mu.Lock()
	defer webSalesOrderActiveNotified.mu.Unlock()
	webSalesOrderActiveNotified.m = make(map[int]time.Time)
}

func sendWebOrderStartTelegramNotification(cfg *config.Config, email, contact, serviceName string, serviceID int, amount float64, ip string) {
	c := strings.TrimSpace(contact)
	if c == "" {
		c = "—"
	}
	if strings.TrimSpace(ip) == "" {
		ip = "unknown"
	}
	sn := strings.TrimSpace(serviceName)
	if sn == "" {
		sn = "Тариф"
	}
	var b strings.Builder
	b.WriteString("🟡 Web order started\n\n")
	b.WriteString("Тариф: ")
	b.WriteString(sn)
	b.WriteString("\nservice_id: ")
	b.WriteString(strconv.Itoa(serviceID))
	b.WriteString("\nСумма: ")
	b.WriteString(strconv.FormatFloat(amount, 'f', -1, 64))
	b.WriteString(" ₽\nEmail: ")
	b.WriteString(email)
	b.WriteString("\nКонтакт: ")
	b.WriteString(c)
	b.WriteString("\nIP: ")
	b.WriteString(ip)
	b.WriteString("\n\nПисьмо со ссылкой оплаты отправлено.")
	webSalesTelegramSend(cfg, b.String(), "web order start telegram")
}

func sendWebOrderCreatedTelegramNotification(cfg *config.Config, email, serviceName string, serviceID int, amount float64, userID int, login string, userServiceID int, userServiceStatus, ip string) {
	if strings.TrimSpace(ip) == "" {
		ip = "unknown"
	}
	sn := strings.TrimSpace(serviceName)
	if sn == "" {
		sn = "Тариф"
	}
	lg := strings.TrimSpace(login)
	if lg == "" {
		lg = "—"
	}
	var b strings.Builder
	b.WriteString("🧾 Web order created\n\n")
	b.WriteString("Тариф: ")
	b.WriteString(sn)
	b.WriteString("\nservice_id: ")
	b.WriteString(strconv.Itoa(serviceID))
	b.WriteString("\nСумма: ")
	b.WriteString(strconv.FormatFloat(amount, 'f', -1, 64))
	b.WriteString(" ₽\n\nEmail: ")
	b.WriteString(email)
	b.WriteString("\nSHM user_id: ")
	b.WriteString(strconv.Itoa(userID))
	b.WriteString("\nLogin: ")
	b.WriteString(lg)
	b.WriteString("\nuser_service_id: ")
	b.WriteString(strconv.Itoa(userServiceID))
	b.WriteString("\nСтатус услуги: ")
	b.WriteString(strings.TrimSpace(userServiceStatus))
	b.WriteString("\nIP: ")
	b.WriteString(ip)
	b.WriteString("\n\nПользователь перешел к оплате.")
	webSalesTelegramSend(cfg, b.String(), "web order created telegram")
}

func sendWebOrderActiveTelegramNotification(cfg *config.Config, email, serviceName string, serviceID int, userID, userServiceID int, connectURL, ip string) {
	if strings.TrimSpace(ip) == "" {
		ip = "unknown"
	}
	sn := strings.TrimSpace(serviceName)
	if sn == "" {
		sn = strconv.Itoa(serviceID)
	}
	cu := strings.TrimSpace(connectURL)
	if cu == "" {
		cu = "—"
	}
	var b strings.Builder
	b.WriteString("✅ Web order paid\n\n")
	b.WriteString("Тариф: ")
	b.WriteString(sn)
	b.WriteString("\nservice_id: ")
	b.WriteString(strconv.Itoa(serviceID))
	b.WriteString("\nEmail: ")
	b.WriteString(email)
	b.WriteString("\nSHM user_id: ")
	b.WriteString(strconv.Itoa(userID))
	b.WriteString("\nuser_service_id: ")
	b.WriteString(strconv.Itoa(userServiceID))
	b.WriteString("\nIP: ")
	b.WriteString(ip)
	b.WriteString("\n\nОплата найдена, VPN активен.\nПодключение: ")
	b.WriteString(cu)
	webSalesTelegramSend(cfg, b.String(), "web order paid telegram")
}
