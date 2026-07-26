package web

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestCreateAndVerifyAccountToken(t *testing.T) {
	secret := "account-token-secret-acc-tok-xx"
	em := "web-test@example.com"
	tok, err := CreateAccountToken(secret, "vff", em, 511, "web_abcde", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ParseAndVerifyAccountToken(secret, "vff", tok)
	if err != nil || cl.Email != em || cl.UserID != 511 || cl.Login != "web_abcde" || cl.BrandID != "vff" {
		t.Fatalf("%+v err=%v", cl, err)
	}
	raw, _ := json.Marshal(cl)
	if strings.Contains(string(raw), "attribution") {
		t.Fatalf("account token must not contain attribution: %s", raw)
	}
}

func TestAccountTokenMissingBrandRejected(t *testing.T) {
	secret := "account-token-secret-acc-tok-xx"
	payload := AccountTokenClaims{
		Typ: accountTokenTypAccount, Email: "a@b.c", UserID: 1, Login: "web_x",
		Exp: time.Now().Add(time.Hour).Unix(),
	}
	raw, _ := json.Marshal(payload)
	tok, err := signAndEncodeAccountPayload(secret, raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountToken(secret, "vff", tok)
	if err != ErrAccountTokenBrand {
		t.Fatalf("got %v", err)
	}
}

func TestAccountTokenWrongBrandRejected(t *testing.T) {
	secret := "account-token-secret-acc-tok-xx"
	tok, err := CreateAccountToken(secret, "vff", "a@b.c", 2, "web_y", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountToken(secret, "fc", tok)
	if err != ErrAccountTokenBrand {
		t.Fatalf("got %v", err)
	}
}

func TestSignupAndLinkTokensWrongBrandRejected(t *testing.T) {
	secret := "account-token-secret-acc-tok-xx"
	cfg := &config.Config{}
	signup, err := CreateAccountSignupToken(secret, "vff", "a@b.c", "web_z", mustMagicLinkAttribution(t, "", attribution.MarketingInput{}), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAndVerifyAccountSignupToken(secret, "fc", signup); err != ErrAccountTokenBrand {
		t.Fatalf("signup: %v", err)
	}
	tg, err := CreateAccountTelegramLinkToken(secret, "vff", 9, 100, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAccountTelegramLinkToken(secret, "fc", tg); err != ErrAccountTokenBrand {
		t.Fatalf("tg link: %v", err)
	}
	em, err := CreateAccountLinkEmailToken(secret, "vff", 9, 100, "a@b.c", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAccountLinkEmailToken(secret, "fc", em); err != ErrAccountTokenBrand {
		t.Fatalf("email link: %v", err)
	}
}

func TestAccountTokenExpired(t *testing.T) {
	tok, err := CreateAccountToken("sec-sec-sec-sec-sec-x", "vff", "a@b.c", 1, "web_z", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_, err = ParseAndVerifyAccountToken("sec-sec-sec-sec-sec-x", "vff", tok)
	if err != ErrAccountTokenExpired {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestAccountTokenWrongTyp(t *testing.T) {
	secret := "order-token-secret-order-token-sec"
	payload := AccountTokenClaims{Typ: "order", Email: "e@f.g", UserID: 10, Login: "x", Exp: time.Now().Add(time.Hour).Unix()}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	encPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := signOrderTokenPayload([]byte(secret), payloadJSON)
	encSig := base64.RawURLEncoding.EncodeToString(sig)
	token := encPayload + "." + encSig
	_, err = ParseAndVerifyAccountToken(secret, "vff", token)
	if err != ErrAccountTokenType {
		t.Fatalf("want type err, got %v", err)
	}
}

func TestAccountTokenWrongSignature(t *testing.T) {
	tok, err := CreateAccountToken("aaa", "vff", "a@b.c", 5, "web_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountToken("bbb", "vff", tok)
	if err != ErrAccountTokenSignature {
		t.Fatalf("got %v", err)
	}
}

func TestWebSalesOrderTokenTTLDefault(t *testing.T) {
	if webSalesOrderTokenTTL(nil) != 24*time.Hour {
		t.Fatal("nil cfg ttl")
	}
	cfg := &config.Config{}
	cfg.WebSales.OrderTokenTTLHours = 48
	if webSalesOrderTokenTTL(cfg) != 48*time.Hour {
		t.Fatal("custom ttl")
	}
}

func TestCreateAndVerifyAccountSignupToken(t *testing.T) {
	secret := "signup-account-secret-xxxx"
	em := "new-user@example.com"
	login := "web_abcdef9012345678"
	attr := mustMagicLinkAttribution(t, "shop.example", attribution.MarketingInput{
		LandingPath: "/account",
		Referrer:    "https://vpn-for-friends.com/x",
		UTMSource:   "telegram",
		UTMMedium:   "post",
		UTMCampaign: "summer",
	})
	tok, err := CreateAccountSignupToken(secret, "vff", em, login, attr, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ParseAndVerifyAccountSignupToken(secret, "vff", tok)
	if err != nil || cl.Email != em || cl.Login != login || cl.Typ != accountTokenTypSignup {
		t.Fatalf("%+v err=%v", cl, err)
	}
	if cl.Attribution == nil || !attribution.Equal(*cl.Attribution, attr) {
		t.Fatalf("attribution round-trip: got %#v want %#v", cl.Attribution, attr)
	}
	raw, _ := json.Marshal(cl.Attribution)
	s := string(raw)
	for _, banned := range []string{"user_agent", "ip_address", "full_url", "query_string", "https://vpn-for-friends.com/x"} {
		if strings.Contains(s, banned) {
			t.Fatalf("persisted/token record must not contain %q: %s", banned, s)
		}
	}
}

func TestCreateAccountSignupToken_InvalidRecordRejected(t *testing.T) {
	_, err := CreateAccountSignupToken("secret-secret-secret", "vff", "a@b.c", "web_z", attribution.Record{}, time.Hour)
	if err == nil {
		t.Fatal("want error for invalid record")
	}
}

func TestAccountSignupToken_TamperedAttributionBreaksSignature(t *testing.T) {
	secret := "signup-account-secret-xxxx"
	attr := mustMagicLinkAttribution(t, "shop.example", attribution.MarketingInput{UTMSource: "a"})
	tok, err := CreateAccountSignupToken(secret, "vff", "a@b.c", "web_z", attr, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		t.Fatalf("token parts: %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	attrMap, ok := claims["attribution"].(map[string]any)
	if !ok {
		t.Fatal("attribution missing")
	}
	ft, ok := attrMap["first_touch"].(map[string]any)
	if !ok {
		t.Fatal("first_touch missing")
	}
	ft["utm_source"] = "tampered"
	newPayload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	tampered := base64.RawURLEncoding.EncodeToString(newPayload) + "." + parts[1]
	_, err = ParseAndVerifyAccountSignupToken(secret, "vff", tampered)
	if err != ErrAccountTokenSignature {
		t.Fatalf("want signature error, got %v", err)
	}
}

func TestAccountSignupToken_MalformedAttributionRejected(t *testing.T) {
	secret := "signup-account-secret-xxxx"
	payload := AccountSignupTokenClaims{
		Typ: accountTokenTypSignup, BrandID: "vff", Email: "a@b.c", Login: "web_z",
		Attribution: &attribution.Record{Version: 1},
		Exp:         time.Now().Add(time.Hour).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := signAndEncodeAccountPayload(secret, raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountSignupToken(secret, "vff", tok)
	if err != ErrAccountTokenMalformed {
		t.Fatalf("want malformed, got %v", err)
	}
}

func TestAccountSignupToken_LegacyNilAttributionAccepted(t *testing.T) {
	secret := "signup-account-secret-xxxx"
	payload := AccountSignupTokenClaims{
		Typ: accountTokenTypSignup, BrandID: "vff", Email: "a@b.c", Login: "web_z",
		Exp: time.Now().Add(time.Hour).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := signAndEncodeAccountPayload(secret, raw)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ParseAndVerifyAccountSignupToken(secret, "vff", tok)
	if err != nil {
		t.Fatal(err)
	}
	if cl.Attribution != nil {
		t.Fatalf("legacy must keep nil attribution, got %#v", cl.Attribution)
	}
}

func TestAccountSignupTokenExpired(t *testing.T) {
	tok, err := CreateAccountSignupToken("su-su-su-su-su", "vff", "a@b.c", "web_z", mustMagicLinkAttribution(t, "", attribution.MarketingInput{}), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_, err = ParseAndVerifyAccountSignupToken("su-su-su-su-su", "vff", tok)
	if err != ErrAccountTokenExpired {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestAccountSignupTokenWrongTyp(t *testing.T) {
	secret := "order-token-secret-order-token-sec"
	payload := AccountSignupTokenClaims{Typ: "account", Email: "e@f.g", Login: "web_x", Exp: time.Now().Add(time.Hour).Unix()}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	encPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := signOrderTokenPayload([]byte(secret), payloadJSON)
	encSig := base64.RawURLEncoding.EncodeToString(sig)
	token := encPayload + "." + encSig
	_, err = ParseAndVerifyAccountSignupToken(secret, "vff", token)
	if err != ErrAccountTokenType {
		t.Fatalf("want type err, got %v", err)
	}
}

func TestAccountSignupTokenWrongSignature(t *testing.T) {
	tok, err := CreateAccountSignupToken("aaa", "vff", "a@b.c", "web_xx", mustMagicLinkAttribution(t, "", attribution.MarketingInput{}), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountSignupToken("bbb", "vff", tok)
	if err != ErrAccountTokenSignature {
		t.Fatalf("got %v", err)
	}
}
