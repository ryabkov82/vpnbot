package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/registrationevent"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func captureDefaultSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func countUserRegisteredEvents(t *testing.T, raw string) int {
	t.Helper()
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line json: %v %q", err, line)
		}
		if m["event"] == registrationevent.Name {
			n++
		}
	}
	return n
}

func sampleWebGoogleAttribution(t *testing.T, domain string) attribution.Record {
	t.Helper()
	if domain == "" {
		domain = "connect.vpn-for-friends.com"
	}
	rec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelWebGoogle,
		RegistrationDomain:  domain,
		CapturedAt:          time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC),
	}, attribution.MarketingInput{UTMSource: "google_ads"})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestFindOrCreateWebUserWithAttribution_EmitsOneRegistrationEvent(t *testing.T) {
	buf := captureDefaultSlog(t)
	login := webuser.WebLoginFromEmail("evt@example.com")
	reg := &testWebUserRegistrar{secondAndLater: &models.User{ID: 501, Login: login}}
	rec := sampleWebMagicLinkAttribution(t, "connect.vpn-for-friends.com")

	_, created, err := findOrCreateWebUserWithAttribution(reg, "evt@example.com", testWebLoginPrefix, testWebUserSource, "vff", rec)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if n := countUserRegisteredEvents(t, buf.String()); n != 1 {
		t.Fatalf("events=%d log=%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), `"registration_channel":"web_magic_link"`) {
		t.Fatalf("channel missing: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"user_id":501`) {
		t.Fatalf("user_id missing: %s", buf.String())
	}
}

func TestFindOrCreateWebUserWithAttribution_GoogleEmitsOneEvent(t *testing.T) {
	buf := captureDefaultSlog(t)
	login := webuser.WebLoginFromEmail("g@example.com")
	reg := &testWebUserRegistrar{secondAndLater: &models.User{ID: 777, Login: login}}
	rec := sampleWebGoogleAttribution(t, "connect.friends-connect.club")

	_, created, err := findOrCreateWebUserWithAttribution(reg, "g@example.com", testWebLoginPrefix, "friends-connect.club", "fc", rec)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if n := countUserRegisteredEvents(t, buf.String()); n != 1 {
		t.Fatalf("events=%d log=%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), `"registration_channel":"web_google"`) {
		t.Fatalf("google channel missing: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"brand_id":"fc"`) {
		t.Fatalf("brand missing: %s", buf.String())
	}
}

func TestFindOrCreateWebUserWithAttribution_ExistingNoEvent(t *testing.T) {
	buf := captureDefaultSlog(t)
	login := webuser.WebLoginFromEmail("exist@example.com")
	reg := &testWebUserRegistrar{firstGet: &models.User{ID: 9, Login: login}}
	rec := sampleWebMagicLinkAttribution(t, "connect.vpn-for-friends.com")
	_, created, err := findOrCreateWebUserWithAttribution(reg, "exist@example.com", testWebLoginPrefix, testWebUserSource, "vff", rec)
	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if n := countUserRegisteredEvents(t, buf.String()); n != 0 {
		t.Fatalf("want 0 events, got %d: %s", n, buf.String())
	}
}

func TestFindOrCreateWebUserWithAttribution_RegisterErrorNoEvent(t *testing.T) {
	buf := captureDefaultSlog(t)
	reg := &testWebUserRegistrar{regErr: errors.New("api down")}
	rec := sampleWebMagicLinkAttribution(t, "connect.vpn-for-friends.com")
	_, _, err := findOrCreateWebUserWithAttribution(reg, "fail@example.com", testWebLoginPrefix, testWebUserSource, "vff", rec)
	if err == nil {
		t.Fatal("want error")
	}
	if n := countUserRegisteredEvents(t, buf.String()); n != 0 {
		t.Fatalf("want 0 events, got %d", n)
	}
}

func TestFindOrCreateWebUserWithAttribution_PostCreateLookupErrorNoEvent(t *testing.T) {
	buf := captureDefaultSlog(t)
	// Register succeeds (lastReg set), but reload returns nil → error, no event.
	reg := &testWebUserRegistrar{firstGet: nil, secondAndLater: nil}
	rec := sampleWebMagicLinkAttribution(t, "connect.vpn-for-friends.com")
	_, _, err := findOrCreateWebUserWithAttribution(reg, "gone@example.com", testWebLoginPrefix, testWebUserSource, "vff", rec)
	if err == nil {
		t.Fatal("want post-create error")
	}
	if n := countUserRegisteredEvents(t, buf.String()); n != 0 {
		t.Fatalf("want 0 events, got %d", n)
	}
}

func TestFindOrCreateWebUser_LegacyNoAttributionNoEvent(t *testing.T) {
	buf := captureDefaultSlog(t)
	login := webuser.WebLoginFromEmail("legacy@example.com")
	reg := &testWebUserRegistrar{secondAndLater: &models.User{ID: 44, Login: login}}
	_, created, err := findOrCreateWebUser(reg, "legacy@example.com", testWebLoginPrefix, testWebUserSource, "vff")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if reg.lastReg.Settings.Attribution != nil {
		t.Fatal("legacy must not set attribution")
	}
	if n := countUserRegisteredEvents(t, buf.String()); n != 0 {
		t.Fatalf("legacy must not emit event, got %d: %s", n, buf.String())
	}
}
