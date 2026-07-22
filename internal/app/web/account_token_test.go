package web

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

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
	signup, err := CreateAccountSignupToken(secret, "vff", "a@b.c", "web_z", time.Hour)
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
	tok, err := CreateAccountSignupToken(secret, "vff", em, login, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ParseAndVerifyAccountSignupToken(secret, "vff", tok)
	if err != nil || cl.Email != em || cl.Login != login || cl.Typ != accountTokenTypSignup {
		t.Fatalf("%+v err=%v", cl, err)
	}
}

func TestAccountSignupTokenExpired(t *testing.T) {
	tok, err := CreateAccountSignupToken("su-su-su-su-su", "vff", "a@b.c", "web_z", time.Millisecond)
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
	tok, err := CreateAccountSignupToken("aaa", "vff", "a@b.c", "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountSignupToken("bbb", "vff", tok)
	if err != ErrAccountTokenSignature {
		t.Fatalf("got %v", err)
	}
}
