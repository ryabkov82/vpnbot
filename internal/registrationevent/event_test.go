package registrationevent

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
)

func mustRecord(t *testing.T, channel attribution.RegistrationChannel, domain string, m attribution.MarketingInput) attribution.Record {
	t.Helper()
	rec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: channel,
		RegistrationDomain:  domain,
		CapturedAt:          time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC),
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestNew_ExactSchema(t *testing.T) {
	rec := mustRecord(t, attribution.RegistrationChannelWebGoogle, "connect.friends-connect.club", attribution.MarketingInput{
		UTMSource: "telegram", UTMCampaign: "summer",
	})
	ev, err := New("fc", 123, rec)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Event != Name || ev.EventVersion != SchemaVersion {
		t.Fatalf("event meta %#v", ev)
	}
	if ev.AttributionVersion != attribution.SchemaVersion {
		t.Fatalf("attribution_version=%d", ev.AttributionVersion)
	}
	if ev.BrandID != "fc" || ev.UserID != 123 {
		t.Fatalf("ids %#v", ev)
	}
	if ev.RegistrationChannel != attribution.RegistrationChannelWebGoogle {
		t.Fatalf("channel %q", ev.RegistrationChannel)
	}
	if ev.RegistrationDomain != "connect.friends-connect.club" {
		t.Fatalf("domain %q", ev.RegistrationDomain)
	}
	if ev.CapturedAt != "2026-07-27T15:00:00Z" {
		t.Fatalf("captured_at %q", ev.CapturedAt)
	}
	if ev.Organic {
		t.Fatal("want organic=false")
	}
}

func TestNew_Organic(t *testing.T) {
	organic := mustRecord(t, attribution.RegistrationChannelTelegram, "connect.vpn-for-friends.com", attribution.MarketingInput{})
	ev, err := New("vff", 1, organic)
	if err != nil || !ev.Organic {
		t.Fatalf("organic: %#v err=%v", ev, err)
	}
	withUTM := mustRecord(t, attribution.RegistrationChannelWebMagicLink, "connect.vpn-for-friends.com", attribution.MarketingInput{
		UTMSource: "x",
	})
	ev2, err := New("vff", 2, withUTM)
	if err != nil || ev2.Organic {
		t.Fatalf("non-organic utm: %#v err=%v", ev2, err)
	}
	withTG := mustRecord(t, attribution.RegistrationChannelTelegram, "connect.vpn-for-friends.com", attribution.MarketingInput{
		TelegramStartParam: "summer",
	})
	ev3, err := New("vff", 3, withTG)
	if err != nil || ev3.Organic {
		t.Fatalf("non-organic tg: %#v err=%v", ev3, err)
	}
}

func TestNew_Validation(t *testing.T) {
	rec := mustRecord(t, attribution.RegistrationChannelTelegram, "connect.vpn-for-friends.com", attribution.MarketingInput{})
	if _, err := New("  ", 1, rec); !errors.Is(err, ErrBrandIDRequired) {
		t.Fatalf("empty brand: %v", err)
	}
	if _, err := New("vff", 0, rec); !errors.Is(err, ErrUserIDRequired) {
		t.Fatalf("zero user: %v", err)
	}
	if _, err := New("vff", -5, rec); !errors.Is(err, ErrUserIDRequired) {
		t.Fatalf("neg user: %v", err)
	}
	if _, err := New("vff", 1, attribution.Record{}); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("invalid record: %v", err)
	}
}

func TestLog_PrivacyNoSentinels(t *testing.T) {
	rec := mustRecord(t, attribution.RegistrationChannelWebMagicLink, "connect.vpn-for-friends.com", attribution.MarketingInput{
		LandingPath:        "/secret_landing_path",
		Referrer:           "https://secret_referrer.example/x",
		UTMSource:          "secret_utm_source",
		UTMMedium:          "secret_utm_medium",
		UTMCampaign:        "secret_campaign",
		UTMContent:         "secret_utm_content",
		UTMTerm:            "secret_utm_term",
		TelegramStartParam: "secret_start_payload",
	})
	// Email is not part of Record; include sentinel only to assert it never appears via brand/domain mixups.
	_ = "secret-email@example.com"

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	ev, err := New("vff", 99, rec)
	if err != nil {
		t.Fatal(err)
	}
	Log(logger, ev)
	out := buf.String()

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("json: %v raw=%s", err, out)
	}
	for _, key := range []string{
		"event", "event_version", "attribution_version", "brand_id", "user_id",
		"registration_channel", "registration_domain", "captured_at", "organic",
	} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("missing key %q in %s", key, out)
		}
	}
	if parsed["event"] != Name {
		t.Fatalf("event=%v", parsed["event"])
	}

	sentinels := []string{
		"secret-email@example.com",
		"secret_utm_source",
		"secret_utm_medium",
		"secret_campaign",
		"secret_utm_content",
		"secret_utm_term",
		"secret_start_payload",
		"secret_referrer",
		"secret_landing_path",
	}
	for _, s := range sentinels {
		if strings.Contains(out, s) {
			t.Fatalf("output contains sentinel %q: %s", s, out)
		}
	}
	bannedKeys := []string{
		"email", "ip", "login", "telegram_start_param",
		"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term",
		"referrer", "landing_path", "token", "password", "settings",
	}
	for _, k := range bannedKeys {
		if _, ok := parsed[k]; ok {
			t.Fatalf("banned key %q present", k)
		}
	}
	// OAuth "code" as attribute key must not appear (avoid broad substring checks).
	if _, ok := parsed["code"]; ok {
		t.Fatal("banned key code present")
	}
}

func TestEmit_PropagatesNewError(t *testing.T) {
	err := Emit(slog.Default(), "", 1, attribution.Record{})
	if err == nil {
		t.Fatal("want error")
	}
}
