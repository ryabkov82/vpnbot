package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func orderBrandCfg(id, category string) config.BrandConfig {
	cfg := &config.Config{}
	cfg.Brand.ID = id
	cfg.Brand.Name = id
	cfg.Brand.AllowedHosts = []string{"example.test"}
	cfg.Brand.PublicBaseURL = "https://example.test"
	cfg.Brand.LandingURL = "https://landing.test"
	cfg.Brand.ServiceCategory = category
	cfg.Brand.WebUserLoginPrefix = "web_"
	if id == "fc" {
		cfg.Brand.WebUserLoginPrefix = "web_fc_"
	}
	cfg.Brand.WebUserSource = "example.test"
	cfg.Brand.PaymentProfile = id + "_pay"
	return cfg.EffectiveBrand()
}

type orderTestBackend struct {
	t            *testing.T
	serviceJSON  string
	orderHits    atomic.Int32
	lookupHits   atomic.Int32
	lastOrderRaw []byte
	lookupSeq    []string
}

func (b *orderTestBackend) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/shm/v1/admin/service" && r.Method == http.MethodGet:
			b.lookupHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if n := len(b.lookupSeq); n > 0 {
				idx := int(b.lookupHits.Load()) - 1
				if idx < 0 {
					idx = 0
				}
				if idx >= n {
					idx = n - 1
				}
				_, _ = io.WriteString(w, b.lookupSeq[idx])
				return
			}
			if b.serviceJSON == "" {
				_, _ = io.WriteString(w, `{"data":[]}`)
				return
			}
			_, _ = io.WriteString(w, b.serviceJSON)
		case r.URL.Path == "/shm/v1/admin/service/order" && r.Method == http.MethodPut:
			b.orderHits.Add(1)
			body, _ := io.ReadAll(r.Body)
			b.lastOrderRaw = body
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{"user_service_id":501,"service_id":3,"status":"NOT PAID","name":"plan"}]}`)
		default:
			http.NotFound(w, r)
		}
	}
}

func newOrderTestService(t *testing.T, brand config.BrandConfig, be *orderTestBackend) *Service {
	t.Helper()
	srv := httptest.NewServer(be.handler())
	t.Cleanup(srv.Close)
	cfg := &config.Config{}
	cfg.API.BaseURL = srv.URL
	cfg.API.Timeout = 5
	cfg.Brand = brand
	return NewService(api.NewAPIClient(cfg), brand)
}

func TestServiceOrderByUserID_VFFCategoryOK(t *testing.T) {
	be := &orderTestBackend{t: t, serviceJSON: `{"data":[{"service_id":3,"allow_to_order":1,"cost":100,"category":"vpn-mz-test","name":"vff"}]}`}
	s := newOrderTestService(t, orderBrandCfg("vff", "vpn-mz-test"), be)
	us, err := s.ServiceOrderByUserID(42, 3)
	if err != nil {
		t.Fatal(err)
	}
	if us == nil || us.ServiceID != 501 {
		t.Fatalf("us=%+v", us)
	}
	if be.orderHits.Load() != 1 {
		t.Fatalf("ServiceOrder calls=%d", be.orderHits.Load())
	}
	if be.lookupHits.Load() != 1 {
		t.Fatalf("lookup calls=%d", be.lookupHits.Load())
	}
}

func TestServiceOrderByUserID_FCCategoryOK(t *testing.T) {
	be := &orderTestBackend{t: t, serviceJSON: `{"data":[{"service_id":8,"allow_to_order":1,"cost":100,"category":"vpn-mz-fc","name":"fc"}]}`}
	s := newOrderTestService(t, orderBrandCfg("fc", "vpn-mz-fc"), be)
	_, err := s.ServiceOrderByUserID(7, 8)
	if err != nil {
		t.Fatal(err)
	}
	if be.orderHits.Load() != 1 {
		t.Fatalf("ServiceOrder calls=%d", be.orderHits.Load())
	}
}

func TestServiceOrderByUserID_CrossBrandFCGetsVFF(t *testing.T) {
	be := &orderTestBackend{t: t, serviceJSON: `{"data":[{"service_id":3,"allow_to_order":1,"cost":100,"category":"vpn-mz-test","name":"vff"}]}`}
	s := newOrderTestService(t, orderBrandCfg("fc", "vpn-mz-fc"), be)
	_, err := s.ServiceOrderByUserID(7, 3)
	if err == nil {
		t.Fatal("expected denial")
	}
	if be.orderHits.Load() != 0 {
		t.Fatalf("ServiceOrder must not be called, hits=%d", be.orderHits.Load())
	}
	// API GetServiceByID treats other category as not found; service layer wraps it.
	if !errors.Is(err, api.ErrServiceNotFound) && !errors.Is(err, ErrServiceCategoryDenied) {
		t.Fatalf("want not found or category denied, got %v", err)
	}
}

func TestServiceOrderByUserID_CrossBrandVFFGetsFC(t *testing.T) {
	be := &orderTestBackend{t: t, serviceJSON: `{"data":[{"service_id":8,"allow_to_order":1,"cost":100,"category":"vpn-mz-fc","name":"fc"}]}`}
	s := newOrderTestService(t, orderBrandCfg("vff", "vpn-mz-test"), be)
	_, err := s.ServiceOrderByUserID(42, 8)
	if err == nil || be.orderHits.Load() != 0 {
		t.Fatalf("must deny, err=%v orderHits=%d", err, be.orderHits.Load())
	}
}

func TestValidateOrderedService_EmptyActualCategory(t *testing.T) {
	err := validateOrderedService(&models.Service{ServiceID: 3, Category: "  "}, 3, "vpn-mz-test")
	if !errors.Is(err, ErrServiceCategoryDenied) {
		t.Fatalf("err=%v", err)
	}
	var denied *ServiceCategoryDeniedError
	if !errors.As(err, &denied) || denied.Expected != "vpn-mz-test" || denied.Actual != "" {
		t.Fatalf("denied=%+v", denied)
	}
}

func TestServiceOrderByUserID_EmptyExpectedCategory(t *testing.T) {
	be := &orderTestBackend{t: t, serviceJSON: `{"data":[{"service_id":3,"allow_to_order":1,"cost":100,"category":"vpn-mz-test","name":"x"}]}`}
	s := newOrderTestService(t, orderBrandCfg("vff", ""), be)
	_, err := s.ServiceOrderByUserID(1, 3)
	if !errors.Is(err, ErrServiceCategoryDenied) {
		t.Fatalf("err=%v", err)
	}
	if be.orderHits.Load() != 0 || be.lookupHits.Load() != 0 {
		t.Fatalf("must fail before SHM calls: order=%d lookup=%d", be.orderHits.Load(), be.lookupHits.Load())
	}
}

func TestServiceOrderByUserID_LookupError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/shm/v1/admin/service" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "boom")
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{}
	cfg.API.BaseURL = srv.URL
	cfg.API.Timeout = 5
	brand := orderBrandCfg("vff", "vpn-mz-test")
	cfg.Brand = brand
	s := NewService(api.NewAPIClient(cfg), brand)
	_, err := s.ServiceOrderByUserID(1, 3)
	if err == nil || !strings.Contains(err.Error(), "service order lookup") {
		t.Fatalf("err=%v", err)
	}
}

func TestServiceOrderByUserID_ServiceNotFound(t *testing.T) {
	be := &orderTestBackend{t: t, serviceJSON: `{"data":[]}`}
	s := newOrderTestService(t, orderBrandCfg("vff", "vpn-mz-test"), be)
	_, err := s.ServiceOrderByUserID(1, 3)
	if err == nil || !errors.Is(err, api.ErrServiceNotFound) {
		t.Fatalf("err=%v", err)
	}
	if be.orderHits.Load() != 0 {
		t.Fatal("ServiceOrder must not be called")
	}
}

func TestValidateOrderedService_IDMismatch(t *testing.T) {
	err := validateOrderedService(&models.Service{ServiceID: 9, Category: "vpn-mz-test"}, 3, "vpn-mz-test")
	if !errors.Is(err, ErrServiceCategoryDenied) {
		t.Fatalf("err=%v", err)
	}
}

func TestServiceOrderByUserID_RegressionRequestBody(t *testing.T) {
	be := &orderTestBackend{t: t, serviceJSON: `{"data":[{"service_id":3,"allow_to_order":1,"cost":100,"category":"vpn-mz-test","name":"vff"}]}`}
	s := newOrderTestService(t, orderBrandCfg("vff", "vpn-mz-test"), be)
	us, err := s.ServiceOrderByUserID(42, 3)
	if err != nil || us == nil {
		t.Fatalf("us=%v err=%v", us, err)
	}
	var body map[string]any
	if err := json.Unmarshal(be.lastOrderRaw, &body); err != nil {
		t.Fatal(err)
	}
	if body["user_id"] != float64(42) || body["service_id"] != float64(3) {
		t.Fatalf("body=%v", body)
	}
	if body["check_exists_unpaid"] != float64(1) {
		t.Fatalf("check_exists_unpaid=%v", body["check_exists_unpaid"])
	}
	if us.Status != "NOT PAID" || us.BaseServiceID != 3 {
		t.Fatalf("result=%+v", us)
	}
}

func TestServiceOrderByUserID_RelookupForeignCategoryBlocked(t *testing.T) {
	// Handler-level "allowed" fixture is irrelevant: mutation-boundary relookup returns foreign category.
	be := &orderTestBackend{
		t: t,
		lookupSeq: []string{
			`{"data":[{"service_id":3,"allow_to_order":1,"cost":100,"category":"vpn-mz-fc","name":"fc"}]}`,
		},
	}
	s := newOrderTestService(t, orderBrandCfg("vff", "vpn-mz-test"), be)
	_, err := s.ServiceOrderByUserID(42, 3)
	if err == nil || be.orderHits.Load() != 0 {
		t.Fatalf("must block on relookup, err=%v orderHits=%d", err, be.orderHits.Load())
	}
}

func TestValidateOrderedService_Mismatch(t *testing.T) {
	err := validateOrderedService(&models.Service{ServiceID: 3, Category: "vpn-mz-test"}, 3, "vpn-mz-fc")
	if !errors.Is(err, ErrServiceCategoryDenied) {
		t.Fatalf("err=%v", err)
	}
	var denied *ServiceCategoryDeniedError
	if !errors.As(err, &denied) || denied.Expected != "vpn-mz-fc" || denied.Actual != "vpn-mz-test" {
		t.Fatalf("denied=%+v", denied)
	}
}

func TestValidateOrderedService_NilService(t *testing.T) {
	err := validateOrderedService(nil, 3, "vpn-mz-test")
	if !errors.Is(err, api.ErrServiceNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateOrderedService_EmptyExpected(t *testing.T) {
	err := validateOrderedService(&models.Service{ServiceID: 3, Category: "vpn-mz-test"}, 3, "  ")
	if !errors.Is(err, ErrServiceCategoryDenied) {
		t.Fatalf("err=%v", err)
	}
}
