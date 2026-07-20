package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
)

func ownedTestCfg(category string) *config.Config {
	cfg := &config.Config{}
	cfg.API.Timeout = 5
	cfg.Services.Category = category
	return cfg
}

func TestGetOwnedUserServiceByUserID_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/shm/v1/admin/user/service":
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"user_service_id":100,"status":"ACTIVE","category":"vpn-mz-main","name":"ok"}]}`)
		case strings.HasPrefix(r.URL.Path, "/shm/v1/storage/manage/vpn_mrzb_"):
			_, _ = io.WriteString(w, `{"subscription_url":"https://sub.example","links":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	us, err := s.GetOwnedUserServiceByUserID(7, "100")
	if err != nil || us == nil || us.ServiceID != 100 {
		t.Fatalf("us=%v err=%v", us, err)
	}
}

func TestGetOwnedUserServiceByUserID_OtherUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"user_id":999,"user_service_id":100,"status":"ACTIVE","category":"vpn-mz-main"}]}`)
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	us, err := s.GetOwnedUserServiceByUserID(7, "100")
	if us != nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("want unavailable, got us=%v err=%v", us, err)
	}
}

func TestGetOwnedUserServiceByUserID_WrongID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"user_id":7,"user_service_id":101,"status":"ACTIVE","category":"vpn-mz-main"}]}`)
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	_, err := s.GetOwnedUserServiceByUserID(7, "100")
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("got %v", err)
	}
}

func TestGetOwnedUserServiceByUserID_OtherCategory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"user_id":7,"user_service_id":100,"status":"ACTIVE","category":"vpn-mz-other"}]}`)
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	_, err := s.GetOwnedUserServiceByUserID(7, "100")
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("got %v", err)
	}
}

func TestGetOwnedUserServiceByTelegramID_UserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	_, _, err := s.GetOwnedUserServiceByTelegramID(12345, "100")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestDownloadUserKey_OtherUserNoDownload(t *testing.T) {
	var downloadHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/shm/v1/admin/user":
			// Telegram user lookup
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"login":"@12345","settings":{"telegram":{"chat_id":12345}}}]}`)
		case r.URL.Path == "/shm/v1/admin/user/service":
			_, _ = io.WriteString(w, `{"data":[{"user_id":999,"user_service_id":100,"status":"ACTIVE","category":"vpn-mz-main"}]}`)
		case strings.Contains(r.URL.Path, "uploadDocumentFromStorage"):
			downloadHits++
			_, _ = io.WriteString(w, "key")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	_, err := s.DownloadUserKey(12345, "100")
	if !errors.Is(err, ErrUserServiceUnavailable) {
		t.Fatalf("got %v", err)
	}
	if downloadHits != 0 {
		t.Fatalf("download hits=%d", downloadHits)
	}
}

func TestGetUserKeyMarzban_OtherUserNoKey(t *testing.T) {
	var keyHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/shm/v1/admin/user":
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"login":"@12345","settings":{"telegram":{"chat_id":12345}}}]}`)
		case r.URL.Path == "/shm/v1/admin/user/service":
			_, _ = io.WriteString(w, `{"data":[{"user_id":999,"user_service_id":100,"status":"ACTIVE","category":"vpn-mz-main"}]}`)
		case strings.HasPrefix(r.URL.Path, "/shm/v1/storage/manage/vpn_mrzb_"):
			keyHits++
			_, _ = io.WriteString(w, `{"subscription_url":"https://x","links":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	_, err := s.GetUserKeyMarzban(12345, "100")
	if !errors.Is(err, ErrUserServiceUnavailable) {
		t.Fatalf("got %v", err)
	}
	if keyHits != 0 {
		t.Fatalf("key hits=%d", keyHits)
	}
}

func TestGetOwnedUserServiceByTelegramID_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/shm/v1/admin/user":
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"login":"@12345","settings":{"telegram":{"chat_id":12345}}}]}`)
		case r.URL.Path == "/shm/v1/admin/user/service":
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"user_service_id":100,"status":"ACTIVE","category":"vpn-mz-main","name":"ok"}]}`)
		case strings.HasPrefix(r.URL.Path, "/shm/v1/storage/manage/vpn_mrzb_"):
			_, _ = io.WriteString(w, `{"subscription_url":"https://sub.example","links":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	us, user, err := s.GetOwnedUserServiceByTelegramID(12345, "100")
	if err != nil || us == nil || user == nil || user.ID != 7 || us.ServiceID != 100 {
		t.Fatalf("us=%v user=%v err=%v", us, user, err)
	}
}

func TestDownloadUserKey_OwnServiceDownloads(t *testing.T) {
	var downloadHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/shm/v1/admin/user":
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"login":"@12345","settings":{"telegram":{"chat_id":12345}}}]}`)
		case r.URL.Path == "/shm/v1/admin/user/service":
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"user_service_id":100,"status":"ACTIVE","category":"vpn-mz-main"}]}`)
		case strings.HasPrefix(r.URL.Path, "/shm/v1/storage/manage/vpn_mrzb_"):
			_, _ = io.WriteString(w, `{"subscription_url":"https://sub.example","links":[]}`)
		case strings.Contains(r.URL.Path, "uploadDocumentFromStorage"):
			downloadHits++
			_, _ = io.WriteString(w, "plain-key-bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	body, err := s.DownloadUserKey(12345, "100")
	if err != nil || string(body) != "plain-key-bytes" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	if downloadHits != 1 {
		t.Fatalf("download hits=%d", downloadHits)
	}
}

func TestDeleteUserService_TelegramOtherUserNoDelete(t *testing.T) {
	var deleteHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			deleteHits++
		case r.URL.Path == "/shm/v1/admin/user":
			_, _ = io.WriteString(w, `{"data":[{"user_id":7,"login":"@12345","settings":{"telegram":{"chat_id":12345}}}]}`)
		case r.URL.Path == "/shm/v1/admin/user/service":
			_, _ = io.WriteString(w, `{"data":[{"user_id":999,"user_service_id":100,"status":"NOT PAID","category":"vpn-mz-main"}]}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"data":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := ownedTestCfg("vpn-mz-main")
	cfg.API.BaseURL = srv.URL
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	err := s.DeleteUserService(12345, "100")
	if err == nil || !errors.Is(err, ErrUserServiceUnavailable) {
		t.Fatalf("got %v", err)
	}
	if deleteHits != 0 {
		t.Fatalf("delete hits=%d", deleteHits)
	}
}
