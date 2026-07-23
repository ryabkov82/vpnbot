package payments

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildYooKassaPaymentURL_VFF(t *testing.T) {
	const base = "https://example.com/"
	got, err := BuildYooKassaPaymentURL(base, 42, 199.0, 1700000000, "yookassa_vff")
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
	if q.Get("action") != "create" || q.Get("user_id") != "42" || q.Get("ts") != "1700000000" || q.Get("ps") != "yookassa_vff" || q.Get("amount") != "199" {
		t.Fatalf("query: %#v", q)
	}
	if strings.Contains(got, "ps=yookassa_fc") || q.Get("ps") == "yookassa" {
		t.Fatalf("must use brand pay system, got %s", got)
	}
	if !strings.HasPrefix(got, "https://example.com/shm/pay_systems/yookassa.cgi?") {
		t.Fatalf("prefix: %s", got)
	}
}

func TestBuildYooKassaPaymentURL_FC(t *testing.T) {
	got, err := BuildYooKassaPaymentURL("https://bill.example", 7, 50, 1, "yookassa_fc")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("ps") != "yookassa_fc" {
		t.Fatalf("ps=%q", u.Query().Get("ps"))
	}
	if strings.Contains(got, "ps=yookassa_vff") || u.Query().Get("ps") == "yookassa" {
		t.Fatalf("must use FC pay system, got %s", got)
	}
}

func TestBuildYooKassaPaymentURL_TrimsBaseSlash(t *testing.T) {
	got, err := BuildYooKassaPaymentURL("https://x.y/ ", 1, 10.5, 1, "yookassa_vff")
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
	got, err := BuildYooKassaPaymentURL("https://h", 9, 100.25, 0, "yookassa_vff")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("amount") != "100.25" {
		t.Fatalf("amount: %q", u.Query().Get("amount"))
	}
}

func TestBuildYooKassaPaymentURL_EncodesPaySystem(t *testing.T) {
	ps := "yookassa_vff+extra"
	got, err := BuildYooKassaPaymentURL("https://h", 1, 10, 1, ps)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("ps") != ps {
		t.Fatalf("decoded ps=%q", u.Query().Get("ps"))
	}
	wantEnc := url.QueryEscape(ps)
	if !strings.Contains(u.RawQuery, wantEnc) {
		t.Fatalf("raw query must encode ps as %q; got %q", wantEnc, u.RawQuery)
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
	if _, err := BuildYooKassaPaymentURL("https://x", 0, 10, 1, "yookassa_vff"); err == nil {
		t.Fatal("want error for user id 0")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 0, 1, "yookassa_vff"); err == nil {
		t.Fatal("want error for amount 0")
	}
	if _, err := BuildYooKassaPaymentURL("  ", 1, 10, 1, "yookassa_vff"); err == nil {
		t.Fatal("want error for empty base")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 10, 1, ""); err == nil {
		t.Fatal("want error for empty pay system")
	}
	if _, err := BuildYooKassaPaymentURL("https://x", 1, 10, 1, "   "); err == nil {
		t.Fatal("want error for whitespace pay system")
	}
}
