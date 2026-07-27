package bot

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/config"
)

func testTelegramAttrCfg(publicBaseURL string) *config.Config {
	cfg := &config.Config{}
	cfg.Brand.ID = "vff"
	cfg.Brand.PublicBaseURL = publicBaseURL
	return cfg
}

func TestBuildTelegramRegistrationAttribution_Exact(t *testing.T) {
	fixed := time.Date(2026, 7, 27, 9, 30, 0, 0, time.UTC)
	cfg := testTelegramAttrCfg("https://connect.vpn-for-friends.com")
	rec, err := buildTelegramRegistrationAttribution(cfg, "telegram_summer", fixed)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Valid() {
		t.Fatal("want valid")
	}
	ft := rec.FirstTouch
	if ft.RegistrationChannel != attribution.RegistrationChannelTelegram {
		t.Fatalf("channel %q", ft.RegistrationChannel)
	}
	if ft.RegistrationDomain != "connect.vpn-for-friends.com" {
		t.Fatalf("domain %q", ft.RegistrationDomain)
	}
	if ft.TelegramStartParam != "telegram_summer" {
		t.Fatalf("start_param %q", ft.TelegramStartParam)
	}
	if ft.CapturedAt != "2026-07-27T09:30:00Z" {
		t.Fatalf("captured_at %q", ft.CapturedAt)
	}
	if !strings.EqualFold(string(ft.RegistrationChannel), "telegram") {
		t.Fatal("channel string")
	}
}

func TestBuildTelegramRegistrationAttribution_FCDomain(t *testing.T) {
	cfg := testTelegramAttrCfg("https://connect.friends-connect.club")
	cfg.Brand.ID = "fc"
	rec, err := buildTelegramRegistrationAttribution(cfg, "camp", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rec.FirstTouch.RegistrationDomain != "connect.friends-connect.club" {
		t.Fatalf("domain %q", rec.FirstTouch.RegistrationDomain)
	}
}

func TestBuildTelegramRegistrationAttribution_EmptyOrganic(t *testing.T) {
	cfg := testTelegramAttrCfg("https://connect.vpn-for-friends.com")
	rec, err := buildTelegramRegistrationAttribution(cfg, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !rec.IsOrganic() || rec.FirstTouch.TelegramStartParam != "" {
		t.Fatalf("want organic: %#v", rec)
	}
}

func TestBuildTelegramRegistrationAttribution_MalformedNormalized(t *testing.T) {
	cfg := testTelegramAttrCfg("https://connect.vpn-for-friends.com")
	rec, err := buildTelegramRegistrationAttribution(cfg, "a"+strings.Repeat("x", 500)+"\u0001", time.Now().UTC())
	if err != nil {
		t.Fatalf("malformed payload must not fail: %v", err)
	}
	if !rec.Valid() {
		t.Fatal("want valid normalized")
	}
}

func TestBuildTelegramRegistrationAttribution_InvalidServerContext(t *testing.T) {
	fixed := time.Now().UTC()
	if _, err := buildTelegramRegistrationAttribution(nil, "x", fixed); err == nil {
		t.Fatal("nil cfg")
	}
	cfg := testTelegramAttrCfg("")
	if _, err := buildTelegramRegistrationAttribution(cfg, "x", fixed); err == nil {
		t.Fatal("empty public_base_url")
	}
	cfg.Brand.PublicBaseURL = "://bad"
	if _, err := buildTelegramRegistrationAttribution(cfg, "x", fixed); err == nil {
		t.Fatal("invalid URL")
	}
	cfg.Brand.PublicBaseURL = "http:///nohost"
	if _, err := buildTelegramRegistrationAttribution(cfg, "x", fixed); err == nil {
		t.Fatal("URL without hostname")
	}
}

func TestPendingTelegramAttribution_FirstTouchSemantics(t *testing.T) {
	s := NewService(nil, testTelegramAttrCfg("https://connect.vpn-for-friends.com"))
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	chat := int64(1001)

	organic, err := buildTelegramRegistrationAttribution(s.config, "", now)
	if err != nil {
		t.Fatal(err)
	}
	taggedA, err := buildTelegramRegistrationAttribution(s.config, "campaign_a", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	taggedB, err := buildTelegramRegistrationAttribution(s.config, "campaign_b", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	s.rememberTelegramAttribution(chat, organic, now)
	s.rememberTelegramAttribution(chat, taggedA, now.Add(time.Minute))
	got, ok := s.peekTelegramAttribution(chat, now.Add(2*time.Minute))
	if !ok || !attribution.Equal(got, organic) {
		t.Fatalf("organic must not be overwritten: ok=%v got=%#v", ok, got)
	}

	s.clearTelegramAttribution(chat)
	s.rememberTelegramAttribution(chat, taggedA, now)
	s.rememberTelegramAttribution(chat, taggedB, now.Add(time.Minute))
	got, ok = s.peekTelegramAttribution(chat, now.Add(2*time.Minute))
	if !ok || !attribution.Equal(got, taggedA) {
		t.Fatalf("campaign A must stick: %#v", got)
	}
}

func TestPendingTelegramAttribution_ExpiredReplacedAndCleared(t *testing.T) {
	s := NewService(nil, testTelegramAttrCfg("https://connect.vpn-for-friends.com"))
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	chat := int64(2002)

	first, _ := buildTelegramRegistrationAttribution(s.config, "old", now)
	s.rememberTelegramAttribution(chat, first, now)
	// Force expiry.
	s.telegramAttributionMu.Lock()
	p := s.telegramAttributionPending[chat]
	p.ExpiresAt = now.Add(time.Hour)
	s.telegramAttributionPending[chat] = p
	s.telegramAttributionMu.Unlock()

	if _, ok := s.peekTelegramAttribution(chat, now.Add(2*time.Hour)); ok {
		t.Fatal("expired must not be returned")
	}

	second, _ := buildTelegramRegistrationAttribution(s.config, "new", now.Add(3*time.Hour))
	s.rememberTelegramAttribution(chat, second, now.Add(3*time.Hour))
	got, ok := s.peekTelegramAttribution(chat, now.Add(3*time.Hour))
	if !ok || !attribution.Equal(got, second) {
		t.Fatalf("expired may be replaced: %#v", got)
	}

	s.clearTelegramAttribution(chat)
	if _, ok := s.peekTelegramAttribution(chat, now.Add(3*time.Hour)); ok {
		t.Fatal("clear must remove")
	}
}

func TestPendingTelegramAttribution_ChatIsolationAndConcurrent(t *testing.T) {
	s := NewService(nil, testTelegramAttrCfg("https://connect.vpn-for-friends.com"))
	now := time.Now().UTC()
	a, _ := buildTelegramRegistrationAttribution(s.config, "alpha", now)
	b, _ := buildTelegramRegistrationAttribution(s.config, "beta", now)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.rememberTelegramAttribution(11, a, now)
			_, _ = s.peekTelegramAttribution(11, now)
		}()
		go func() {
			defer wg.Done()
			s.rememberTelegramAttribution(22, b, now)
			_, _ = s.peekTelegramAttribution(22, now)
		}()
	}
	wg.Wait()

	gotA, okA := s.peekTelegramAttribution(11, now)
	gotB, okB := s.peekTelegramAttribution(22, now)
	if !okA || !okB || !attribution.Equal(gotA, a) || !attribution.Equal(gotB, b) {
		t.Fatalf("isolation failed: A=%#v B=%#v", gotA, gotB)
	}
}

func TestPendingTelegramAttribution_FailureKeepsRecord(t *testing.T) {
	s := NewService(nil, testTelegramAttrCfg("https://connect.vpn-for-friends.com"))
	now := time.Now().UTC()
	rec, _ := buildTelegramRegistrationAttribution(s.config, "keep_me", now)
	s.rememberTelegramAttribution(33, rec, now)
	// Simulate registration failure: do not clear.
	got, ok := s.peekTelegramAttribution(33, now)
	if !ok || !attribution.Equal(got, rec) {
		t.Fatal("pending must remain after failed registration")
	}
	s.clearTelegramAttribution(33) // success path
	if _, ok := s.peekTelegramAttribution(33, now); ok {
		t.Fatal("success clear removes pending")
	}
}
