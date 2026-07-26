package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
)

func TestUserSettings_NilAttributionOmitted(t *testing.T) {
	s := UserSettings{
		BrandID:  "fc",
		Telegram: TelegramInfo{ChatID: 123},
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"attribution"`) {
		t.Fatalf("nil attribution must be omitted: %s", raw)
	}
}

func TestUserSettings_AttributionRoundTrip(t *testing.T) {
	rec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelWebMagicLink,
		RegistrationDomain:  "connect.friends-connect.club",
		CapturedAt:          time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC),
	}, attribution.MarketingInput{
		LandingPath: "/account",
		UTMSource:   "telegram",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := UserSettings{
		BrandID:     "fc",
		Telegram:    TelegramInfo{ChatID: 123},
		Web:         WebInfo{Email: "u@example.com", Source: "vpn-for-friends.com"},
		Attribution: &rec,
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	for _, needle := range []string{
		`"attribution"`,
		`"version":1`,
		`"first_touch"`,
		`"registration_channel":"web_magic_link"`,
		`"registration_domain":"connect.friends-connect.club"`,
		`"captured_at":"2026-07-26T18:00:00Z"`,
	} {
		if !strings.Contains(js, needle) {
			t.Fatalf("missing %s in %s", needle, js)
		}
	}
	if strings.Contains(js, `"attribution":{"attribution"`) {
		t.Fatalf("extra nesting: %s", js)
	}

	var back UserSettings
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.BrandID != "fc" || back.Telegram.ChatID != 123 || back.Web.Email != "u@example.com" {
		t.Fatalf("identity fields: %+v", back)
	}
	if back.Attribution == nil || !attribution.Equal(*back.Attribution, rec) {
		t.Fatalf("attribution round-trip: %+v", back.Attribution)
	}
}

func TestUserSettings_UnmarshalSHMShape(t *testing.T) {
	raw := []byte(`{
		"brand_id": "fc",
		"telegram": {"chat_id": 123, "username": "tester"},
		"web": {"email": "u@example.com", "source": "telegram_link"},
		"attribution": {
			"version": 1,
			"first_touch": {
				"registration_channel": "telegram",
				"registration_domain": "connect.friends-connect.club",
				"telegram_start_param": "summer",
				"captured_at": "2026-07-26T18:00:00Z"
			}
		}
	}`)
	var s UserSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if s.BrandID != "fc" || s.Telegram.ChatID != 123 || s.Web.Source != "telegram_link" {
		t.Fatalf("%+v", s)
	}
	if s.Attribution == nil || !s.Attribution.Valid() {
		t.Fatalf("attribution=%+v", s.Attribution)
	}
	if s.Attribution.FirstTouch.RegistrationChannel != attribution.RegistrationChannelTelegram {
		t.Fatalf("channel=%q", s.Attribution.FirstTouch.RegistrationChannel)
	}
	if s.Attribution.FirstTouch.TelegramStartParam != "summer" {
		t.Fatalf("start=%q", s.Attribution.FirstTouch.TelegramStartParam)
	}
}
