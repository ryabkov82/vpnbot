package models

import (
	"encoding/json"
	"testing"
)

func TestServiceConfig_UnmarshalPricingAndRemnawave(t *testing.T) {
	raw := `{
		"remnawave": {
			"internal_squad_name": "squad-a",
			"bot": {
				"title": "Bot Title",
				"description": "Bot Desc"
			}
		},
		"pricing": {
			"public_code": "vpn_1m",
			"international_enabled": true,
			"international_currency": "USD",
			"international_amount_cents": 200
		}
	}`
	var cfg ServiceConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Remnawave.InternalSquadName != "squad-a" {
		t.Fatalf("squad: %q", cfg.Remnawave.InternalSquadName)
	}
	if cfg.Remnawave.Bot.Title != "Bot Title" || cfg.Remnawave.Bot.Description != "Bot Desc" {
		t.Fatalf("bot: %#v", cfg.Remnawave.Bot)
	}
	if cfg.Pricing.PublicCode != "vpn_1m" {
		t.Fatalf("public_code: %q", cfg.Pricing.PublicCode)
	}
	if !cfg.Pricing.InternationalEnabled {
		t.Fatal("international_enabled want true")
	}
	if cfg.Pricing.InternationalCurrency != "USD" {
		t.Fatalf("currency: %q", cfg.Pricing.InternationalCurrency)
	}
	if cfg.Pricing.InternationalAmountCents != 200 {
		t.Fatalf("cents: %d", cfg.Pricing.InternationalAmountCents)
	}
}

func TestService_UnmarshalFullPayload(t *testing.T) {
	raw := `{
		"service_id": 3,
		"name": "1 месяц",
		"descr": "d",
		"cost": 150,
		"period": 1,
		"allow_to_order": 1,
		"config": {
			"remnawave": {"internal_squad_name": "x", "bot": {"title": "T", "description": "D"}},
			"pricing": {
				"public_code": "vpn_1m",
				"international_enabled": true,
				"international_currency": "USD",
				"international_amount_cents": 200
			}
		}
	}`
	var s Service
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	if s.ServiceID != 3 || s.Cost != 150 {
		t.Fatalf("service: %#v", s)
	}
	if s.Config == nil || s.Config.Pricing.PublicCode != "vpn_1m" {
		t.Fatalf("pricing: %#v", s.Config)
	}
	preview := BuildServicePreview(&s)
	if preview.Title != "T" || preview.Description != "D" || preview.Cost != 150 {
		t.Fatalf("preview: %#v", preview)
	}
}
