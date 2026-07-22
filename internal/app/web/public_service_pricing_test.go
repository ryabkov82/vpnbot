package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func serviceWithPricing(id int, name string, cost float64, period float32, cents int64) models.Service {
	return models.Service{
		ServiceID:    id,
		Name:         name,
		Descr:        "desc",
		Cost:         cost,
		Period:       period,
		AllowToOrder: 1,
		Config: &models.ServiceConfig{
			Pricing: models.ServicePricingConfig{
				PublicCode:               "vpn_1m",
				InternationalEnabled:     true,
				InternationalCurrency:    "USD",
				InternationalAmountCents: cents,
			},
		},
	}
}

func TestFormatUSDFromCents(t *testing.T) {
	cases := []struct {
		cents int64
		want  string
	}{
		{200, "$2"},
		{500, "$5"},
		{800, "$8"},
		{1200, "$12"},
	}
	for _, tc := range cases {
		if got := formatUSDFromCents(tc.cents); got != tc.want {
			t.Fatalf("cents=%d got %q want %q", tc.cents, got, tc.want)
		}
	}
}

func TestFormatUSDMonthlyFromCents(t *testing.T) {
	cases := []struct {
		cents  int64
		period float64
		want   string
	}{
		{200, 1, "$2/mo"},
		{500, 3, "$1.67/mo"},
		{800, 6, "$1.33/mo"},
		{1200, 12, "$1/mo"},
	}
	for _, tc := range cases {
		if got := formatUSDMonthlyFromCents(tc.cents, tc.period); got != tc.want {
			t.Fatalf("cents=%d period=%v got %q want %q", tc.cents, tc.period, got, tc.want)
		}
	}
}

func TestApplyPublicServicePricing_RU_RUB(t *testing.T) {
	s := serviceWithPricing(3, "1 месяц", 150, 1, 200)
	row := publicServiceJSON{Period: 1}
	applyPublicServicePricing(&row, &s, 150, accountLocaleRU)
	if row.DisplayCurrency != "RUB" || row.DisplayAmountText != "150 ₽" {
		t.Fatalf("display: currency=%q text=%q", row.DisplayCurrency, row.DisplayAmountText)
	}
	if row.DisplayMonthlyText != "" {
		t.Fatalf("monthly: %q", row.DisplayMonthlyText)
	}
	if row.ActualCurrency != "RUB" || row.ActualAmount != 150 {
		t.Fatalf("actual: %#v", row)
	}
	if row.PublicCode != "vpn_1m" || !row.InternationalEnabled || row.InternationalAmountCents != 200 {
		t.Fatalf("intl fields: %#v", row)
	}
}

func TestApplyPublicServicePricing_EN_USD(t *testing.T) {
	s := serviceWithPricing(3, "1 месяц", 150, 1, 200)
	row := publicServiceJSON{Period: 1}
	applyPublicServicePricing(&row, &s, 150, accountLocaleEN)
	if row.DisplayCurrency != "USD" || row.DisplayAmountText != "$2" || row.DisplayMonthlyText != "$2/mo" {
		t.Fatalf("display: %#v", row)
	}
}

func TestApplyPublicServicePricing_EN_FallbackRUB(t *testing.T) {
	s := models.Service{ServiceID: 3, Name: "1 mo", Cost: 150, Period: 1}
	row := publicServiceJSON{Period: 1}
	applyPublicServicePricing(&row, &s, 150, accountLocaleEN)
	if row.DisplayCurrency != "RUB" || row.DisplayAmountText != "150 RUB" {
		t.Fatalf("fallback: %#v", row)
	}
}

func TestBuildPublicServiceRowsFromList_RemnawaveUnchanged(t *testing.T) {
	squad := "secret-squad-x"
	s := models.Service{
		ServiceID:    9,
		Name:         "Base",
		Descr:        "Fallback descr",
		Cost:         99,
		Period:       1,
		AllowToOrder: 1,
		Config: &models.ServiceConfig{
			Remnawave: models.ServiceRemnawaveConfig{
				InternalSquadName: squad,
				Bot: models.ServiceBotConfig{
					Title:       "Bot Title",
					Description: "Bot Desc",
				},
			},
		},
	}
	cfg := &config.Config{PremiumSquadName: squad}
	out := buildPublicServiceRowsFromList(cfg, []models.Service{s}, accountLocaleRU)
	if len(out) != 1 {
		t.Fatal(out)
	}
	if out[0].Name != "Bot Title" || out[0].Description != "Bot Desc" {
		t.Fatalf("preview: %#v", out[0])
	}
	if out[0].Tier != publicTierPremium {
		t.Fatalf("tier: %q", out[0].Tier)
	}
}

func TestServeAccountCatalog_EN_USDDisplay(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "u@test.com", 5, "web_ab", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{serviceWithPricing(3, "1 месяц", 150, 1, 200)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok+"&lang=en", nil)
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Services) != 1 {
		t.Fatalf("%#v", out.Services)
	}
	svc := out.Services[0]
	if svc.DisplayCurrency != "USD" || svc.DisplayAmountText != "$2" || svc.DisplayMonthlyText != "$2/mo" {
		t.Fatalf("USD display: %#v", svc)
	}
	if svc.PublicCode != "vpn_1m" {
		t.Fatalf("public_code: %q", svc.PublicCode)
	}
}

func TestServeAccountCatalog_EN_DisplayLocalizedCopy(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "u@test.com", 5, "web_ab", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{{
			ServiceID:    3,
			Name:         "1 месяц",
			Descr:        "Русское описание",
			Cost:         150,
			Period:       1,
			AllowToOrder: 1,
			Config: &models.ServiceConfig{
				Remnawave: models.ServiceRemnawaveConfig{
					Bot: models.ServiceBotConfig{Title: "Bot RU", Description: "Bot desc RU"},
				},
				Pricing: models.ServicePricingConfig{
					InternationalEnabled:     true,
					InternationalCurrency:    "USD",
					InternationalAmountCents: 200,
				},
				Display: models.ServiceDisplayConfig{
					EN: models.ServiceDisplayLocaleConfig{
						Title:       "1 month VPN",
						Description: "Secure VPN access for one month.",
					},
				},
			},
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok+"&lang=en", nil)
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	svc := out.Services[0]
	if svc.Name != "1 month VPN" || svc.Description != "Secure VPN access for one month." {
		t.Fatalf("localized: %#v", svc)
	}
	raw := strings.ToLower(rec.Body.String())
	for _, leaked := range []string{"config", "remnawave", "internal_squad", "allow_to_order"} {
		if strings.Contains(raw, leaked) {
			t.Fatalf("leaked %s: %s", leaked, rec.Body.String())
		}
	}
}

func TestServeAccountCatalog_EN_PeriodFallbackCopy(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "u@test.com", 1, "lg", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{
			{ServiceID: 3, Name: "1 месяц", Descr: "d", Cost: 150, Period: 1, AllowToOrder: 1},
			{ServiceID: 4, Name: "3 месяца", Descr: "d", Cost: 400, Period: 3, AllowToOrder: 1},
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok+"&lang=en", nil)
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec, req)
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	want := map[int]struct{ name, desc string }{
		3: {"1 month", "VPN subscription for the selected period."},
		4: {"3 months", "VPN subscription for the selected period."},
	}
	for _, svc := range out.Services {
		w, ok := want[svc.ServiceID]
		if !ok {
			continue
		}
		if svc.Name != w.name || svc.Description != w.desc {
			t.Fatalf("service %d: name=%q desc=%q want %q / %q", svc.ServiceID, svc.Name, svc.Description, w.name, w.desc)
		}
	}
}

func TestServeAccountCatalog_RU_UnchangedWithoutDisplay(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "u@test.com", 5, "web_ab", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{serviceWithPricing(3, "1 месяц", 150, 1, 200)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok, nil)
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	svc := out.Services[0]
	if svc.DisplayCurrency != "RUB" || svc.DisplayAmountText != "150 ₽" {
		t.Fatalf("RUB display: %#v", svc)
	}
	if svc.Name != "1 месяц" {
		t.Fatalf("RU name unchanged: %q", svc.Name)
	}
}

func TestServeAccountCatalog_EN_MonthlyPeriods(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "u@test.com", 1, "lg", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{
			serviceWithPricing(3, "3m", 450, 3, 500),
			serviceWithPricing(4, "6m", 800, 6, 800),
			serviceWithPricing(5, "12m", 1200, 12, 1200),
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok+"&lang=en", nil)
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var out publicServicesListJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	want := map[int]string{
		3: "$1.67/mo",
		4: "$1.33/mo",
		5: "$1/mo",
	}
	for _, svc := range out.Services {
		if got, ok := want[svc.ServiceID]; !ok || svc.DisplayMonthlyText != got {
			t.Fatalf("service %d monthly=%q want %q", svc.ServiceID, svc.DisplayMonthlyText, got)
		}
	}
}

func TestServeAccountCatalog_PricingNoInternalLeak(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "u@test.com", 12, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		shmServices: []models.Service{{
			ServiceID:    31,
			Name:         "N",
			Cost:         199,
			Period:       2,
			AllowToOrder: 1,
			Config: &models.ServiceConfig{
				Remnawave: models.ServiceRemnawaveConfig{
					InternalSquadName: "secret-squad",
					Bot:               models.ServiceBotConfig{Title: "BT", Description: "BD"},
				},
				Pricing: models.ServicePricingConfig{
					PublicCode:               "vpn_2m",
					InternationalEnabled:     true,
					InternationalCurrency:    "USD",
					InternationalAmountCents: 500,
				},
			},
		}},
	}
	rec := httptest.NewRecorder()
	serveAccountCatalogServices(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/account/catalog/services?token="+tok+"&lang=en", nil))
	raw := strings.ToLower(rec.Body.String())
	for _, leaked := range []string{"config", "allow_to_order", "internal_squad"} {
		if strings.Contains(raw, leaked) {
			t.Fatalf("leaked %s in body: %s", leaked, rec.Body.String())
		}
	}
	if !strings.Contains(rec.Body.String(), `"public_code":"vpn_2m"`) {
		t.Fatalf("public_code missing: %s", rec.Body.String())
	}
}
