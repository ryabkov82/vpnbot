package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

// publicLeadApp — контракт для публичной заявки (в т.ч. тестовый stub).
type publicLeadApp interface {
	GetServices() ([]models.Service, error)
	GetServiceByID(serviceID int) (*models.Service, error)
}

type publicLeadRequestJSON struct {
	ServiceID int    `json:"service_id"`
	Email     string `json:"email"`
	Contact   string `json:"contact"`
	Website   string `json:"website"`
}

type publicLeadAcceptedJSON struct {
	Status string `json:"status"`
}

// leadRateLimiter — in-memory лимиты: по IP и по email в одной транзакции (без частичного учёта).
type leadRateLimiter struct {
	mu sync.Mutex
	ip map[string][]time.Time
	em map[string][]time.Time

	ipMax   int
	ipWin   time.Duration
	emMax   int
	emWin   time.Duration
	nowFunc func() time.Time
}

func newLeadRateLimiter(ipMax int, ipWin time.Duration, emMax int, emWin time.Duration) *leadRateLimiter {
	return &leadRateLimiter{
		ip:      map[string][]time.Time{},
		em:      map[string][]time.Time{},
		ipMax:   ipMax,
		ipWin:   ipWin,
		emMax:   emMax,
		emWin:   emWin,
		nowFunc: time.Now,
	}
}

func (r *leadRateLimiter) now() time.Time {
	if r.nowFunc != nil {
		return r.nowFunc()
	}
	return time.Now()
}

func pruneLeadHits(ts []time.Time, win time.Duration, now time.Time) []time.Time {
	if len(ts) == 0 {
		return ts
	}
	cutoff := now.Add(-win)
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	return ts[i:]
}

// allow возвращает true, если заявка разрешена и оба счётчика обновлены.
func (r *leadRateLimiter) allow(ipKey, emailKey string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.now()

	ipHits := pruneLeadHits(r.ip[ipKey], r.ipWin, n)
	if len(ipHits) >= r.ipMax {
		return false
	}
	emHits := pruneLeadHits(r.em[emailKey], r.emWin, n)
	if len(emHits) >= r.emMax {
		return false
	}

	ipHits = append(ipHits, n)
	emHits = append(emHits, n)
	r.ip[ipKey] = ipHits
	r.em[emailKey] = emHits
	return true
}

func clientIPForPublicLead(r *http.Request) string {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func validateLeadEmail(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	if strings.TrimSpace(addr.Name) != "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(s), addr.Address)
}

func trialBaseServiceID(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	if !cfg.Features.Trial.Enabled || cfg.Features.Trial.BaseServiceID <= 0 {
		return 0
	}
	return cfg.Features.Trial.BaseServiceID
}

func resolveServiceForPublicLead(app publicLeadApp, serviceID int) (*models.Service, error) {
	list, err := app.GetServices()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ServiceID == serviceID {
			return &list[i], nil
		}
	}
	svc, err := app.GetServiceByID(serviceID)
	if err != nil {
		return nil, err
	}
	if svc == nil {
		return nil, errors.New("service not found")
	}
	return svc, nil
}

func isServiceNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	// GetServiceByID в API-клиенте возвращает fmt.Errorf("service %d not found", …).
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func servePublicLead(cfg *config.Config, app publicLeadApp) http.HandlerFunc {
	rl := newLeadRateLimiter(5, 15*time.Minute, 3, time.Hour)
	return servePublicLeadWithLimiter(cfg, app, rl)
}

func servePublicLeadWithLimiter(cfg *config.Config, app publicLeadApp, rl *leadRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/lead" {
			http.NotFound(w, r)
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req publicLeadRequestJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		if strings.TrimSpace(req.Website) != "" {
			writeJSON(w, http.StatusOK, publicLeadAcceptedJSON{Status: "accepted"})
			return
		}

		if req.ServiceID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid_service")
			return
		}

		if !validateLeadEmail(req.Email) {
			writeJSONError(w, http.StatusBadRequest, "invalid_email")
			return
		}

		if tid := trialBaseServiceID(cfg); tid > 0 && req.ServiceID == tid {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		svc, err := resolveServiceForPublicLead(app, req.ServiceID)
		if err != nil {
			if isServiceNotFoundErr(err) {
				writeJSONError(w, http.StatusNotFound, "service_not_found")
				return
			}
			slog.Error("api/public/lead resolve service", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "services_unavailable")
			return
		}
		if svc == nil {
			writeJSONError(w, http.StatusNotFound, "service_not_found")
			return
		}

		ipKey := clientIPForPublicLead(r)
		if ipKey == "" {
			ipKey = "unknown"
		}
		emailKey := strings.ToLower(strings.TrimSpace(req.Email))

		if !rl.allow(ipKey, emailKey) {
			writeJSONError(w, http.StatusTooManyRequests, "rate_limited")
			return
		}

		preview := models.BuildServicePreview(svc)
		svcName := strings.TrimSpace(preview.Title)
		if svcName == "" {
			svcName = "Тариф"
		}

		slog.Info("public lead",
			"service_id", req.ServiceID,
			"service_name", svcName,
			"email", strings.TrimSpace(req.Email),
			"contact", strings.TrimSpace(req.Contact),
			"ip", ipKey,
		)

		sendLeadTelegramNotification(cfg, publicLead{
			ServiceID: req.ServiceID,
			Email:     strings.TrimSpace(req.Email),
			Contact:   strings.TrimSpace(req.Contact),
		}, svcName, ipKey)

		writeJSON(w, http.StatusOK, publicLeadAcceptedJSON{Status: "accepted"})
	}
}
