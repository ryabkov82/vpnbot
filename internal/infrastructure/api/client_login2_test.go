package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func mustUnescapeFilter(t *testing.T, rawFilter string) map[string]interface{} {
	t.Helper()
	s, err := url.QueryUnescape(rawFilter)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if json.Unmarshal([]byte(s), &m) != nil {
		t.Fatalf("bad filter JSON: %q", s)
	}
	return m
}

func TestAPIClient_GetUserByLogin2_OK(t *testing.T) {
	em := `v@xz.io`
	wantLogin := webuser.WebLoginFromEmail(em)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		m := mustUnescapeFilter(t, r.URL.Query().Get("filter"))
		if m["login2"] != wantLogin {
			t.Fatalf("want login2 filter %q got %#v", wantLogin, m)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		payload := models.User{
			ID:       55,
			Login:    "@999",
			Login2:   wantLogin,
			Settings: models.UserSettings{},
		}
		b, _ := json.Marshal(struct {
			Data []models.User `json:"data"`
		}{Data: []models.User{payload}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	c := APIClient{
		ServerURL:  srv.URL,
		HTTPClient: srv.Client(),
	}
	got, err := c.GetUserByLogin2(wantLogin)
	if err != nil || got == nil || got.ID != 55 || got.Login2 != wantLogin {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestAPIClient_GetUserByLogin2_EmptySkipped(t *testing.T) {
	c := APIClient{ServerURL: "http://example.invalid"}
	got, err := c.GetUserByLogin2("  ")
	if err != nil || got != nil {
		t.Fatalf("%#v err=%v", got, err)
	}
}

func TestAPIClient_PostAdminUserUpdateIncludesLogin2WhenSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			m := mustUnescapeFilter(t, r.URL.Query().Get("filter"))
			if m["login2"] != "web_foo" {
				t.Fatalf("unexpected filter %#v", m)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			b, _ := json.Marshal(map[string][]models.User{"data": {{ID: 7, Login: "@1", Login2: "web_foo"}}})
			_, _ = w.Write(b)
		case http.MethodPost:
			raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			var body map[string]interface{}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			if body["user_id"] != float64(7) || body["login2"] != "web_foo" {
				t.Fatalf("body %#v", body)
			}
			if _, ok := body["settings"].(map[string]interface{}); !ok {
				t.Fatalf("settings missing %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	c := APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	got, err := c.PostAdminUserUpdateSettings(7, "web_foo", map[string]interface{}{
		"web": map[string]string{"email": "a@b.c"},
	})
	if err != nil || got == nil || got.ID != 7 || got.Login2 != "web_foo" {
		t.Fatalf("%#v err=%v", got, err)
	}
}

func TestAPIClient_PostAdminUserUpdateOmitsLogin2WhenBlank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		if strings.Contains(string(raw), "login2") {
			t.Fatalf("login2 must be omitted got %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"user_id":3,"login":"web_x"}]}`))
	}))
	t.Cleanup(srv.Close)
	c := APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	if _, err := c.PostAdminUserUpdateSettings(3, "   ", map[string]interface{}{
		"web": map[string]string{"email": "a@b.c"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIClient_PostAdminUserUpdateSettings_Login2VerifyFails(t *testing.T) {
	var posts, puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			m := mustUnescapeFilter(t, r.URL.Query().Get("filter"))
			if m["login2"] != "web_hash" {
				t.Fatalf("filter %#v", m)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
		case http.MethodPost:
			posts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			puts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)

	c := APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.PostAdminUserUpdateSettings(9, "web_hash", map[string]interface{}{
		"web": map[string]string{"email": "you@example.com"},
	})
	if err == nil || !errors.Is(err, ErrLogin2NotPersistedSHM) {
		t.Fatalf("want ErrLogin2NotPersistedSHM got %v", err)
	}
	if posts != 1 || puts != 1 {
		t.Fatalf("post=%d put=%d", posts, puts)
	}
}

func TestAPIClient_PostAdminUserUpdateSettings_Login2RecoveredAfterPUT(t *testing.T) {
	var posts, puts, verifyGets int
	var verifyOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			m := mustUnescapeFilter(t, r.URL.Query().Get("filter"))
			if m["login2"] != "web_recover" {
				t.Fatalf("filter %#v", m)
			}
			verifyGets++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if verifyOK {
				b, _ := json.Marshal(map[string][]models.User{"data": {{
					ID:     11,
					Login:  "@u",
					Login2: "web_recover",
				}}})
				_, _ = w.Write(b)
				return
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
		case http.MethodPost:
			posts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			puts++
			verifyOK = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)

	c := APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	got, err := c.PostAdminUserUpdateSettings(11, "web_recover", map[string]interface{}{
		"web": map[string]string{"email": "r@x.io"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 11 || got.Login2 != "web_recover" {
		t.Fatalf("got %#v", got)
	}
	if posts != 1 || puts != 1 || verifyGets != 2 {
		t.Fatalf("post=%d put=%d gets=%d", posts, puts, verifyGets)
	}
}
