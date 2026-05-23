package web

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCreateAndValidatePremiumAccessToken_OK(t *testing.T) {
	secret := "test-secret-key"
	tok, err := CreatePremiumAccessToken(secret, 424242, 9001, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidatePremiumAccessToken(secret, tok, 9001)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 424242 || claims.ServiceID != 9001 || claims.Exp <= time.Now().Unix() {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestValidatePremiumAccessToken_Expired(t *testing.T) {
	secret := "s"
	payload := PremiumAccessClaims{ServiceID: 1, UserID: 2, Exp: time.Now().Add(-time.Hour).Unix()}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString(b)
	sig := signPremiumPayload([]byte(secret), b)
	tok := enc + "." + base64.RawURLEncoding.EncodeToString(sig)
	_, err = ValidatePremiumAccessToken(secret, tok, 1)
	if !errors.Is(err, ErrPremiumTokenExpired) {
		t.Fatalf("want ErrPremiumTokenExpired, got %v", err)
	}
}

func TestCreatePremiumSHMAccessToken_ValidateRoundTrip(t *testing.T) {
	secret := "sh-web"
	tok, err := CreatePremiumSHMAccessToken(secret, 55, 1200, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ValidatePremiumAccessToken(secret, tok, 1200)
	if err != nil || cl.ShmUserID != 55 || cl.UserID != 0 || cl.ServiceID != 1200 {
		t.Fatalf("%+v %v", cl, err)
	}
}

func TestValidatePremiumAccessToken_InvalidPrincipalCombination(t *testing.T) {
	secret := "s-both"
	claims := PremiumAccessClaims{
		ServiceID: 1,
		UserID:    9,
		ShmUserID: 8,
		Exp:       time.Now().Add(time.Hour).Unix(),
	}
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString(b)
	sig := signPremiumPayload([]byte(secret), b)
	tok := enc + "." + base64.RawURLEncoding.EncodeToString(sig)
	_, err = ValidatePremiumAccessToken(secret, tok, 1)
	if err == nil {
		t.Fatal("want error for conflicting principals")
	}
}

func TestValidatePremiumAccessToken_WrongServiceID(t *testing.T) {
	secret := "s2"
	tok, err := CreatePremiumAccessToken(secret, 1, 100, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ValidatePremiumAccessToken(secret, tok, 101)
	if !errors.Is(err, ErrPremiumTokenService) {
		t.Fatalf("want ErrPremiumTokenService, got %v", err)
	}
}

func TestValidatePremiumAccessToken_CorruptedSignature(t *testing.T) {
	secret := "s3"
	tok, err := CreatePremiumAccessToken(secret, 9, 8, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		t.Fatal("expected two token parts")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) == 0 {
		t.Fatal("empty sig")
	}
	sig[0] ^= 0xff
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString(sig)
	_, err = ValidatePremiumAccessToken(secret, tampered, 8)
	if !errors.Is(err, ErrPremiumTokenSignature) {
		t.Fatalf("want ErrPremiumTokenSignature, got %v", err)
	}
}

func TestValidatePremiumAccessToken_EmptySecret(t *testing.T) {
	tok, err := CreatePremiumAccessToken("ok-secret", 1, 2, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ValidatePremiumAccessToken("", tok, 2)
	if !errors.Is(err, ErrPremiumTokenEmptySecret) {
		t.Fatalf("want ErrPremiumTokenEmptySecret, got %v", err)
	}
	_, err = CreatePremiumAccessToken("", 1, 2, time.Hour)
	if !errors.Is(err, ErrPremiumTokenEmptySecret) {
		t.Fatalf("create want ErrPremiumTokenEmptySecret, got %v", err)
	}
}

func TestValidatePremiumAccessToken_Malformed(t *testing.T) {
	_, err := ValidatePremiumAccessToken("sec", "nodot", 1)
	if !errors.Is(err, ErrPremiumTokenMalformed) {
		t.Fatalf("want ErrPremiumTokenMalformed, got %v", err)
	}
	_, err = ValidatePremiumAccessToken("sec", "a.", 1)
	if !errors.Is(err, ErrPremiumTokenMalformed) {
		t.Fatalf("want ErrPremiumTokenMalformed, got %v", err)
	}
}
