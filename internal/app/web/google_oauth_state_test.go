package web

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
)

func mustGoogleAttr(t *testing.T, domain string, m attribution.MarketingInput) attribution.Record {
	t.Helper()
	if domain == "" {
		domain = "connect.vpn-for-friends.com"
	}
	rec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelWebGoogle,
		RegistrationDomain:  domain,
		CapturedAt:          time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC),
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestGoogleOAuthState_LoginRoundTrip(t *testing.T) {
	secret := strings.Repeat("a", 40)
	attr := mustGoogleAttr(t, "connect.friends-connect.club", attribution.MarketingInput{
		LandingPath: "/account",
		Referrer:    "https://friends-connect.club/x",
		UTMSource:   "telegram",
		UTMCampaign: "summer",
	})
	state, err := createGoogleOAuthLoginState(secret, "fc", attr, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(state, googleOAuthStatePrefix) {
		t.Fatalf("prefix: %q", state)
	}
	cl, err := parseAndVerifyGoogleOAuthState(secret, "fc", state)
	if err != nil {
		t.Fatal(err)
	}
	if cl.Mode != googleOAuthModeLogin || cl.BrandID != "fc" || cl.Nonce == "" {
		t.Fatalf("%+v", cl)
	}
	if cl.Attribution == nil || !attribution.Equal(*cl.Attribution, attr) {
		t.Fatalf("attr %#v", cl.Attribution)
	}
}

func TestGoogleOAuthState_LinkRoundTripNoAttribution(t *testing.T) {
	secret := strings.Repeat("b", 40)
	state, err := createGoogleOAuthLinkState(secret, "vff", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := parseAndVerifyGoogleOAuthState(secret, "vff", state)
	if err != nil {
		t.Fatal(err)
	}
	if cl.Mode != googleOAuthModeLink || cl.Attribution != nil {
		t.Fatalf("%+v", cl)
	}
}

func TestGoogleOAuthState_Rejects(t *testing.T) {
	secret := strings.Repeat("c", 40)
	attr := mustGoogleAttr(t, "", attribution.MarketingInput{})

	t.Run("tampered", func(t *testing.T) {
		state, err := createGoogleOAuthLoginState(secret, "vff", attr, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(strings.TrimPrefix(state, googleOAuthStatePrefix), ".")
		payload, _ := base64.RawURLEncoding.DecodeString(parts[0])
		var m map[string]any
		_ = json.Unmarshal(payload, &m)
		m["mode"] = googleOAuthModeLink
		newPayload, _ := json.Marshal(m)
		tampered := googleOAuthStatePrefix + base64.RawURLEncoding.EncodeToString(newPayload) + "." + parts[1]
		if _, err := parseAndVerifyGoogleOAuthState(secret, "vff", tampered); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("wrong_brand", func(t *testing.T) {
		state, _ := createGoogleOAuthLoginState(secret, "vff", attr, time.Hour)
		if _, err := parseAndVerifyGoogleOAuthState(secret, "fc", state); err == nil {
			t.Fatal("want brand error")
		}
	})

	t.Run("expired", func(t *testing.T) {
		state, err := createGoogleOAuthLoginState(secret, "vff", attr, time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		if _, err := parseAndVerifyGoogleOAuthState(secret, "vff", state); err == nil {
			t.Fatal("want expired")
		}
	})

	t.Run("login_without_attribution", func(t *testing.T) {
		payload := googleOAuthStateClaims{
			Typ: googleOAuthStateTyp, BrandID: "vff", Mode: googleOAuthModeLogin,
			Nonce: "n", Exp: time.Now().Add(time.Hour).Unix(),
		}
		raw, _ := json.Marshal(payload)
		signed, _ := signAndEncodeAccountPayload(secret, raw)
		if _, err := parseAndVerifyGoogleOAuthState(secret, "vff", googleOAuthStatePrefix+signed); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("link_with_attribution", func(t *testing.T) {
		cp := attr
		payload := googleOAuthStateClaims{
			Typ: googleOAuthStateTyp, BrandID: "vff", Mode: googleOAuthModeLink,
			Nonce: "n", Attribution: &cp, Exp: time.Now().Add(time.Hour).Unix(),
		}
		raw, _ := json.Marshal(payload)
		signed, _ := signAndEncodeAccountPayload(secret, raw)
		if _, err := parseAndVerifyGoogleOAuthState(secret, "vff", googleOAuthStatePrefix+signed); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("invalid_attribution", func(t *testing.T) {
		bad := attribution.Record{Version: 1}
		if _, err := createGoogleOAuthLoginState(secret, "vff", bad, time.Hour); err == nil {
			t.Fatal("create must reject invalid record")
		}
	})

	t.Run("empty_nonce", func(t *testing.T) {
		cp := attr
		payload := googleOAuthStateClaims{
			Typ: googleOAuthStateTyp, BrandID: "vff", Mode: googleOAuthModeLogin,
			Attribution: &cp, Exp: time.Now().Add(time.Hour).Unix(),
		}
		raw, _ := json.Marshal(payload)
		signed, _ := signAndEncodeAccountPayload(secret, raw)
		if _, err := parseAndVerifyGoogleOAuthState(secret, "vff", googleOAuthStatePrefix+signed); err == nil {
			t.Fatal("want empty nonce error")
		}
	})

	t.Run("empty_secret", func(t *testing.T) {
		if _, err := createGoogleOAuthLoginState("", "vff", attr, time.Hour); err == nil {
			t.Fatal("want empty secret error")
		}
	})

	t.Run("unknown_mode", func(t *testing.T) {
		cp := attr
		payload := googleOAuthStateClaims{
			Typ: googleOAuthStateTyp, BrandID: "vff", Mode: "other",
			Nonce: "n", Attribution: &cp, Exp: time.Now().Add(time.Hour).Unix(),
		}
		raw, _ := json.Marshal(payload)
		signed, _ := signAndEncodeAccountPayload(secret, raw)
		if _, err := parseAndVerifyGoogleOAuthState(secret, "vff", googleOAuthStatePrefix+signed); err == nil {
			t.Fatal("want unknown mode error")
		}
	})

	t.Run("unknown_typ", func(t *testing.T) {
		cp := attr
		payload := googleOAuthStateClaims{
			Typ: "account", BrandID: "vff", Mode: googleOAuthModeLogin,
			Nonce: "n", Attribution: &cp, Exp: time.Now().Add(time.Hour).Unix(),
		}
		raw, _ := json.Marshal(payload)
		signed, _ := signAndEncodeAccountPayload(secret, raw)
		if _, err := parseAndVerifyGoogleOAuthState(secret, "vff", googleOAuthStatePrefix+signed); err == nil {
			t.Fatal("want typ error")
		}
	})
}

func TestGoogleOAuthState_LegacyNotParsedAsSigned(t *testing.T) {
	legacy, err := newGoogleOAuthState()
	if err != nil {
		t.Fatal(err)
	}
	if !isLegacyGoogleOAuthState(legacy) {
		t.Fatal("want legacy")
	}
	if _, err := parseAndVerifyGoogleOAuthState(strings.Repeat("d", 40), "vff", legacy); err == nil {
		t.Fatal("legacy must not parse as signed")
	}
}

func TestGoogleOAuthState_SizeBound(t *testing.T) {
	cfg := testGoogleOAuthMinimalCfg(strings.Repeat("e", 40), true, "cid", "https://cb/x", "sec")
	prev := googleOAuthMaxStateBytesOverride
	googleOAuthMaxStateBytesOverride = 500
	t.Cleanup(func() { googleOAuthMaxStateBytesOverride = prev })

	state, rec, err := createGoogleOAuthLoginStateForStart(cfg, attribution.MarketingInput{
		LandingPath: "/account",
		UTMSource:   "telegram",
		UTMCampaign: "summer",
	}, time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(state) > googleOAuthMaxStateBytesOverride {
		t.Fatalf("len=%d", len(state))
	}
	if !rec.IsOrganic() {
		t.Fatalf("fallback organic expected: %#v", rec)
	}
}
