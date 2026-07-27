package bot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
	appService "github.com/ryabkov82/vpnbot/internal/service"
)

func telegramFlowTestCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Brand.ID = "vff"
	cfg.Brand.Name = "VPN for Friends"
	cfg.Brand.PublicBaseURL = "https://connect.vpn-for-friends.com"
	cfg.Brand.LandingURL = "https://vpn-for-friends.com"
	cfg.Brand.WebUserLoginPrefix = "web_"
	cfg.Brand.WebUserSource = "vpn-for-friends.com"
	cfg.Features.Trial.Enabled = true
	cfg.Features.Trial.RequireStartParam = true
	cfg.Features.Trial.AllowedStartParams = []string{"allowlisted_trial"}
	cfg.Features.Trial.EligibilityTTLHours = 24
	return cfg
}

// simulateStartCapture mirrors handleStart attribution side-effects for an unknown user.
func simulateStartCapture(t *testing.T, botSvc *Service, chatID int64, payload string, now time.Time) attribution.Record {
	t.Helper()
	rec, err := buildTelegramRegistrationAttribution(botSvc.config, payload, now)
	if err != nil {
		t.Fatal(err)
	}
	botSvc.rememberTelegramAttribution(chatID, rec, now)
	return rec
}

func TestTelegramFlow_TaggedStartThenRegister(t *testing.T) {
	cfg := telegramFlowTestCfg()
	var got models.UserRegistrationRequest
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/shm/v1/admin/user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/shm/v1/admin/user":
			calls.Add(1)
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &got)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	core := appService.NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, cfg.Brand)
	botSvc := NewService(core, cfg)
	chatID := int64(4242)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	want := simulateStartCapture(t, botSvc, chatID, "telegram_summer", now)
	// Second start must not overwrite.
	simulateStartCapture(t, botSvc, chatID, "other_campaign", now.Add(time.Minute))

	pending, ok := botSvc.peekTelegramAttribution(chatID, now.Add(2*time.Minute))
	if !ok || !attribution.Equal(pending, want) {
		t.Fatalf("first-touch pending %#v", pending)
	}

	err := core.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Password: "p",
		FullName: "U",
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
	}, pending)
	if err != nil {
		t.Fatal(err)
	}
	botSvc.clearTelegramAttribution(chatID)

	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if got.Settings.Attribution == nil || !attribution.Equal(*got.Settings.Attribution, want) {
		t.Fatalf("registered attr %#v want %#v", got.Settings.Attribution, want)
	}
	if _, ok := botSvc.peekTelegramAttribution(chatID, now.Add(3*time.Minute)); ok {
		t.Fatal("pending must be cleared after success")
	}
}

func TestTelegramFlow_RegisterWithoutPending_OrganicFallback(t *testing.T) {
	cfg := telegramFlowTestCfg()
	var got models.UserRegistrationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)

	core := appService.NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, cfg.Brand)
	botSvc := NewService(core, cfg)
	now := time.Now().UTC()
	organic, err := buildTelegramRegistrationAttribution(cfg, "", now)
	if err != nil {
		t.Fatal(err)
	}
	// No pending — register path uses organic fallback.
	if _, ok := botSvc.peekTelegramAttribution(77, now); ok {
		t.Fatal("no pending expected")
	}
	if err := core.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 77}},
	}, organic); err != nil {
		t.Fatal(err)
	}
	if got.Settings.Attribution == nil || !got.Settings.Attribution.IsOrganic() {
		t.Fatalf("want organic %#v", got.Settings.Attribution)
	}
}

func TestTelegramFlow_FailedRegisterKeepsPending(t *testing.T) {
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
		t.Fatal("want register error")
	}
	// Failure path must not clear pending (mirrors handleRegister).
	got, ok := botSvc.peekTelegramAttribution(88, now)
	if !ok || !attribution.Equal(got, rec) {
		t.Fatal("pending must survive registration failure")
	}
}

func TestTelegramFlow_ExistingUserClearsPendingNoRegister(t *testing.T) {
	cfg := telegramFlowTestCfg()
	var putCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Data []models.User `json:"data"`
		}{Data: []models.User{{
			ID:    1,
			Login: "@55",
			Settings: models.UserSettings{
				BrandID:  "vff",
				Telegram: models.TelegramInfo{ChatID: 55},
			},
		}}})
	}))
	t.Cleanup(srv.Close)

	core := appService.NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, cfg.Brand)
	botSvc := NewService(core, cfg)
	now := time.Now().UTC()
	simulateStartCapture(t, botSvc, 55, "stale", now)

	u, err := core.GetUser(55)
	if err != nil || u == nil {
		t.Fatalf("user %#v err=%v", u, err)
	}
	// Existing-user /start|/register semantics.
	botSvc.clearTelegramAttribution(55)
	if putCalls.Load() != 0 {
		t.Fatal("must not register existing user")
	}
	if _, ok := botSvc.peekTelegramAttribution(55, now); ok {
		t.Fatal("stale pending cleared")
	}
}

func TestTelegramFlow_ArbitraryPayload_NoTrialEligibility(t *testing.T) {
	cfg := telegramFlowTestCfg()
	core := appService.NewService(nil, cfg.Brand)
	botSvc := NewService(core, cfg)
	chatID := int64(66)
	payload := "marketing_only_not_in_allowlist"

	// Trial gate mirrors handleStart (allowlist only).
	allowed := false
	for _, p := range cfg.Features.Trial.AllowedStartParams {
		if payload == p {
			allowed = true
			break
		}
	}
	if allowed {
		t.Fatal("test payload must not be allowlisted")
	}
	if cfg.Features.Trial.Enabled && cfg.Features.Trial.RequireStartParam && payload != "" && allowed {
		core.SetTrialEligible(chatID, time.Now().Add(time.Hour))
	}
	if core.IsTrialEligible(chatID) {
		t.Fatal("arbitrary marketing payload must not grant trial")
	}

	rec := simulateStartCapture(t, botSvc, chatID, payload, time.Now().UTC())
	if rec.FirstTouch.TelegramStartParam != payload {
		t.Fatalf("attribution start_param %q", rec.FirstTouch.TelegramStartParam)
	}
}

func TestTelegramFlow_AllowlistedPayload_TrialAndAttribution(t *testing.T) {
	cfg := telegramFlowTestCfg()
	core := appService.NewService(nil, cfg.Brand)
	botSvc := NewService(core, cfg)
	chatID := int64(67)
	payload := "allowlisted_trial"

	core.SetTrialEligible(chatID, time.Now().Add(time.Hour))
	if !core.IsTrialEligible(chatID) {
		t.Fatal("allowlisted payload should keep trial eligibility")
	}
	rec := simulateStartCapture(t, botSvc, chatID, payload, time.Now().UTC())
	if rec.FirstTouch.TelegramStartParam != payload {
		t.Fatalf("start_param %q", rec.FirstTouch.TelegramStartParam)
	}
	if !core.IsTrialEligible(chatID) {
		t.Fatal("attribution capture must not clear trial eligibility")
	}
}

func TestTelegramFlow_RegisterErrorSurfaces(t *testing.T) {
	// sanity: wrong channel rejected before create
	cfg := telegramFlowTestCfg()
	core := appService.NewService(nil, cfg.Brand)
	webRec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelWebMagicLink,
		RegistrationDomain:  "connect.vpn-for-friends.com",
		CapturedAt:          time.Now().UTC(),
	}, attribution.MarketingInput{})
	if err != nil {
		t.Fatal(err)
	}
	err = core.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 1}},
	}, webRec)
	if !errors.Is(err, appService.ErrAttributionWrongChannel) {
		t.Fatalf("got %v", err)
	}
}
