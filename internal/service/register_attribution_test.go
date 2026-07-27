package service

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
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func sampleTelegramAttribution(t *testing.T, domain, startParam string) attribution.Record {
	t.Helper()
	if domain == "" {
		domain = "connect.vpn-for-friends.com"
	}
	rec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelTelegram,
		RegistrationDomain:  domain,
		CapturedAt:          time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC),
	}, attribution.MarketingInput{TelegramStartParam: startParam})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRegisterUserWithAttribution_CreatesExactRecord(t *testing.T) {
	rec := sampleTelegramAttribution(t, "connect.vpn-for-friends.com", "telegram_summer")
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var got models.UserRegistrationRequest
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Login != "@12345" {
			t.Fatalf("login=%q", got.Login)
		}
		if got.Settings.BrandID != "vff" {
			t.Fatalf("brand_id=%q", got.Settings.BrandID)
		}
		if got.Settings.Attribution == nil || !attribution.Equal(*got.Settings.Attribution, rec) {
			t.Fatalf("attribution %#v", got.Settings.Attribution)
		}
		if got.Settings.Attribution == &rec {
			t.Fatal("must store a copy")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	err := s.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Password: "p",
		FullName: "T",
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{ChatID: 12345},
		},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestRegisterUserWithAttribution_FCIdentity(t *testing.T) {
	rec := sampleTelegramAttribution(t, "connect.friends-connect.club", "fc_camp")
	var gotLogin string
	var gotBrand string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got models.UserRegistrationRequest
		_ = json.NewDecoder(r.Body).Decode(&got)
		gotLogin = got.Login
		gotBrand = got.Settings.BrandID
		if got.Settings.Attribution == nil || !attribution.Equal(*got.Settings.Attribution, rec) {
			t.Fatalf("attr %#v", got.Settings.Attribution)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("fc"))
	if err := s.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 99}},
	}, rec); err != nil {
		t.Fatal(err)
	}
	if gotLogin != "@fc_99" || gotBrand != "fc" {
		t.Fatalf("login=%q brand=%q", gotLogin, gotBrand)
	}
}

func TestRegisterUserWithAttribution_InvalidRejectedBeforeAPI(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))

	err := s.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 1}},
	}, attribution.Record{})
	if !errors.Is(err, ErrAttributionRequired) {
		t.Fatalf("got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("API must not be called")
	}

	webRec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelWebGoogle,
		RegistrationDomain:  "connect.vpn-for-friends.com",
		CapturedAt:          time.Now().UTC(),
	}, attribution.MarketingInput{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 1}},
	}, webRec)
	if !errors.Is(err, ErrAttributionWrongChannel) {
		t.Fatalf("got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("API must not be called for wrong channel")
	}
}

func TestRegisterUser_PlainKeepsNoAttribution(t *testing.T) {
	var got models.UserRegistrationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	if err := s.RegisterUser(models.UserRegistrationRequest{
		Password: "p",
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 7}},
	}); err != nil {
		t.Fatal(err)
	}
	if got.Settings.Attribution != nil {
		t.Fatalf("plain RegisterUser must not set attribution: %#v", got.Settings.Attribution)
	}
	if got.Login != "@7" || got.Settings.BrandID != "vff" {
		t.Fatalf("login/brand %#v", got)
	}
}

func TestRegisterUserWithAttribution_APIErrorNoFollowUp(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	rec := sampleTelegramAttribution(t, "", "x")
	err := s.RegisterUserWithAttribution(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 3}},
	}, rec)
	if err == nil {
		t.Fatal("want error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}
