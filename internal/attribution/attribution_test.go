package attribution

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRegistrationChannelValid(t *testing.T) {
	for _, c := range []RegistrationChannel{
		RegistrationChannelTelegram,
		RegistrationChannelWebMagicLink,
		RegistrationChannelWebGoogle,
	} {
		if !c.Valid() {
			t.Fatalf("%q should be valid", c)
		}
	}
	for _, c := range []RegistrationChannel{"", "public_lead", "buy", "admin", "telegram_link", "session", "login", "other"} {
		if c.Valid() {
			t.Fatalf("%q must be invalid", c)
		}
	}
}

func TestNewFirstTouch_WebMagicLinkExact(t *testing.T) {
	fixed := time.Date(2026, 7, 26, 18, 0, 0, 123456789, time.UTC)
	rec, err := NewFirstTouch(ServerContext{
		RegistrationChannel: RegistrationChannelWebMagicLink,
		RegistrationDomain:  "connect.friends-connect.club",
		CapturedAt:          fixed,
	}, MarketingInput{
		LandingPath: "/account?utm_source=telegram",
		Referrer:    "https://friends-connect.club/articles/post?id=1",
		UTMSource:   "telegram",
		UTMMedium:   "post",
		UTMCampaign: "summer",
		UTMContent:  "",
		UTMTerm:     "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Version != SchemaVersion {
		t.Fatalf("version=%d", rec.Version)
	}
	ft := rec.FirstTouch
	if ft.RegistrationChannel != RegistrationChannelWebMagicLink {
		t.Fatalf("channel=%q", ft.RegistrationChannel)
	}
	if ft.RegistrationDomain != "connect.friends-connect.club" {
		t.Fatalf("domain=%q", ft.RegistrationDomain)
	}
	if ft.LandingPath != "/account" {
		t.Fatalf("landing=%q", ft.LandingPath)
	}
	if ft.ReferrerHost != "friends-connect.club" {
		t.Fatalf("referrer=%q", ft.ReferrerHost)
	}
	if ft.UTMSource != "telegram" || ft.UTMMedium != "post" || ft.UTMCampaign != "summer" {
		t.Fatalf("utm=%+v", ft)
	}
	if ft.CapturedAt != "2026-07-26T18:00:00Z" {
		t.Fatalf("captured_at=%q", ft.CapturedAt)
	}
	if !rec.Valid() {
		t.Fatal("record must be valid")
	}
	if rec.IsOrganic() {
		t.Fatal("UTM present → not organic")
	}
}

func TestNewFirstTouch_GoogleAndTelegram(t *testing.T) {
	ts := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	g, err := NewFirstTouch(ServerContext{
		RegistrationChannel: RegistrationChannelWebGoogle,
		RegistrationDomain:  "connect.vpn-for-friends.com",
		CapturedAt:          ts,
	}, MarketingInput{})
	if err != nil {
		t.Fatal(err)
	}
	if g.FirstTouch.RegistrationChannel != RegistrationChannelWebGoogle {
		t.Fatalf("got %q", g.FirstTouch.RegistrationChannel)
	}

	tg, err := NewFirstTouch(ServerContext{
		RegistrationChannel: RegistrationChannelTelegram,
		RegistrationDomain:  "connect.friends-connect.club",
		CapturedAt:          ts,
	}, MarketingInput{TelegramStartParam: "promo42"})
	if err != nil {
		t.Fatal(err)
	}
	if tg.FirstTouch.RegistrationChannel != RegistrationChannelTelegram {
		t.Fatalf("got %q", tg.FirstTouch.RegistrationChannel)
	}
	if tg.FirstTouch.TelegramStartParam != "promo42" {
		t.Fatalf("start=%q", tg.FirstTouch.TelegramStartParam)
	}
	if tg.IsOrganic() {
		t.Fatal("telegram start param → not organic")
	}
}

func TestNewFirstTouch_ServerValidation(t *testing.T) {
	ts := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	base := ServerContext{
		RegistrationChannel: RegistrationChannelWebMagicLink,
		RegistrationDomain:  "connect.example.com",
		CapturedAt:          ts,
	}
	cases := []struct {
		name string
		mut  func(*ServerContext)
	}{
		{name: "empty_channel", mut: func(s *ServerContext) { s.RegistrationChannel = "" }},
		{name: "unknown_channel", mut: func(s *ServerContext) { s.RegistrationChannel = "public_lead" }},
		{name: "empty_domain", mut: func(s *ServerContext) { s.RegistrationDomain = "" }},
		{name: "scheme", mut: func(s *ServerContext) { s.RegistrationDomain = "https://connect.example.com" }},
		{name: "path", mut: func(s *ServerContext) { s.RegistrationDomain = "connect.example.com/account" }},
		{name: "port", mut: func(s *ServerContext) { s.RegistrationDomain = "connect.example.com:443" }},
		{name: "ip", mut: func(s *ServerContext) { s.RegistrationDomain = "203.0.113.10" }},
		{name: "bad_label", mut: func(s *ServerContext) { s.RegistrationDomain = "-bad.example.com" }},
		{name: "zero_time", mut: func(s *ServerContext) { s.CapturedAt = time.Time{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.mut(&s)
			if _, err := NewFirstTouch(s, MarketingInput{}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewFirstTouch_DomainNormalization(t *testing.T) {
	ts := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "Connect.Friends-Connect.Club", want: "connect.friends-connect.club"},
		{in: "connect.friends-connect.club.", want: "connect.friends-connect.club"},
		{in: "  connect.example.com  ", want: "connect.example.com"},
		{in: "localhost", want: "localhost"},
	} {
		rec, err := NewFirstTouch(ServerContext{
			RegistrationChannel: RegistrationChannelTelegram,
			RegistrationDomain:  tc.in,
			CapturedAt:          ts,
		}, MarketingInput{})
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if rec.FirstTouch.RegistrationDomain != tc.want {
			t.Fatalf("%q → %q want %q", tc.in, rec.FirstTouch.RegistrationDomain, tc.want)
		}
	}
}

func TestNewFirstTouch_MarketingSafety(t *testing.T) {
	ts := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	server := ServerContext{
		RegistrationChannel: RegistrationChannelWebMagicLink,
		RegistrationDomain:  "connect.example.com",
		CapturedAt:          ts,
	}

	rec, err := NewFirstTouch(server, MarketingInput{
		LandingPath:        "/account\r\n#x",
		Referrer:           "https://Example.COM:8443/path?q=1#f",
		UTMSource:          "a\x00b\nc",
		TelegramStartParam: "ok\x1fparam",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Landing with CR/LF sanitized then parsed — after strip controls becomes "/account#x" → path /account fragment dropped via Parse.
	if rec.FirstTouch.LandingPath != "/account" {
		t.Fatalf("landing=%q", rec.FirstTouch.LandingPath)
	}
	if rec.FirstTouch.ReferrerHost != "example.com" {
		t.Fatalf("referrer=%q", rec.FirstTouch.ReferrerHost)
	}
	if strings.ContainsAny(rec.FirstTouch.UTMSource, "\x00\n\r") {
		t.Fatalf("utm still has controls: %q", rec.FirstTouch.UTMSource)
	}
	if strings.ContainsAny(rec.FirstTouch.TelegramStartParam, "\x1f") {
		t.Fatalf("start param controls: %q", rec.FirstTouch.TelegramStartParam)
	}

	drop, err := NewFirstTouch(server, MarketingInput{
		LandingPath: "https://evil.test/path",
		Referrer:    "javascript:alert(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if drop.FirstTouch.LandingPath != "" || drop.FirstTouch.ReferrerHost != "" {
		t.Fatalf("unsafe values must drop: %+v", drop.FirstTouch)
	}

	drop2, err := NewFirstTouch(server, MarketingInput{
		LandingPath: "javascript:alert(1)",
		Referrer:    "ftp://files.example.com/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if drop2.FirstTouch.LandingPath != "" || drop2.FirstTouch.ReferrerHost != "" {
		t.Fatalf("non-http/external must drop: %+v", drop2.FirstTouch)
	}

	ipRef, err := NewFirstTouch(server, MarketingInput{Referrer: "https://203.0.113.10/x"})
	if err != nil {
		t.Fatal(err)
	}
	if ipRef.FirstTouch.ReferrerHost != "" {
		t.Fatalf("IP referrer must drop, got %q", ipRef.FirstTouch.ReferrerHost)
	}

	long := strings.Repeat("я", MaxUTMCampaignRunes+20)
	trunc, err := NewFirstTouch(server, MarketingInput{UTMCampaign: long})
	if err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(trunc.FirstTouch.UTMCampaign) != MaxUTMCampaignRunes {
		t.Fatalf("rune count=%d", utf8.RuneCountInString(trunc.FirstTouch.UTMCampaign))
	}

	frag, err := NewFirstTouch(server, MarketingInput{LandingPath: "/account#section"})
	if err != nil {
		t.Fatal(err)
	}
	if frag.FirstTouch.LandingPath != "/account" {
		t.Fatalf("fragment landing=%q", frag.FirstTouch.LandingPath)
	}
}

func TestSemantics_OrganicUnknown(t *testing.T) {
	ts := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	organic, err := NewFirstTouch(ServerContext{
		RegistrationChannel: RegistrationChannelWebMagicLink,
		RegistrationDomain:  "connect.example.com",
		CapturedAt:          ts,
	}, MarketingInput{
		LandingPath: "/account",
		Referrer:    "https://news.example.com/a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !organic.IsOrganic() {
		t.Fatal("empty UTM/start with referrer-only must be organic")
	}
	if organic.FirstTouch.ReferrerHost == "" {
		t.Fatal("referrer host should persist")
	}

	withUTM := organic
	withUTM.FirstTouch.UTMSource = "ads"
	if withUTM.IsOrganic() {
		t.Fatal("UTM → not organic")
	}

	invalid := Record{Version: 2}
	if invalid.Valid() || invalid.IsOrganic() {
		t.Fatal("invalid record must not be organic")
	}

	// Absence of Record (= unknown) is a storage concern; package has no VFF default.
	var zero Record
	if zero.IsOrganic() {
		t.Fatal("zero record must not be treated as organic/VFF")
	}
}

func TestJSON_RoundTripContract(t *testing.T) {
	ts := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	rec, err := NewFirstTouch(ServerContext{
		RegistrationChannel: RegistrationChannelWebMagicLink,
		RegistrationDomain:  "connect.friends-connect.club",
		CapturedAt:          ts,
	}, MarketingInput{
		LandingPath: "/account",
		UTMSource:   "telegram",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, key := range []string{
		`"version"`,
		`"first_touch"`,
		`"registration_channel"`,
		`"registration_domain"`,
		`"landing_path"`,
		`"utm_source"`,
		`"captured_at"`,
	} {
		if !strings.Contains(s, key) {
			t.Fatalf("missing key %s in %s", key, s)
		}
	}
	// omitempty: empty optional fields absent
	for _, absent := range []string{
		`"utm_medium"`,
		`"utm_campaign"`,
		`"utm_content"`,
		`"utm_term"`,
		`"referrer_host"`,
		`"telegram_start_param"`,
		`"email"`,
		`"user_id"`,
		`"telegram_id"`,
		`"chat_id"`,
		`"ip_address"`,
		`"user_agent"`,
		`"brand_id"`,
	} {
		if strings.Contains(s, absent) {
			t.Fatalf("must not contain %s in %s", absent, s)
		}
	}
	if !strings.Contains(s, `"version":1`) {
		t.Fatalf("version: %s", s)
	}

	var back Record
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !Equal(rec, back) {
		t.Fatalf("round-trip mismatch\n%+v\n%+v", rec, back)
	}
}

func TestEqual(t *testing.T) {
	ts := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	a, err := NewFirstTouch(ServerContext{
		RegistrationChannel: RegistrationChannelTelegram,
		RegistrationDomain:  "connect.example.com",
		CapturedAt:          ts,
	}, MarketingInput{})
	if err != nil {
		t.Fatal(err)
	}
	b := a
	if !Equal(a, b) {
		t.Fatal("equal records")
	}
	b.FirstTouch.UTMSource = "x"
	if Equal(a, b) {
		t.Fatal("differing field must not be equal")
	}
}
