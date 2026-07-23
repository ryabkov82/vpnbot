package payments

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildYooKassaPaymentURL_VFF(t *testing.T) {
	const base = "https://example.com/"
	got, err := BuildYooKassaPaymentURL(base, 42, 199.0, 1700000000, "yookassa", "vff")
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
	if q.Get("action") != "create" || q.Get("user_id") != "42" || q.Get("ts") != "1700000000" ||
		q.Get("ps") != "yookassa" || q.Get("amount") != "199" || q.Get("brand_id") != "vff" {
		t.Fatalf("query: %#v", q)
	}
	if q.Get("brand_id") == "fc" {
		t.Fatalf("VFF must not use fc brand_id: %s", got)
	}
}

func TestBuildYooKassaPaymentURL_FC(t *testing.T) {
	got, err := BuildYooKassaPaymentURL("https://bill.example", 7, 50, 1, "yookassa", "fc")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("ps") != "yookassa" || q.Get("brand_id") != "fc" {
		t.Fatalf("query: %#v", q)
	}
	if q.Get("brand_id") == "vff" {
		t.Fatalf("FC must not use vff brand_id: %s", got)
	}
}

func TestBuildYooKassaPaymentURL_EncodesBrandID(t *testing.T) {
	// Valid brand ids are restricted; underscore still goes through url.Values encoding.
	got, err := BuildYooKassaPaymentURL("https://h", 1, 10, 1, "yookassa", "brand_v2")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("brand_id") != "brand_v2" {
		t.Fatalf("decoded brand_id=%q", u.Query().Get("brand_id"))
	}
	wantEnc := url.QueryEscape("brand_v2")
	if !strings.Contains(u.RawQuery, "brand_id="+wantEnc) {
		t.Fatalf("raw query must encode brand_id; got %q", u.RawQuery)
	}
}

func TestBuildYooKassaPaymentURL_TrimsBaseSlash(t *testing.T) {
	got, err := BuildYooKassaPaymentURL("https://x.y/ ", 1, 10.5, 1, "yookassa", "vff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://x.y/shm/pay_systems/yookassa.cgi") {
		t.Fatalf("got %s", got)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("amount") != "10.5" || u.Query().Get("brand_id") != "vff" {
		t.Fatalf("query: %#v", u.Query())
	}
}

func TestBuildYooKassaPaymentURL_FractionalAmount(t *testing.T) {
	got, err := BuildYooKassaPaymentURL("https://h", 9, 100.25, 0, "yookassa", "vff")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("amount") != "100.25" {
		t.Fatalf("amount: %q", u.Query().Get("amount"))
	}
}

func TestBuildCryptoCloudPaymentURL_NoBrandID(t *testing.T) {
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
	if _, ok := q["brand_id"]; ok || strings.Contains(got, "brand_id=") {
		t.Fatalf("CryptoCloud must not include brand_id: %s", got)
	}
}

func TestBuildYooKassaPaymentURL_Validation(t *testing.T) {
	if _, err := BuildYooKassaPaymentURL("https://x", 0, 10, 1, "yookassa", "vff"); err == nil {
		t.Fatal("want error for user id 0")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 0, 1, "yookassa", "vff"); err == nil {
		t.Fatal("want error for amount 0")
	}
	if _, err := BuildYooKassaPaymentURL("  ", 1, 10, 1, "yookassa", "vff"); err == nil {
		t.Fatal("want error for empty base")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 10, 1, "", "vff"); err == nil {
		t.Fatal("want error for empty pay system")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 10, 1, "yookassa", ""); err == nil {
		t.Fatal("want error for empty brand id")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 10, 1, "yookassa", "   "); err == nil {
		t.Fatal("want error for whitespace brand id")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 10, 1, "yookassa", "Bad ID"); err == nil {
		t.Fatal("want error for invalid brand id")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 10, 1, "yookassa", "vff/x"); err == nil {
		t.Fatal("want error for invalid brand id chars")
	}
}
