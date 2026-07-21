package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestTelegramSHMLogin(t *testing.T) {
	tests := []struct {
		brandID string
		chatID  int64
		want    string
	}{
		{"vff", 123, "@123"},
		{"fc", 123, "@fc_123"},
		{"other-brand", 123, "@other-brand_123"},
	}
	for _, tc := range tests {
		got := telegramSHMLogin(tc.brandID, tc.chatID)
		if got != tc.want {
			t.Fatalf("telegramSHMLogin(%q, %d)=%q, want %q", tc.brandID, tc.chatID, got, tc.want)
		}
	}
}

func brandCfg(id string) config.BrandConfig {
	return config.BrandConfig{
		ID:                 id,
		Name:               "Test " + id,
		AllowedHosts:       []string{id + ".example.com"},
		PublicBaseURL:      "https://" + id + ".example.com",
		LandingURL:         "https://landing." + id + ".example.com",
		ServiceCategory:    "vpn-" + id,
		WebUserLoginPrefix: "web_",
		WebUserSource:      id + ".example.com",
		PaymentProfile:     id + "_pay",
	}
}

func decodeAdminUserFilter(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	s, err := url.QueryUnescape(raw)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("filter %q: %v", s, err)
	}
	return m
}

func writeUsersJSON(w http.ResponseWriter, users ...models.User) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Data []models.User `json:"data"`
	}{Data: users})
}

func TestGetUser_InvalidChatID(t *testing.T) {
	s := NewService(nil, brandCfg("vff"))
	for _, chatID := range []int64{0, -1} {
		u, err := s.GetUser(chatID)
		if u != nil || err == nil || err.Error() != "invalid telegram chat id" {
			t.Fatalf("chatID=%d: got u=%v err=%v", chatID, u, err)
		}
	}
}

func TestGetUser_EmptyBrandID(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	u, err := s.GetUser(123)
	if u != nil || err == nil || err.Error() != "active brand id is required" {
		t.Fatalf("got u=%v err=%v", u, err)
	}
}

func TestRegisterUser_InvalidChatID(t *testing.T) {
	s := NewService(nil, brandCfg("vff"))
	err := s.RegisterUser(models.UserRegistrationRequest{})
	if err == nil || err.Error() != "telegram chat_id must be positive" {
		t.Fatalf("got %v", err)
	}
}

func TestRegisterUser_EmptyBrandID(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	err := s.RegisterUser(models.UserRegistrationRequest{
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 123}},
	})
	if err == nil || err.Error() != "active brand id is required" {
		t.Fatalf("got %v", err)
	}
}

func TestRegisterUser_ForcesCanonicalLoginAndBrand_FC(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var got models.UserRegistrationRequest
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Login != "@fc_123" {
			t.Fatalf("login=%q", got.Login)
		}
		if got.Settings.BrandID != "fc" {
			t.Fatalf("brand_id=%q", got.Settings.BrandID)
		}
		if got.Settings.Telegram.ChatID != 123 {
			t.Fatalf("chat_id=%d", got.Settings.Telegram.ChatID)
		}
		if got.Password != "secret-pass" {
			t.Fatalf("password=%q", got.Password)
		}
		if got.FullName != "Ada Lovelace" {
			t.Fatalf("full_name=%q", got.FullName)
		}
		if got.Settings.Telegram.Username != "ada" || got.Settings.Telegram.FirstName != "Ada" {
			t.Fatalf("telegram profile %#v", got.Settings.Telegram)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cli := &api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	s := NewService(cli, brandCfg("fc"))
	err := s.RegisterUser(models.UserRegistrationRequest{
		Login:    "@wrong",
		Password: "secret-pass",
		FullName: "Ada Lovelace",
		Settings: models.UserSettings{
			BrandID: "vff",
			Telegram: models.TelegramInfo{
				ChatID:    123,
				Username:  "ada",
				FirstName: "Ada",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("API calls=%d, want 1", calls.Load())
	}
}

func TestRegisterUser_ForcesCanonicalLoginAndBrand_VFF(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]interface{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got["login"] != "@123" {
			t.Fatalf("login=%v", got["login"])
		}
		settings, _ := got["settings"].(map[string]interface{})
		if settings["brand_id"] != "vff" {
			t.Fatalf("brand_id=%v", settings["brand_id"])
		}
		tg, _ := settings["telegram"].(map[string]interface{})
		if tg["chat_id"] != float64(123) {
			t.Fatalf("chat_id=%v", tg["chat_id"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cli := &api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	s := NewService(cli, brandCfg("vff"))
	err := s.RegisterUser(models.UserRegistrationRequest{
		Login:    "@fc_123",
		Password: "p",
		FullName: "User",
		Settings: models.UserSettings{
			BrandID:  "fc",
			Telegram: models.TelegramInfo{ChatID: 123},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("API calls=%d, want 1", calls.Load())
	}
}

func TestGetUser_VFFLegacy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := decodeAdminUserFilter(t, r.URL.Query().Get("filter"))
		if m["login"] != "@123" {
			t.Fatalf("filter %#v", m)
		}
		writeUsersJSON(w, models.User{
			ID:    1,
			Login: "@123",
			Settings: models.UserSettings{
				Telegram: models.TelegramInfo{ChatID: 123},
			},
		})
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	u, err := s.GetUser(123)
	if err != nil || u == nil || u.ID != 1 {
		t.Fatalf("got %#v err=%v", u, err)
	}
}

func TestGetUser_VFFExplicit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeUsersJSON(w, models.User{
			ID:    2,
			Login: "@123",
			Settings: models.UserSettings{
				BrandID:  "vff",
				Telegram: models.TelegramInfo{ChatID: 123},
			},
		})
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	u, err := s.GetUser(123)
	if err != nil || u == nil || u.Settings.BrandID != "vff" {
		t.Fatalf("got %#v err=%v", u, err)
	}
}

func TestGetUser_VFFWrongBrand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeUsersJSON(w, models.User{
			ID:    3,
			Login: "@123",
			Settings: models.UserSettings{
				BrandID:  "fc",
				Telegram: models.TelegramInfo{ChatID: 123},
			},
		})
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	u, err := s.GetUser(123)
	if u != nil || !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %#v err=%v", u, err)
	}
}

func TestGetUser_FCExplicit(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := decodeAdminUserFilter(t, r.URL.Query().Get("filter"))
		login, _ := m["login"].(string)
		seen = append(seen, login)
		writeUsersJSON(w, models.User{
			ID:    4,
			Login: "@fc_123",
			Settings: models.UserSettings{
				BrandID:  "fc",
				Telegram: models.TelegramInfo{ChatID: 123},
			},
		})
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("fc"))
	u, err := s.GetUser(123)
	if err != nil || u == nil || u.ID != 4 {
		t.Fatalf("got %#v err=%v", u, err)
	}
	if len(seen) != 1 || seen[0] != "@fc_123" {
		t.Fatalf("lookups=%v, want only @fc_123", seen)
	}
}

func TestGetUser_FCMissingBrand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeUsersJSON(w, models.User{
			ID:    5,
			Login: "@fc_123",
			Settings: models.UserSettings{
				Telegram: models.TelegramInfo{ChatID: 123},
			},
		})
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("fc"))
	u, err := s.GetUser(123)
	if u != nil || !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %#v err=%v", u, err)
	}
}

func TestGetUser_FCWrongTelegramID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeUsersJSON(w, models.User{
			ID:    6,
			Login: "@fc_123",
			Settings: models.UserSettings{
				BrandID:  "fc",
				Telegram: models.TelegramInfo{ChatID: 999},
			},
		})
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("fc"))
	u, err := s.GetUser(123)
	if u != nil || !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %#v err=%v", u, err)
	}
}

func TestGetUser_FCNoFallbackToVFFLogin(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := decodeAdminUserFilter(t, r.URL.Query().Get("filter"))
		login, _ := m["login"].(string)
		seen = append(seen, login)
		// Даже если бы искали @123 — ответ «не найден».
		writeUsersJSON(w)
	}))
	t.Cleanup(srv.Close)

	s := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("fc"))
	u, err := s.GetUser(123)
	if err != nil || u != nil {
		t.Fatalf("got %#v err=%v", u, err)
	}
	for _, login := range seen {
		if login == "@123" {
			t.Fatalf("FC must not fall back to @123; seen=%v", seen)
		}
	}
	if len(seen) != 1 || seen[0] != "@fc_123" {
		t.Fatalf("lookups=%v, want only @fc_123", seen)
	}
}

func TestRegisterAndLookup_SameTelegramIDTwoBrands(t *testing.T) {
	type regBody struct {
		Login    string
		BrandID  string
		ChatID   int64
		Password string
	}
	regs := map[string]regBody{}
	users := map[string]models.User{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/shm/v1/admin/user":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var req models.UserRegistrationRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatal(err)
			}
			regs[req.Login] = regBody{
				Login:    req.Login,
				BrandID:  req.Settings.BrandID,
				ChatID:   req.Settings.Telegram.ChatID,
				Password: req.Password,
			}
			users[req.Login] = models.User{
				ID:    len(users) + 1,
				Login: req.Login,
				Settings: models.UserSettings{
					BrandID:  req.Settings.BrandID,
					Telegram: req.Settings.Telegram,
				},
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/shm/v1/admin/user":
			m := decodeAdminUserFilter(t, r.URL.Query().Get("filter"))
			login, _ := m["login"].(string)
			if u, ok := users[login]; ok {
				writeUsersJSON(w, u)
				return
			}
			writeUsersJSON(w)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cli := &api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	vff := NewService(cli, brandCfg("vff"))
	fc := NewService(cli, brandCfg("fc"))

	const chatID int64 = 123
	if err := vff.RegisterUser(models.UserRegistrationRequest{
		Password: "vff-pass",
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := fc.RegisterUser(models.UserRegistrationRequest{
		Password: "fc-pass",
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
	}); err != nil {
		t.Fatal(err)
	}

	if regs["@123"].BrandID != "vff" || regs["@123"].ChatID != chatID {
		t.Fatalf("vff reg %#v", regs["@123"])
	}
	if regs["@fc_123"].BrandID != "fc" || regs["@fc_123"].ChatID != chatID {
		t.Fatalf("fc reg %#v", regs["@fc_123"])
	}

	uv, err := vff.GetUser(chatID)
	if err != nil || uv == nil || uv.Login != "@123" || uv.Settings.BrandID != "vff" {
		t.Fatalf("vff lookup %#v err=%v", uv, err)
	}
	uf, err := fc.GetUser(chatID)
	if err != nil || uf == nil || uf.Login != "@fc_123" || uf.Settings.BrandID != "fc" {
		t.Fatalf("fc lookup %#v err=%v", uf, err)
	}
	if uv.ID == uf.ID {
		t.Fatalf("expected distinct SHM users, both id=%d", uv.ID)
	}
}

func TestUserSettings_BrandIDOmitEmptyDecode(t *testing.T) {
	var s models.UserSettings
	if err := json.Unmarshal([]byte(`{"telegram":{"chat_id":1}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.BrandID != "" {
		t.Fatalf("legacy brand_id=%q", s.BrandID)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["brand_id"]; ok {
		t.Fatalf("empty brand_id must be omitted: %s", b)
	}
}
