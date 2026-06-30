package payments

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildYooKassaPaymentURL_OK(t *testing.T) {
	const base = "https://example.com/"
	got, err := BuildYooKassaPaymentURL(base, 42, 199.0, 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/shm/pay_systems/yookassa.cgi" {
		t.Fatalf("path: %q", u.Path)
	}
	q := u.Query()
	if q.Get("action") != "create" || q.Get("user_id") != "42" || q.Get("ts") != "1700000000" || q.Get("ps") != "yookassa" || q.Get("amount") != "199" {
		t.Fatalf("query: %#v", q)
	}
	if !strings.HasPrefix(got, "https://example.com/shm/pay_systems/yookassa.cgi?") {
		t.Fatalf("prefix: %s", got)
	}
}

func TestBuildYooKassaPaymentURL_TrimsBaseSlash(t *testing.T) {
	got, err := BuildYooKassaPaymentURL("https://x.y/ ", 1, 10.5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://x.y/shm/pay_systems/yookassa.cgi") {
		t.Fatalf("got %s", got)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("amount") != "10.5" {
		t.Fatalf("amount: %q", u.Query().Get("amount"))
	}
}

func TestBuildYooKassaPaymentURL_FractionalAmount(t *testing.T) {
	got, err := BuildYooKassaPaymentURL("https://h", 9, 100.25, 0)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("amount") != "100.25" {
		t.Fatalf("amount: %q", u.Query().Get("amount"))
	}
}

func TestBuildCryptoCloudPaymentURL_OK(t *testing.T) {
	got, err := BuildCryptoCloudPaymentURL("https://bill.example/", 701, 150, 1700000001)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/shm/pay_systems/cryptocloud.cgi" {
		t.Fatalf("path: %q", u.Path)
	}
	q := u.Query()
	if q.Get("action") != "create" || q.Get("user_id") != "701" || q.Get("ts") != "1700000001" || q.Get("ps") != "cryptocloud" || q.Get("amount") != "150" {
		t.Fatalf("query: %#v", q)
	}
	if !strings.HasPrefix(got, "https://bill.example/shm/pay_systems/cryptocloud.cgi?") {
		t.Fatalf("prefix: %s", got)
	}
}

func TestBuildYooKassaPaymentURL_Validation(t *testing.T) {
	if _, err := BuildYooKassaPaymentURL("https://x", 0, 10, 1); err == nil {
		t.Fatal("want error for user id 0")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 0, 1); err == nil {
		t.Fatal("want error for amount 0")
	}
	if _, err := BuildYooKassaPaymentURL("  ", 1, 10, 1); err == nil {
		t.Fatal("want error for empty base")
	}
}
