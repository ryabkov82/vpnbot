package web

import (
	"testing"
	"time"
)

func TestCreateAndVerifyAccountToken(t *testing.T) {
	secret := "account-token-secret-acc-tok-xx"
	em := "web-test@example.com"
	tok, err := CreateAccountToken(secret, em, 511, "web_abcde", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ParseAndVerifyAccountToken(secret, tok)
	if err != nil || cl.Email != em || cl.UserID != 511 || cl.Login != "web_abcde" {
		t.Fatalf("%+v err=%v", cl, err)
	}
}

func TestAccountTokenExpired(t *testing.T) {
	tok, err := CreateAccountToken("sec-sec-sec-sec-sec-x", "a@b.c", 1, "web_z", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_, err = ParseAndVerifyAccountToken("sec-sec-sec-sec-sec-x", tok)
	if err != ErrAccountTokenExpired {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestAccountTokenWrongTyp(t *testing.T) {
	secret := "order-token-secret-order-token-sec"
	orderTok, err := CreateOrderToken(secret, "e@f.g", 2, 10, 20, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountToken(secret, orderTok)
	if err != ErrAccountTokenType {
		t.Fatalf("want type err, got %v", err)
	}
}

func TestAccountTokenWrongSignature(t *testing.T) {
	tok, err := CreateAccountToken("aaa", "a@b.c", 5, "web_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerifyAccountToken("bbb", tok)
	if err != ErrAccountTokenSignature {
		t.Fatalf("got %v", err)
	}
}
