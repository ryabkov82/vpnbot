package service

import (
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
	var gotUID, gotUS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/shm/v1/admin/user/service" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		gotUID = r.URL.Query().Get("user_id")
		gotUS = r.URL.Query().Get("user_service_id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.API.BaseURL = srv.URL
	cfg.API.Timeout = 5
	cli := api.NewAPIClient(cfg)
	s := NewService(cli, config.BrandConfig{})

	err := s.DeleteUserServiceByUserID(42, "337")
	if err != nil {
		t.Fatal(err)
	}
	if gotUID != "42" || gotUS != "337" {
		t.Fatalf("query got user_id=%q user_service_id=%q", gotUID, gotUS)
	}
}
