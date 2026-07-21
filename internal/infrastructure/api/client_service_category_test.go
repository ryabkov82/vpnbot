package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func newCategoryTestClient(srv *httptest.Server, category string) *APIClient {
	cfg := &config.Config{}
	cfg.Brand.ServiceCategory = category
	return &APIClient{
		ServerURL:  srv.URL,
		HTTPClient: srv.Client(),
		config:     cfg,
	}
}

func writeServiceData(t *testing.T, w http.ResponseWriter, services []models.Service) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(struct {
		Data []models.Service `json:"data"`
	}{Data: services}); err != nil {
		t.Fatal(err)
	}
}

func writeUserServiceData(t *testing.T, w http.ResponseWriter, services []models.UserService) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(struct {
		Data []models.UserService `json:"data"`
	}{Data: services}); err != nil {
		t.Fatal(err)
	}
}

// --- GetServiceByID ---

func TestGetServiceByID_CategoryAddedToFilter(t *testing.T) {
	var gotFilter map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/service" {
			http.NotFound(w, r)
			return
		}
		gotFilter = mustUnescapeFilter(t, r.URL.Query().Get("filter"))
		writeServiceData(t, w, []models.Service{{ServiceID: 7, Name: "Main", AllowToOrder: 1, Category: "vpn-mz-main"}})
	}))
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	svc, err := c.GetServiceByID(7)
	if err != nil {
		t.Fatal(err)
	}
	if svc == nil || svc.ServiceID != 7 || svc.Category != "vpn-mz-main" {
		t.Fatalf("unexpected service: %+v", svc)
	}
	if gotFilter["service_id"] != float64(7) {
		t.Fatalf("filter service_id: %#v", gotFilter)
	}
	if gotFilter["allow_to_order"] != float64(1) {
		t.Fatalf("filter allow_to_order: %#v", gotFilter)
	}
	if gotFilter["category"] != "vpn-mz-main" {
		t.Fatalf("filter category: %#v", gotFilter)
	}
}

func TestGetServiceByID_EmptyCategoryNotInFilter(t *testing.T) {
	var gotFilter map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFilter = mustUnescapeFilter(t, r.URL.Query().Get("filter"))
		writeServiceData(t, w, []models.Service{{ServiceID: 7, AllowToOrder: 1, Category: "vpn-mz-anything"}})
	}))
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "")
	svc, err := c.GetServiceByID(7)
	if err != nil {
		t.Fatal(err)
	}
	if svc == nil || svc.ServiceID != 7 {
		t.Fatalf("unexpected service: %+v", svc)
	}
	if _, ok := gotFilter["category"]; ok {
		t.Fatalf("category must not be in filter for legacy empty config: %#v", gotFilter)
	}
}

func TestGetServiceByID_AllowedCategoryReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeServiceData(t, w, []models.Service{{ServiceID: 3, Name: "OK", AllowToOrder: 1, Category: "vpn-mz-main"}})
	}))
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	svc, err := c.GetServiceByID(3)
	if err != nil || svc == nil {
		t.Fatalf("want service, got svc=%v err=%v", svc, err)
	}
	if svc.Name != "OK" {
		t.Fatalf("unexpected service: %+v", svc)
	}
}

func TestGetServiceByID_OtherCategoryTreatedAsNotFound(t *testing.T) {
	// SHM (или его нестрогий фильтр) вернул услугу другой категории — локальная проверка обязана её отсечь.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeServiceData(t, w, []models.Service{{ServiceID: 9, Name: "Foreign", AllowToOrder: 1, Category: "vpn-mz-other"}})
	}))
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	svc, err := c.GetServiceByID(9)
	if svc != nil {
		t.Fatalf("service of other category must not be returned: %+v", svc)
	}
	if err == nil || !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("want ErrServiceNotFound, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("error text must stay compatible with existing 'not found' checks: %v", err)
	}
}

func TestGetServiceByID_NotOrderableStillUnavailable(t *testing.T) {
	// allow_to_order != 1 отсекается фильтром SHM: пустой data → not found.
	var gotFilter map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFilter = mustUnescapeFilter(t, r.URL.Query().Get("filter"))
		writeServiceData(t, w, nil)
	}))
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	svc, err := c.GetServiceByID(11)
	if svc != nil {
		t.Fatalf("unexpected service: %+v", svc)
	}
	if err == nil || !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("want ErrServiceNotFound, got %v", err)
	}
	if gotFilter["allow_to_order"] != float64(1) {
		t.Fatalf("allow_to_order filter must be kept: %#v", gotFilter)
	}
}

// --- GetUserServiceByUserID ---

type userServiceTestBackend struct {
	t           *testing.T
	userService models.UserService
	emptyData   bool
	filter      map[string]interface{}
	marzbanHits int
}

func (b *userServiceTestBackend) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/shm/v1/admin/user/service":
			b.filter = mustUnescapeFilter(b.t, r.URL.Query().Get("filter"))
			if b.emptyData {
				writeUserServiceData(b.t, w, nil)
				return
			}
			writeUserServiceData(b.t, w, []models.UserService{b.userService})
		case strings.HasPrefix(r.URL.Path, "/shm/v1/storage/manage/vpn_mrzb_"):
			b.marzbanHits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"subscription_url":"https://sub.example/key","links":[]}`))
		default:
			http.NotFound(w, r)
		}
	}
}

func TestGetUserServiceByUserID_CategoryAndOwnerFilterOK(t *testing.T) {
	be := &userServiceTestBackend{t: t, userService: models.UserService{
		ServiceID: 42, UserID: 5, Status: "ACTIVE", Category: "vpn-mz-main",
	}}
	srv := httptest.NewServer(be.handler())
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	us, err := c.GetUserServiceByUserID(5, "42")
	if err != nil {
		t.Fatal(err)
	}
	if us == nil || us.ServiceID != 42 {
		t.Fatalf("unexpected user service: %+v", us)
	}
	if be.filter["category"] != "vpn-mz-main" {
		t.Fatalf("category must be in SHM filter: %#v", be.filter)
	}
	if be.filter["user_id"] != float64(5) {
		t.Fatalf("user_id filter: %#v", be.filter)
	}
	if be.filter["user_service_id"] != "42" {
		t.Fatalf("user_service_id filter must be string: %#v", be.filter)
	}
	if be.marzbanHits != 1 {
		t.Fatalf("marzban key must be fetched after checks, hits=%d", be.marzbanHits)
	}
}

func TestGetUserServiceByUserID_OtherUserUnavailableNoMarzban(t *testing.T) {
	be := &userServiceTestBackend{t: t, userService: models.UserService{
		ServiceID: 42, UserID: 999, Status: "ACTIVE", Category: "vpn-mz-main",
	}}
	srv := httptest.NewServer(be.handler())
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	us, err := c.GetUserServiceByUserID(5, "42")
	if us != nil || !errors.Is(err, ErrUserServiceUnavailable) {
		t.Fatalf("want unavailable, got us=%v err=%v", us, err)
	}
	if be.marzbanHits != 0 {
		t.Fatalf("marzban must not be called, hits=%d", be.marzbanHits)
	}
}

func TestGetUserServiceByUserID_OtherServiceIDUnavailable(t *testing.T) {
	be := &userServiceTestBackend{t: t, userService: models.UserService{
		ServiceID: 99, UserID: 5, Status: "ACTIVE", Category: "vpn-mz-main",
	}}
	srv := httptest.NewServer(be.handler())
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	us, err := c.GetUserServiceByUserID(5, "42")
	if us != nil || !errors.Is(err, ErrUserServiceUnavailable) {
		t.Fatalf("want unavailable, got us=%v err=%v", us, err)
	}
	if be.marzbanHits != 0 {
		t.Fatalf("marzban hits=%d", be.marzbanHits)
	}
}

func TestGetUserServiceByUserID_OtherCategoryUnavailableNoMarzban(t *testing.T) {
	be := &userServiceTestBackend{t: t, userService: models.UserService{
		ServiceID: 42, UserID: 5, Status: "ACTIVE", Category: "vpn-mz-other",
	}}
	srv := httptest.NewServer(be.handler())
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	us, err := c.GetUserServiceByUserID(5, "42")
	if us != nil || !errors.Is(err, ErrUserServiceUnavailable) {
		t.Fatalf("want unavailable, got us=%v err=%v", us, err)
	}
	if be.marzbanHits != 0 {
		t.Fatalf("marzban hits=%d", be.marzbanHits)
	}
}

func TestGetUserServiceByUserID_EmptyDataUnavailable(t *testing.T) {
	be := &userServiceTestBackend{t: t, emptyData: true}
	srv := httptest.NewServer(be.handler())
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "vpn-mz-main")
	us, err := c.GetUserServiceByUserID(5, "42")
	if us != nil || !errors.Is(err, ErrUserServiceUnavailable) {
		t.Fatalf("want unavailable, got us=%v err=%v", us, err)
	}
}

func TestGetUserServiceByUserID_InvalidArgs(t *testing.T) {
	c := &APIClient{}
	cases := []struct {
		uid int
		sid string
	}{
		{0, "1"}, {-1, "1"}, {1, ""}, {1, "   "}, {1, "x"}, {1, "0"}, {1, "-3"},
	}
	for _, tc := range cases {
		_, err := c.GetUserServiceByUserID(tc.uid, tc.sid)
		if err == nil {
			t.Fatalf("uid=%d sid=%q: want error", tc.uid, tc.sid)
		}
	}
}

func TestGetUserServiceByUserID_EmptyCategoryLegacy(t *testing.T) {
	be := &userServiceTestBackend{t: t, userService: models.UserService{
		ServiceID: 42, UserID: 5, Status: "ACTIVE", Category: "vpn-mz-whatever",
	}}
	srv := httptest.NewServer(be.handler())
	t.Cleanup(srv.Close)

	c := newCategoryTestClient(srv, "")
	us, err := c.GetUserServiceByUserID(5, "42")
	if err != nil {
		t.Fatal(err)
	}
	if us == nil || us.ServiceID != 42 {
		t.Fatalf("legacy behaviour must return service: %+v", us)
	}
	if _, ok := be.filter["category"]; ok {
		t.Fatalf("category must not be in filter for legacy empty config: %#v", be.filter)
	}
	if be.marzbanHits != 1 {
		t.Fatalf("legacy marzban fetch expected, hits=%d", be.marzbanHits)
	}
}
