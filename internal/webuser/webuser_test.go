package webuser

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeEmail_OK(t *testing.T) {
	got, err := NormalizeEmail("  User@Example.COM ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "user@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeEmail_Invalid(t *testing.T) {
	for _, in := range []string{"", "   ", "not-an-email", `Name <a@b.com>`} {
		_, err := NormalizeEmail(in)
		if !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("input %q: want ErrInvalidEmail, got %v", in, err)
		}
	}
}

func TestWebLoginFromEmail_Stability(t *testing.T) {
	a := WebLoginFromEmail("  User@Example.COM ")
	b := WebLoginFromEmail("user@example.com")
	if a != b {
		t.Fatalf("want equal logins, got %q vs %q", a, b)
	}
	if len(a) != len("web_")+16 {
		t.Fatalf("unexpected login length: %q len=%d", a, len(a))
	}
	if a[:4] != "web_" {
		t.Fatalf("prefix: %q", a)
	}
}

func TestWebLoginFromEmailWithPrefix_EmptyRejected(t *testing.T) {
	for _, prefix := range []string{"", "   ", "\t"} {
		_, err := WebLoginFromEmailWithPrefix("user@example.com", prefix)
		if !errors.Is(err, ErrWebLoginPrefixRequired) {
			t.Fatalf("prefix %q: want ErrWebLoginPrefixRequired, got %v", prefix, err)
		}
	}
}

func TestWebLoginFromEmailWithPrefix_ExplicitTrimmed(t *testing.T) {
	login, err := WebLoginFromEmailWithPrefix("user@example.com", " customer_ ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(login, "customer_") {
		t.Fatalf("login=%q, want customer_ prefix", login)
	}
	again, err := WebLoginFromEmailWithPrefix("user@example.com", "customer_")
	if err != nil {
		t.Fatal(err)
	}
	if login != again {
		t.Fatalf("login must be stable: %q vs %q", login, again)
	}
}

func TestWebLoginFromEmailWithPrefix_VFFCompatible(t *testing.T) {
	email := "user@example.com"
	legacy := WebLoginFromEmail(email)
	withPrefix, err := WebLoginFromEmailWithPrefix(email, "web_")
	if err != nil {
		t.Fatal(err)
	}
	if legacy != withPrefix {
		t.Fatalf("VFF prefix must match legacy byte-for-byte: %q vs %q", legacy, withPrefix)
	}
}

func TestWebLoginFromEmailWithPrefix_DifferentPrefixSameHash(t *testing.T) {
	email := "user@example.com"
	vff, err := WebLoginFromEmailWithPrefix(email, "web_")
	if err != nil {
		t.Fatal(err)
	}
	fc, err := WebLoginFromEmailWithPrefix(email, "web_fc_")
	if err != nil {
		t.Fatal(err)
	}
	if vff == fc {
		t.Fatal("different prefixes must produce different logins")
	}
	if !strings.HasPrefix(vff, "web_") || !strings.HasPrefix(fc, "web_fc_") {
		t.Fatalf("prefixes: %q %q", vff, fc)
	}
	vffHash := strings.TrimPrefix(vff, "web_")
	fcHash := strings.TrimPrefix(fc, "web_fc_")
	if vffHash != fcHash {
		t.Fatalf("hash part must be identical: %q vs %q", vffHash, fcHash)
	}
	if len(vffHash) != 16 {
		t.Fatalf("hash len: %d", len(vffHash))
	}
}
