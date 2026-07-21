package bot

import (
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

func cfgWithServiceCategory(category string) *config.Config {
	cfg := &config.Config{}
	cfg.Brand.ServiceCategory = category
	return cfg
}

func TestOrderServiceCategoryAllowed_AllowedCategory(t *testing.T) {
	cfg := cfgWithServiceCategory("vpn-mz-main")
	svc := &models.Service{ServiceID: 3, AllowToOrder: 1, Category: "vpn-mz-main"}
	if !orderServiceCategoryAllowed(cfg, svc) {
		t.Fatal("service of configured category must be orderable")
	}
}

func TestOrderServiceCategoryAllowed_OtherCategoryDenied(t *testing.T) {
	cfg := cfgWithServiceCategory("vpn-mz-main")
	svc := &models.Service{ServiceID: 9, AllowToOrder: 1, Category: "vpn-mz-other"}
	if orderServiceCategoryAllowed(cfg, svc) {
		t.Fatal("service of other category must not be orderable")
	}
}

func TestOrderServiceCategoryAllowed_TrialServiceOtherCategoryDenied(t *testing.T) {
	// Trial/start-параметр не обходит проверку: handleTrial и buildTrialRow
	// используют этот же helper для услуги features.trial.base_service_id.
	cfg := cfgWithServiceCategory("vpn-mz-main")
	cfg.Features.Trial.Enabled = true
	cfg.Features.Trial.BaseServiceID = 9
	trialSvc := &models.Service{ServiceID: 9, AllowToOrder: 1, Category: "vpn-mz-other"}
	if orderServiceCategoryAllowed(cfg, trialSvc) {
		t.Fatal("trial service of other category must not be orderable")
	}
}

func TestOrderServiceCategoryAllowed_EmptyCategoryLegacy(t *testing.T) {
	cfg := cfgWithServiceCategory("")
	svc := &models.Service{ServiceID: 3, AllowToOrder: 1, Category: "vpn-mz-anything"}
	if !orderServiceCategoryAllowed(cfg, svc) {
		t.Fatal("empty configured category must keep legacy behaviour")
	}
	if !orderServiceCategoryAllowed(nil, svc) {
		t.Fatal("nil config must keep legacy behaviour")
	}
}

func TestOrderServiceCategoryAllowed_NilServiceDenied(t *testing.T) {
	if orderServiceCategoryAllowed(cfgWithServiceCategory(""), nil) {
		t.Fatal("nil service must never be orderable")
	}
	if orderServiceCategoryAllowed(cfgWithServiceCategory("vpn-mz-main"), nil) {
		t.Fatal("nil service must never be orderable")
	}
}
