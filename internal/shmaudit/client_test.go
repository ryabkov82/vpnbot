package shmaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(baseURL string) *Config {
	cfg := &Config{}
	cfg.API.BaseURL = baseURL
	cfg.API.Login = "audit-login"
	cfg.API.Pass = "audit-pass"
	cfg.API.Timeout = 5
	return cfg
}

func TestReadOnlyTransport_AllowsAuthAndWhitelistGET(t *testing.T) {
	var posts, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == pathAuth:
			posts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-1"})
		case r.Method == http.MethodGet && r.URL.Path == pathAdminUser:
			gets.Add(1)
			_ = json.NewEncoder(w).Encode(Page[AuditUser]{Data: []AuditUser{}, Items: 0})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := NewClient(testConfig(srv.URL), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FetchUsers(context.Background(), 10, nil); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("posts=%d gets=%d", posts.Load(), gets.Load())
	}
	// session cookie present
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookies := c.HTTPClient().Jar.Cookies(u)
	found := false
	for _, ck := range cookies {
		if ck.Name == "session_id" && ck.Value == "sess-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("session cookie missing: %+v", cookies)
	}
}

func TestReadOnlyTransport_BlocksWritesAndUnknownGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not receive blocked request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c, err := NewClient(testConfig(srv.URL), 0)
	if err != nil {
		t.Fatal(err)
	}
	client := c.HTTPClient()
	blocked := []struct {
		method string
		path   string
	}{
		{http.MethodPut, pathAdminUser},
		{http.MethodPost, pathAdminUser},
		{http.MethodPatch, pathAdminUser},
		{http.MethodDelete, pathAdminUser},
		{http.MethodGet, "/shm/v1/admin/user/password"},
		{http.MethodGet, "/shm/v1/admin/other"},
	}
	for _, tc := range blocked {
		req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err == nil {
			if resp != nil {
				resp.Body.Close()
			}
			t.Fatalf("expected block for %s %s", tc.method, tc.path)
		}
		msg := err.Error()
		if !strings.Contains(msg, "read-only SHM transport blocked") {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg, tc.method) || !strings.Contains(msg, tc.path) {
			t.Fatalf("error must contain method and path: %v", err)
		}
		if strings.Contains(msg, "audit-pass") || strings.Contains(msg, "Cookie") || strings.Contains(msg, "session") {
			t.Fatalf("error leaked secrets: %v", err)
		}
	}
}

func TestAuditRun_OnlyAuthPOSTThenGETs(t *testing.T) {
	var postCount, getCount atomic.Int32
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == pathAuth:
			postCount.Add(1)
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "audit-login") {
				t.Errorf("auth body missing login")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "s"})
		case r.Method == http.MethodGet:
			getCount.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []any{}, "items": 0, "limit": 10, "offset": 0, "status": 200,
			})
		default:
			http.Error(w, "no", 405)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := testConfig(srv.URL)
	opt := Options{
		ConfigPath:  "unused",
		OutputDir:   dir,
		FCCategory:  testFC,
		VFFCategory: testVFF,
		PageSize:    50,
	}
	if _, err := Run(context.Background(), cfg, opt, nil); err != nil {
		t.Fatal(err)
	}
	if postCount.Load() != 1 {
		t.Fatalf("POST count=%d methods=%v", postCount.Load(), methods)
	}
	if getCount.Load() < 5 {
		t.Fatalf("expected >=5 GETs, got %d", getCount.Load())
	}
	for _, m := range methods {
		if strings.HasPrefix(m, "PUT ") || strings.HasPrefix(m, "PATCH ") || strings.HasPrefix(m, "DELETE ") {
			t.Fatalf("write method observed: %v", methods)
		}
		if strings.HasPrefix(m, "POST ") && m != "POST "+pathAuth {
			t.Fatalf("unexpected POST: %v", methods)
		}
	}
}

func TestFetchAll_PaginationCases(t *testing.T) {
	t.Run("single page", func(t *testing.T) {
		srv := pageServer(t, map[int][]AuditUser{
			0: {{UserID: 1}, {UserID: 2}},
		}, 2)
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 0)
		got, err := c.FetchUsers(context.Background(), 10, nil)
		if err != nil || len(got) != 2 {
			t.Fatalf("got=%d err=%v", len(got), err)
		}
	})

	t.Run("multiple pages", func(t *testing.T) {
		srv := pageServer(t, map[int][]AuditUser{
			0: {{UserID: 1}, {UserID: 2}},
			2: {{UserID: 3}},
		}, 3)
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 0)
		got, err := c.FetchUsers(context.Background(), 2, nil)
		if err != nil || len(got) != 3 {
			t.Fatalf("got=%d err=%v", len(got), err)
		}
	})

	t.Run("items missing", func(t *testing.T) {
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != pathAdminUser {
				http.NotFound(w, r)
				return
			}
			if n == 0 {
				n++
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []AuditUser{{UserID: 1}}, "limit": 1, "offset": 0,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []AuditUser{}, "limit": 1, "offset": 1,
			})
		}))
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 0)
		got, err := c.FetchUsers(context.Background(), 1, nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("got=%d err=%v", len(got), err)
		}
	})

	t.Run("items larger than actual", func(t *testing.T) {
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if n == 0 {
				n++
				_ = json.NewEncoder(w).Encode(Page[AuditUser]{
					Data: []AuditUser{{UserID: 1}}, Items: 100, Limit: 1, Offset: 0,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(Page[AuditUser]{
				Data: []AuditUser{}, Items: 100, Limit: 1, Offset: 1,
			})
		}))
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 0)
		got, err := c.FetchUsers(context.Background(), 1, nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("got=%d err=%v", len(got), err)
		}
	})

	t.Run("http 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("err"))
		}))
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 0)
		_, err := c.FetchUsers(context.Background(), 10, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}))
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 0)
		_, err := c.FetchUsers(context.Background(), 10, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("repeated page", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(Page[AuditUser]{
				Data: []AuditUser{{UserID: 1}}, Items: 0, Limit: 1, Offset: 0,
			})
		}))
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 0)
		_, err := c.FetchUsers(context.Background(), 1, nil)
		if err == nil || !strings.Contains(err.Error(), "repeated page") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("page size validation", func(t *testing.T) {
		c, _ := NewClient(testConfig("http://127.0.0.1:1"), 0)
		_, err := c.FetchUsers(context.Background(), 0, nil)
		if err == nil {
			t.Fatal("expected page-size error")
		}
		_, err = c.FetchUsers(context.Background(), 1001, nil)
		if err == nil {
			t.Fatal("expected page-size error")
		}
	})

	t.Run("context cancel during delay", func(t *testing.T) {
		var n atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			_ = json.NewEncoder(w).Encode(Page[AuditUser]{
				Data: []AuditUser{{UserID: int(n.Load())}}, Items: 0, Limit: 1, Offset: FlexibleInt(n.Load() - 1),
			})
		}))
		defer srv.Close()
		c, _ := NewClient(testConfig(srv.URL), 2*time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		_, err := c.FetchUsers(ctx, 1, nil)
		if err == nil {
			t.Fatal("expected cancel error")
		}
	})
}

func pageServer(t *testing.T, pages map[int][]AuditUser, totalItems int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathAdminUser {
			http.NotFound(w, r)
			return
		}
		offset := 0
		fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
		data := pages[offset]
		if data == nil {
			data = []AuditUser{}
		}
		_ = json.NewEncoder(w).Encode(Page[AuditUser]{
			Data: data, Items: FlexibleInt(totalItems), Limit: FlexibleInt(len(data)), Offset: FlexibleInt(offset),
		})
	}))
}

func TestPage_DecodeNumberAndStringMetadata(t *testing.T) {
	t.Run("numbers", func(t *testing.T) {
		var page Page[AuditUser]
		raw := []byte(`{"data":[],"items":10,"limit":250,"offset":0}`)
		if err := json.Unmarshal(raw, &page); err != nil {
			t.Fatal(err)
		}
		if int(page.Items) != 10 || int(page.Limit) != 250 || int(page.Offset) != 0 {
			t.Fatalf("items=%d limit=%d offset=%d", page.Items, page.Limit, page.Offset)
		}
	})
	t.Run("strings", func(t *testing.T) {
		var page Page[AuditUser]
		raw := []byte(`{"data":[],"items":"10","limit":"250","offset":"0"}`)
		if err := json.Unmarshal(raw, &page); err != nil {
			t.Fatal(err)
		}
		if int(page.Items) != 10 || int(page.Limit) != 250 || int(page.Offset) != 0 {
			t.Fatalf("items=%d limit=%d offset=%d", page.Items, page.Limit, page.Offset)
		}
	})
}

func TestCredentialsNotInAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	}))
	defer srv.Close()
	c, _ := NewClient(testConfig(srv.URL), 0)
	err := c.Authenticate(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "audit-pass") || strings.Contains(msg, "audit-login") {
		t.Fatalf("credentials in error: %v", err)
	}
}
