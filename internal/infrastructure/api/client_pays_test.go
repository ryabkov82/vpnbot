package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetUserPays_OKAndEmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user/pay" {
			http.NotFound(w, r)
			return
		}
		gotFilter := r.URL.Query().Get("filter")
		if !strings.Contains(gotFilter, `"user_id":42`) && !strings.Contains(gotFilter, `"user_id": 42`) {
			t.Errorf("unexpected filter query: %q", gotFilter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"id":1,"user_id":42,"date":"d","money":-1318.74,"pay_system_id":"x","uniq_key":"y","comment":{"k":true}}]}`)
	}))
	t.Cleanup(srv.Close)

	c := APIClient{
		ServerURL:  srv.URL,
		HTTPClient: srv.Client(),
	}

	list, err := c.GetUserPays(42)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Money != -1318.74 {
		t.Fatalf("got %+v", list)
	}
}

func TestGetUserPays_EmptyDataArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)
	c := APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	list, err := c.GetUserPays(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty, got %#v", list)
	}
}

func TestGetUserPays_NonJSONErrorDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{not json}`)
	}))
	t.Cleanup(srv.Close)
	c := APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.GetUserPays(1)
	if err == nil || !strings.Contains(err.Error(), "decode user pays") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestGetUserPays_StatusNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.GetUserPays(7)
	if err == nil || !strings.Contains(err.Error(), "get user pays: API status 500") {
		t.Fatalf("got %v", err)
	}
}
