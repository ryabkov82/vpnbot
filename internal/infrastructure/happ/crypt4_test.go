package happ

import (
	"encoding/base64"
	"strings"
	"testing"
)

func assertValidCrypt4Link(t *testing.T, link string) {
	t.Helper()
	if !strings.HasPrefix(link, crypt4Prefix) {
		t.Fatalf("prefix: got %q want prefix %q", link, crypt4Prefix)
	}
	payload := strings.TrimPrefix(link, crypt4Prefix)
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	if len(raw) != rsa4096Size {
		t.Fatalf("ciphertext len=%d want %d", len(raw), rsa4096Size)
	}
}

func TestCreateCrypt4LinkHTTPS(t *testing.T) {
	link, err := CreateCrypt4Link("https://sub.example.com/abc")
	if err != nil {
		t.Fatal(err)
	}
	assertValidCrypt4Link(t, link)
}

func TestCreateCrypt4LinkEmpty(t *testing.T) {
	if _, err := CreateCrypt4Link(""); err == nil {
		t.Fatal("expected error for empty URL")
	}
	if _, err := CreateCrypt4Link("   "); err == nil {
		t.Fatal("expected error for whitespace URL")
	}
}

func TestCreateCrypt4LinkNonAbsolute(t *testing.T) {
	for _, raw := range []string{"not-a-url", "example.com/sub", "/relative", "https://"} {
		if _, err := CreateCrypt4Link(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestCreateCrypt4LinkTooLong(t *testing.T) {
	base := "https://example.com/"
	tooLong := base + strings.Repeat("a", 501-len(base)+1)
	if len(tooLong) <= 501 {
		t.Fatalf("test URL len=%d, want > 501", len(tooLong))
	}
	if _, err := CreateCrypt4Link(tooLong); err == nil {
		t.Fatal("expected error for plaintext > 501 bytes")
	}
}

func TestCreateCrypt4LinkMaxPlaintext(t *testing.T) {
	base := "https://example.com/"
	maxURL := base + strings.Repeat("a", 501-len(base))
	if len(maxURL) != 501 {
		t.Fatalf("test URL len=%d want 501", len(maxURL))
	}
	link, err := CreateCrypt4Link(maxURL)
	if err != nil {
		t.Fatal(err)
	}
	assertValidCrypt4Link(t, link)
}
