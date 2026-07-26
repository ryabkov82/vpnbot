package web

import (
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
)

func TestBuildWebMagicLinkAttribution_FCExact(t *testing.T) {
	cfg := friendsConnectAccountTestCfg()
	fixed := time.Date(2026, 7, 22, 15, 30, 0, 0, time.UTC)
	req := accountLoginStartRequestJSON{
		LandingPath: "/account?ignored=x",
		Referrer:    "https://friends-connect.club/post?id=1",
		UTMSource:   "telegram",
		UTMMedium:   "post",
		UTMCampaign: "summer",
	}
	rec, err := buildWebMagicLinkAttribution(cfg, req, fixed)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Valid() {
		t.Fatal("record must be valid")
	}
	ft := rec.FirstTouch
	if ft.RegistrationChannel != attribution.RegistrationChannelWebMagicLink {
		t.Fatalf("channel %q", ft.RegistrationChannel)
	}
	if ft.RegistrationDomain != "connect.friends-connect.club" {
		t.Fatalf("domain %q", ft.RegistrationDomain)
	}
	if ft.LandingPath != "/account" {
		t.Fatalf("landing_path %q", ft.LandingPath)
	}
	if ft.ReferrerHost != "friends-connect.club" {
		t.Fatalf("referrer_host %q", ft.ReferrerHost)
	}
	if ft.UTMSource != "telegram" || ft.UTMMedium != "post" || ft.UTMCampaign != "summer" {
		t.Fatalf("utm %#v", ft)
	}
	if ft.CapturedAt != "2026-07-22T15:30:00Z" {
		t.Fatalf("captured_at %q", ft.CapturedAt)
	}
}

func TestBuildWebMagicLinkAttribution_VFFDomainFromConfig(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.PublicBaseURL = "https://connect.vpn-for-friends.com"
	rec, err := buildWebMagicLinkAttribution(cfg, accountLoginStartRequestJSON{}, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rec.FirstTouch.RegistrationDomain != "connect.vpn-for-friends.com" {
		t.Fatalf("domain %q", rec.FirstTouch.RegistrationDomain)
	}
	if !rec.IsOrganic() {
		t.Fatal("empty marketing should be organic")
	}
}

func TestBuildWebMagicLinkAttribution_TrustBoundaryIgnoresHosts(t *testing.T) {
	cfg := friendsConnectAccountTestCfg()
	req := accountLoginStartRequestJSON{
		Referrer: "https://evil.example/steal",
	}
	rec, err := buildWebMagicLinkAttribution(cfg, req, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if rec.FirstTouch.RegistrationDomain != "connect.friends-connect.club" {
		t.Fatalf("registration_domain must come from config, got %q", rec.FirstTouch.RegistrationDomain)
	}
	if rec.FirstTouch.ReferrerHost != "evil.example" {
		t.Fatalf("referrer_host still normalized from marketing: %q", rec.FirstTouch.ReferrerHost)
	}
}

func TestBuildWebMagicLinkAttribution_MalformedMarketingNormalized(t *testing.T) {
	cfg := orderStartTestCfg()
	req := accountLoginStartRequestJSON{
		LandingPath: "https://external.example/account",
		Referrer:    ":::bad",
		UTMSource:   "a" + strings.Repeat("x", 500),
		UTMMedium:   "m\u0001ed",
		UTMCampaign: "ok",
	}
	rec, err := buildWebMagicLinkAttribution(cfg, req, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("malformed optional fields must not error: %v", err)
	}
	if !rec.Valid() {
		t.Fatal("want valid normalized record")
	}
	if rec.FirstTouch.LandingPath != "" {
		t.Fatalf("external landing_path must be dropped, got %q", rec.FirstTouch.LandingPath)
	}
	if rec.FirstTouch.RegistrationDomain != "shop.example" {
		t.Fatalf("domain %q", rec.FirstTouch.RegistrationDomain)
	}
}

func TestBuildWebMagicLinkAttribution_InvalidServerContext(t *testing.T) {
	fixed := time.Now().UTC()
	req := accountLoginStartRequestJSON{}

	if _, err := buildWebMagicLinkAttribution(nil, req, fixed); err == nil {
		t.Fatal("nil cfg")
	}
	cfg := orderStartTestCfg()
	cfg.Brand.PublicBaseURL = ""
	if _, err := buildWebMagicLinkAttribution(cfg, req, fixed); err == nil {
		t.Fatal("empty public_base_url")
	}
	cfg.Brand.PublicBaseURL = "://not-a-url"
	if _, err := buildWebMagicLinkAttribution(cfg, req, fixed); err == nil {
		t.Fatal("invalid URL")
	}
	cfg.Brand.PublicBaseURL = "http:///nohost"
	if _, err := buildWebMagicLinkAttribution(cfg, req, fixed); err == nil {
		t.Fatal("URL without hostname")
	}
}
