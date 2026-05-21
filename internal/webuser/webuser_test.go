package webuser

import (
	"errors"
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
