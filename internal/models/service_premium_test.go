package models

import "testing"

func TestIsPremiumAntiBlockService_nil(t *testing.T) {
	if IsPremiumAntiBlockService(nil, "squad-a") {
		t.Fatal("nil service must be non-premium")
	}
}

func TestIsPremiumAntiBlockService_emptySquadConfig(t *testing.T) {
	if IsPremiumAntiBlockService(&Service{ServiceID: 1}, "") {
		t.Fatal("empty premiumSquadName")
	}
	if IsPremiumAntiBlockService(&Service{ServiceID: 1, Config: &ServiceConfig{}}, "x") {
		t.Fatal("empty internal squad")
	}
}

func TestIsPremiumAntiBlockService_nilConfig(t *testing.T) {
	if IsPremiumAntiBlockService(&Service{ServiceID: 1, Name: "X"}, "squad-x") {
		t.Fatal("nil Config")
	}
}

func TestIsPremiumAntiBlockService_matchAndTrim(t *testing.T) {
	s := &Service{
		ServiceID: 10,
		Config: &ServiceConfig{
			Remnawave: ServiceRemnawaveConfig{
				InternalSquadName: "  premium-antiblock  ",
			},
		},
	}
	name := "  premium-antiblock  "
	if !IsPremiumAntiBlockService(s, name) {
		t.Fatal("expected match with trim")
	}
	if IsPremiumAntiBlockService(s, "other") {
		t.Fatal("non-matching squad must fail")
	}
}
