package remnawave

import (
	"context"
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

func TestGetUserByUsername274(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/by-username/us_42" {
			t.Fatalf("path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"response":{"id":42,"uuid":"11111111-1111-1111-1111-111111111111","username":"us_42"}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	user, err := c.GetUserByUsername(context.Background(), "us_42")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != 42 || user.UUID != "11111111-1111-1111-1111-111111111111" || user.Username != "us_42" {
		t.Fatalf("user=%+v", user)
	}
}

func TestGetUserByUsername323(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"id":42,"username":"us_42"}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	user, err := c.GetUserByUsername(context.Background(), "us_42")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != 42 || user.UUID != "" || user.Username != "us_42" {
		t.Fatalf("user=%+v", user)
	}
}

func TestGetUserByUsernameInvalid(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing id", `{"response":{"username":"us_42"}}`},
		{"id zero", `{"response":{"id":0,"username":"us_42"}}`},
		{"id negative", `{"response":{"id":-1,"username":"us_42"}}`},
		{"id fractional", `{"response":{"id":1.5,"username":"us_42"}}`},
		{"empty username", `{"response":{"id":42,"username":"  "}}`},
		{"username mismatch", `{"response":{"id":42,"username":"other"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := NewClient(srv.URL, "tok")
			if _, err := c.GetUserByUsername(context.Background(), "us_42"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func assertBandwidthQuery(t *testing.T, gotPath, pathPrefix string) {
	t.Helper()
	if !strings.HasPrefix(gotPath, pathPrefix) {
		t.Fatalf("full URL path+query: %q want prefix %q", gotPath, pathPrefix)
	}
	qv, err := url.ParseQuery(strings.TrimPrefix(gotPath, pathPrefix))
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

func TestGetUserBandwidthStatsUsesUUIDWhenPresent(t *testing.T) {
	const wantPath = "/api/bandwidth-stats/users/11111111-1111-1111-1111-111111111111"
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Fatalf("path %q want %s", r.URL.Path, wantPath)
		}
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"series":[{"total":42}]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	start := time.Date(2026, 4, 19, 19, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	stats, err := c.GetUserBandwidthStats(context.Background(), User{
		ID:   42,
		UUID: "11111111-1111-1111-1111-111111111111",
	}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if stats.UsedBytes != 42 {
		t.Fatalf("UsedBytes=%d", stats.UsedBytes)
	}
	assertBandwidthQuery(t, gotPath, wantPath+"?")
}

func TestGetUserBandwidthStatsUsesNumericID(t *testing.T) {
	const wantPath = "/api/bandwidth-stats/users/42"
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Fatalf("path %q want %s", r.URL.Path, wantPath)
		}
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"series":[{"total":42}]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	start := time.Date(2026, 4, 19, 19, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	stats, err := c.GetUserBandwidthStats(context.Background(), User{ID: 42}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if stats.UsedBytes != 42 {
		t.Fatalf("UsedBytes=%d", stats.UsedBytes)
	}
	assertBandwidthQuery(t, gotPath, wantPath+"?")
}

func TestGetUserBandwidthStatsMissingIdentity(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	start := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	if _, err := c.GetUserBandwidthStats(context.Background(), User{}, start, end); err == nil {
		t.Fatal("expected error for missing identity")
	}
	if called {
		t.Fatal("HTTP request must not be sent without identity")
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
