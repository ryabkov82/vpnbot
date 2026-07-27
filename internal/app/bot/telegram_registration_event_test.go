package bot

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/registrationevent"
	appService "github.com/ryabkov82/vpnbot/internal/service"
)

func captureBotSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func countRegistered(t *testing.T, raw string) int {
	t.Helper()
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if m["event"] == registrationevent.Name {
			n++
		}
	}
	return n
}

// emitAfterTelegramRegister mirrors handleRegister post-create event + clear sequence.
func emitAfterTelegramRegister(t *testing.T, botSvc *Service, core *appService.Service, chatID int64, rec attribution.Record) {
	t.Helper()
	createdUser, lookupErr := core.GetUser(chatID)
	if lookupErr != nil || createdUser == nil {
		slog.Warn("registration event skipped",
			"brand_id", botSvc.config.BrandID(),
			"registration_channel", "telegram",
			"reason", "post_create_lookup_failed",
		)
	} else if err := registrationevent.Emit(slog.Default(), botSvc.config.BrandID(), createdUser.ID, rec); err != nil {
		slog.Warn("registration event skipped",
			"brand_id", botSvc.config.BrandID(),
			"registration_channel", "telegram",
			"reason", "emit_failed",
		)
	}
	botSvc.clearTelegramAttribution(chatID)
}

func TestTelegramRegistrationEvent_SuccessOneEvent(t *testing.T) {
	buf := captureBotSlog(t)
	cfg := telegramFlowTestCfg()
	chatID := int64(9001)
	var putCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/shm/v1/admin/user":
			putCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/shm/v1/admin/user":
			// After create, GetUser looks up by login=@9001
			filter, _ := url.QueryUnescape(r.URL.Query().Get("filter"))
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(filter, "@9001") && putCalls.Load() > 0 {
				_ = json.NewEncoder(w).Encode(struct {
					Data []models.User `json:"data"`
				}{Data: []models.User{{
					ID:    321,
					Login: "@9001",
					Settings: models.UserSettings{
						BrandID:  "vff",
						Telegram: models.TelegramInfo{ChatID: chatID},
					},
				}}})
				return
			}
			_, _ = io.WriteString(w, `{"data":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	core := appService.NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, cfg.Brand)
	botSvc := NewService(core, cfg)
	now := time.Now().UTC()
	rec := simulateStartCapture(t, botSvc, chatID, "telegram_summer", now)

	if err := core.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Password: "p",
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
	}, rec); err != nil {
		t.Fatal(err)
	}
	emitAfterTelegramRegister(t, botSvc, core, chatID, rec)

	if n := countRegistered(t, buf.String()); n != 1 {
		t.Fatalf("events=%d log=%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), `"registration_channel":"telegram"`) {
		t.Fatalf("channel missing: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"user_id":321`) {
		t.Fatalf("user_id missing: %s", buf.String())
	}
	for _, banned := range []string{`"telegram_start_param"`, `"chat_id"`, "telegram_summer"} {
		if strings.Contains(buf.String(), banned) {
			t.Fatalf("PII/marketing leaked %q in %s", banned, buf.String())
		}
	}
	if _, ok := botSvc.peekTelegramAttribution(chatID, now); ok {
		t.Fatal("pending must be cleared even after event")
	}
}

func TestTelegramRegistrationEvent_ExistingUserNoEvent(t *testing.T) {
	buf := captureBotSlog(t)
	cfg := telegramFlowTestCfg()
	core := appService.NewService(nil, cfg.Brand)
	botSvc := NewService(core, cfg)
	// Existing-user path: clear pending, no RegisterUser, no event.
	botSvc.clearTelegramAttribution(55)
	if n := countRegistered(t, buf.String()); n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
}

func TestTelegramRegistrationEvent_RegisterErrorNoEvent(t *testing.T) {
	buf := captureBotSlog(t)
	cfg := telegramFlowTestCfg()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)
	core := appService.NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, cfg.Brand)
	botSvc := NewService(core, cfg)
	now := time.Now().UTC()
	rec := simulateStartCapture(t, botSvc, 88, "keep", now)
	err := core.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 88}},
	}, rec)
	if err == nil {
		t.Fatal("want error")
	}
	// Failure path: do not emit, do not clear (mirrors handleRegister).
	if n := countRegistered(t, buf.String()); n != 0 {
		t.Fatalf("want 0 events, got %d", n)
	}
	if _, ok := botSvc.peekTelegramAttribution(88, now); !ok {
		t.Fatal("pending must remain")
	}
}

func TestTelegramRegistrationEvent_LookupFailureStillClearsPending(t *testing.T) {
	buf := captureBotSlog(t)
	cfg := telegramFlowTestCfg()
	chatID := int64(9100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Always empty → GetUser returns nil after create.
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)

	core := appService.NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, cfg.Brand)
	botSvc := NewService(core, cfg)
	now := time.Now().UTC()
	rec := simulateStartCapture(t, botSvc, chatID, "x", now)
	if err := core.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
	}, rec); err != nil {
		t.Fatal(err)
	}
	emitAfterTelegramRegister(t, botSvc, core, chatID, rec)

	if n := countRegistered(t, buf.String()); n != 0 {
		t.Fatalf("no user_registered on lookup failure, got %d: %s", n, buf.String())
	}
	if !strings.Contains(buf.String(), "registration event skipped") {
		t.Fatalf("want skip warning: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "post_create_lookup_failed") {
		t.Fatalf("want reason: %s", buf.String())
	}
	if _, ok := botSvc.peekTelegramAttribution(chatID, now); ok {
		t.Fatal("pending must clear after successful create even if event skipped")
	}
}

func TestTelegramRegistrationEvent_NoSecondEventOnRepeat(t *testing.T) {
	buf := captureBotSlog(t)
	cfg := telegramFlowTestCfg()
	chatID := int64(9200)
	var putCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		filter, _ := url.QueryUnescape(r.URL.Query().Get("filter"))
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(filter, "@9200") {
			_ = json.NewEncoder(w).Encode(struct {
				Data []models.User `json:"data"`
			}{Data: []models.User{{
				ID:    400,
				Login: "@9200",
				Settings: models.UserSettings{
					BrandID:  "vff",
					Telegram: models.TelegramInfo{ChatID: chatID},
				},
			}}})
			return
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)

	core := appService.NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, cfg.Brand)
	botSvc := NewService(core, cfg)
	now := time.Now().UTC()
	rec := simulateStartCapture(t, botSvc, chatID, "once", now)
	if err := core.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
	}, rec); err != nil {
		t.Fatal(err)
	}
	emitAfterTelegramRegister(t, botSvc, core, chatID, rec)

	// Repeat /register for existing user: GetUser finds user, no second create/event.
	u, err := core.GetUser(chatID)
	if err != nil || u == nil {
		t.Fatalf("existing %#v err=%v", u, err)
	}
	botSvc.clearTelegramAttribution(chatID)

	if n := countRegistered(t, buf.String()); n != 1 {
		t.Fatalf("want exactly 1 event, got %d: %s", n, buf.String())
	}
	if putCalls.Load() != 1 {
		t.Fatalf("putCalls=%d", putCalls.Load())
	}
}
