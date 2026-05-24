package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

type stubPublicServicesApp struct {
	services []models.Service
	err      error
}

func (s stubPublicServicesApp) GetServices() ([]models.Service, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.services, nil
}

func TestServePublicServices_GET_OK(t *testing.T) {
	cfg := &config.Config{}
	app := stubPublicServicesApp{services: []models.Service{
		{
			ServiceID: 7,
			Name:      "API name",
			Descr:     "Описание из descr",
			Cost:      199,
			Period:    3,
		},
	}}
	h := servePublicServices(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type: got %q", ct)
	}
	var body publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Services) != 1 {
		t.Fatalf("services len: got %d", len(body.Services))
	}
	s0 := body.Services[0]
	if s0.ServiceID != 7 || s0.Name != "API name" || s0.Cost != 199 || s0.Period != 3 {
		t.Fatalf("unexpected item: %#v", s0)
	}
	if s0.Tier != publicTierStandard || s0.ConnectApp != publicConnectSubscription || len(s0.Badges) != 0 {
		t.Fatalf("standard tier fields: %#v", s0)
	}
	if s0.Description == "" {
		t.Fatal("expected non-empty description from BuildServicePreview")
	}
}

func TestServePublicServices_ExcludesTrialService(t *testing.T) {
	const trialID = 99
	cfg := &config.Config{
		Features: config.Features{
			Trial: config.TrialFeature{
				Enabled:       true,
				BaseServiceID: trialID,
			},
		},
	}
	app := stubPublicServicesApp{services: []models.Service{
		{ServiceID: 7, Name: "VPN 1 мес.", Cost: 199, Period: 1},
		{ServiceID: trialID, Name: "🎁 Тест на 7 дней", Cost: 0, Period: 0.25},
	}}
	h := servePublicServices(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Services) != 1 {
		t.Fatalf("services len: got %d, want 1", len(body.Services))
	}
	if body.Services[0].ServiceID != 7 {
		t.Fatalf("unexpected service_id: got %d, want 7", body.Services[0].ServiceID)
	}
	for _, s := range body.Services {
		if s.ServiceID == trialID {
			t.Fatalf("trial service %d must not be in response", trialID)
		}
	}
}

func TestServePublicServices_POST_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	h := servePublicServices(cfg, stubPublicServicesApp{services: nil})
	req := httptest.NewRequest(http.MethodPost, "/api/public/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
	if rec.Header().Get("Allow") != "GET" {
		t.Fatalf("Allow header: got %q", rec.Header().Get("Allow"))
	}
}

func TestServePublicServices_GetServicesError(t *testing.T) {
	cfg := &config.Config{}
	app := stubPublicServicesApp{err: errors.New("upstream down")}
	h := servePublicServices(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "services_unavailable" {
		t.Fatalf("body %#v", body)
	}
}

func TestServePublicServices_PremiumTierFromSquadMatch(t *testing.T) {
	squad := "premium-squad-alpha"
	cfg := &config.Config{PremiumSquadName: squad}
	app := stubPublicServicesApp{services: []models.Service{
		{
			ServiceID: 10,
			Name:      "Premium AntiBlock",
			Descr:     "Скрытое описание",
			Cost:      400,
			Period:    1,
			Config: &models.ServiceConfig{
				Remnawave: models.ServiceRemnawaveConfig{InternalSquadName: squad},
			},
		},
		{ServiceID: 7, Name: "Обычный", Cost: 100, Period: 1},
	}}
	h := servePublicServices(cfg, app)
	req := httptest.NewRequest(http.MethodGet, "/api/public/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(strings.ToLower(raw), "internal_squad_name") ||
		strings.Contains(raw, `"config"`) {
		t.Fatal("response must not contain internal config leak")
	}
	var body map[string][]map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	list := body["services"]
	if len(list) != 2 {
		t.Fatalf("got %d", len(list))
	}
	// Стандартные тарифы первыми, premium в конце
	if int(list[0]["service_id"].(float64)) != 7 || int(list[1]["service_id"].(float64)) != 10 {
		t.Fatalf("order: want standard(7) then premium(10), got %#v %#v", list[0]["service_id"], list[1]["service_id"])
	}
	var sawTen bool
	for _, row := range list {
		id, _ := row["service_id"].(float64)
		if int(id) == 10 {
			sawTen = true
			if row["tier"] != "premium" || row["connect_app"] != "happ" {
				t.Fatalf("%#v", row)
			}
			bdg, ok := row["badges"].([]interface{})
			if !ok || len(bdg) != 3 {
				t.Fatalf("badges: %#v", row["badges"])
			}
		}
	}
	if !sawTen {
		t.Fatal("missing premium service row")
	}
}

func TestBuildPublicServiceRowsFromList_StandardRowsBeforePremium_ThenPeriodCostServiceID(t *testing.T) {
	squad := "p-squad-ord"
	cfg := &config.Config{PremiumSquadName: squad}
	premiumSvc := func(id int, periodMonths int, cost float64) models.Service {
		return models.Service{
			ServiceID: id, Name: "P", Cost: cost, Period: float32(periodMonths), AllowToOrder: 1,
			Config: &models.ServiceConfig{Remnawave: models.ServiceRemnawaveConfig{InternalSquadName: squad}},
		}
	}
	stdSvc := func(id int, periodMonths int, cost float64) models.Service {
		return models.Service{ServiceID: id, Name: "S", Descr: "d", Cost: cost, Period: float32(periodMonths), AllowToOrder: 1}
	}

	list := []models.Service{
		premiumSvc(2, 1, 40),
		stdSvc(999, 12, 999),
		stdSvc(300, 3, 300),
		stdSvc(400, 6, 250),
		stdSvc(100, 1, 111),
		stdSvc(101, 1, 110),
		premiumSvc(501, 3, 100),
	}
	out := buildPublicServiceRowsFromList(cfg, list)
	got := make([]int, len(out))
	for i := range out {
		got[i] = out[i].ServiceID
	}
	want := []int{101, 100, 300, 400, 999, 2, 501}
	if !slices.Equal(got, want) {
		t.Fatalf("service_id order:\ngot  %v\nwant %v", got, want)
	}
	for i := 0; i < 5; i++ {
		if out[i].Tier != publicTierStandard {
			t.Fatalf("want standard tier at [%d]: %#v", i, out[i])
		}
	}
	for _, i := range []int{5, 6} {
		if out[i].Tier != publicTierPremium {
			t.Fatalf("want premium tier at [%d]: %#v", i, out[i])
		}
	}
}

func TestServePublicServices_AndAccountCatalog_ServicesOrderAligned(t *testing.T) {
	squad := "align-prem-squad"
	cfg := orderStartTestCfg()
	cfg.PremiumSquadName = squad
	cfg.API.BaseURL = "https://api.example.com"

	shm := []models.Service{
		{
			ServiceID: 900, Name: "Prem", Descr: "d", Cost: 10, Period: 1, AllowToOrder: 1,
			Config: &models.ServiceConfig{Remnawave: models.ServiceRemnawaveConfig{InternalSquadName: squad}},
		},
		{ServiceID: 800, Name: "12 мес.", Descr: "d", Cost: 888, Period: 12, AllowToOrder: 1},
		{ServiceID: 803, Name: "6 мес.", Descr: "d", Cost: 400, Period: 6, AllowToOrder: 1},
	}

	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "z@test.com", 1, "web_z", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{shmServices: shm}

	pubRec := httptest.NewRecorder()
	servePublicServices(cfg, stubPublicServicesApp{services: shm}).ServeHTTP(pubRec, httptest.NewRequest(http.MethodGet, "/api/public/services", nil))
	if pubRec.Code != http.StatusOK {
		t.Fatalf("public %d", pubRec.Code)
	}

	catRec := httptest.NewRecorder()
	serveAccountCatalogServices(cfg, st).ServeHTTP(catRec,
		httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok, nil))
	if catRec.Code != http.StatusOK {
		t.Fatalf("catalog %d %s", catRec.Code, catRec.Body.String())
	}

	var pubOut, catOut publicServicesListJSON
	if err := json.NewDecoder(pubRec.Body).Decode(&pubOut); err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(catRec.Body).Decode(&catOut); err != nil {
		t.Fatal(err)
	}
	pubIDs := make([]int, len(pubOut.Services))
	catIDs := make([]int, len(catOut.Services))
	for i := range pubOut.Services {
		pubIDs[i] = pubOut.Services[i].ServiceID
		catIDs[i] = catOut.Services[i].ServiceID
	}
	if !slices.Equal(pubIDs, catIDs) {
		t.Fatalf("order mismatch:\npublic %v\ncatalog %v", pubIDs, catIDs)
	}
	want := []int{803, 800, 900}
	if !slices.Equal(pubIDs, want) {
		t.Fatalf("got %v want %v", pubIDs, want)
	}
}
