package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
)

func TestDeleteUserServiceByUserID_InvalidUserID(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	err := s.DeleteUserServiceByUserID(0, "5")
	if err == nil || !strings.Contains(err.Error(), "invalid user id") {
		t.Fatalf("want invalid user id, got %v", err)
	}
}

func TestDeleteUserServiceByUserID_InvalidServiceID(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	for _, sid := range []string{"", "   ", "\t"} {
		err := s.DeleteUserServiceByUserID(10, sid)
		if err == nil || !strings.Contains(err.Error(), "invalid service id") {
			t.Fatalf("sid %q: want invalid service id, got %v", sid, err)
		}
	}
}

func TestDeleteUserServiceByUserID_ProxyToAPIOK(t *testing.T) {
	var deleteHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/shm/v1/admin/user/service":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{"user_id":42,"user_service_id":337,"status":"NOT PAID","category":"vpn-mz-main","name":"x"}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/shm/v1/admin/user/service":
			deleteHits++
			if r.URL.Query().Get("user_id") != "42" || r.URL.Query().Get("user_service_id") != "337" {
				t.Fatalf("delete query: %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.API.BaseURL = srv.URL
	cfg.API.Timeout = 5
	cfg.Services.Category = "vpn-mz-main"
	cli := api.NewAPIClient(cfg)
	s := NewService(cli, cfg.EffectiveBrand())

	err := s.DeleteUserServiceByUserID(42, "337")
	if err != nil {
		t.Fatal(err)
	}
	if deleteHits != 1 {
		t.Fatalf("delete hits=%d", deleteHits)
	}
}

func TestDeleteUserServiceByUserID_OtherUserNoDelete(t *testing.T) {
	var deleteHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteHits++
		}
		w.Header().Set("Content-Type", "application/json")
		// SHM игнорирует фильтр и отдаёт чужую строку
		_, _ = io.WriteString(w, `{"data":[{"user_id":999,"user_service_id":337,"status":"NOT PAID","category":"vpn-mz-main"}]}`)
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.API.BaseURL = srv.URL
	cfg.API.Timeout = 5
	cfg.Services.Category = "vpn-mz-main"
	s := NewService(api.NewAPIClient(cfg), cfg.EffectiveBrand())

	err := s.DeleteUserServiceByUserID(42, "337")
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("want unavailable, got %v", err)
	}
	if deleteHits != 0 {
		t.Fatalf("delete must not be called, hits=%d", deleteHits)
	}
}
