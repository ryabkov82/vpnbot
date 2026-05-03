package remnawave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseBandwidthUsed(t *testing.T) {
	cases := []struct {
		raw string
		exp int64
	}{
		{`{"response":{"series":[{"total":1000},{"total":2000}]}}`, 3000},
		{`{"response":{"series":[],"topNodes":[{"total":1000},{"total":2000}]}}`, 3000},
		{`{"response":{"topNodes":[{"total":500},{"total":500}]}}`, 1000},
	}
	for _, tc := range cases {
		n, err := parseBandwidthUsed([]byte(tc.raw))
		if err != nil || n != tc.exp {
			t.Fatalf("%s: got %d err=%v want %d", tc.raw, n, err, tc.exp)
		}
	}
	if _, err := parseBandwidthUsed([]byte(`{"response":{"series":[{"uuid":"x"}],"topNodes":[]}}`)); err == nil {
		t.Fatal("expected error for series without total")
	}
	if _, err := parseBandwidthUsed([]byte(`{"response":{"series":[],"topNodes":[]}}`)); err == nil {
		t.Fatal("expected error for empty series and topNodes")
	}
	if _, err := parseBandwidthUsed([]byte(`{"response":{}}`)); err == nil {
		t.Fatal("expected error for empty response object")
	}
	if _, err := parseBandwidthUsed([]byte(`{}`)); err == nil {
		t.Fatal("expected error for missing response")
	}
}

func TestParseBandwidthUsedFloatTotal(t *testing.T) {
	n, err := parseBandwidthUsed([]byte(`{"response":{"series":[{"total":1000.7}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1000 {
		t.Fatalf("got %d want 1000 (truncated from float)", n)
	}
}

func TestGetUserBandwidthStatsQueryParams(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bandwidth-stats/users/my-uuid" {
			t.Fatalf("path %q want /api/bandwidth-stats/users/my-uuid", r.URL.Path)
		}
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":{"series":[{"total":42}]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	if c == nil {
		t.Fatal("nil client")
	}
	start := time.Date(2026, 4, 19, 19, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	stats, err := c.GetUserBandwidthStats(context.Background(), "my-uuid", start, end)
	if err != nil {
		t.Fatal(err)
	}
	if stats.UsedBytes != 42 {
		t.Fatalf("UsedBytes=%d", stats.UsedBytes)
	}
	const prefix = "/api/bandwidth-stats/users/my-uuid?"
	if !strings.HasPrefix(gotPath, prefix) {
		t.Fatalf("full URL path+query: %q", gotPath)
	}
	qv, err := url.ParseQuery(strings.TrimPrefix(gotPath, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if qv.Get("topNodesLimit") != "10" {
		t.Fatalf("topNodesLimit=%q want 10", qv.Get("topNodesLimit"))
	}
	if qv.Get("start") != "2026-04-19" {
		t.Fatalf("start=%q want 2026-04-19", qv.Get("start"))
	}
	if qv.Get("end") != "2026-05-03" {
		t.Fatalf("end=%q want 2026-05-03", qv.Get("end"))
	}
}

func TestGetSubscriptionByUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/subscriptions/by-username/us_42" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":{"subscriptionUrl":"https://example.com/sub"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	sub, err := c.GetSubscriptionByUsername(context.Background(), "us_42")
	if err != nil {
		t.Fatal(err)
	}
	if sub.SubscriptionURL != "https://example.com/sub" {
		t.Fatalf("url=%q", sub.SubscriptionURL)
	}
}

func TestGetSubscriptionByUsernameEmptyURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"subscriptionUrl":"  "}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	if _, err := c.GetSubscriptionByUsername(context.Background(), "x"); err == nil {
		t.Fatal("expected error for empty subscriptionUrl")
	}
}

func TestEncryptHappLink(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/system/tools/happ/encrypt" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("auth header %q", r.Header.Get("Authorization"))
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("content-type %q", ct)
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"encryptedLink":"happ://crypt4/abc"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	enc, err := c.EncryptHappLink(context.Background(), "https://sub.example/x")
	if err != nil {
		t.Fatal(err)
	}
	if enc != "happ://crypt4/abc" {
		t.Fatalf("enc=%q", enc)
	}
	if !strings.Contains(gotBody, `"linkToEncrypt":"https://sub.example/x"`) {
		t.Fatalf("body=%q", gotBody)
	}
}

func TestEncryptHappLinkBadPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"encryptedLink":"https://evil"}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	if _, err := c.EncryptHappLink(context.Background(), "https://x"); err == nil {
		t.Fatal("expected error for non-happ prefix")
	}
}
