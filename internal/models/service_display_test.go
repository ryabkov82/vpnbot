package models

import (
	"encoding/json"
	"testing"
)

func TestServiceConfig_UnmarshalDisplay(t *testing.T) {
	raw := `{
		"display": {
			"ru": {"title": "1 месяц", "description": "Подписка VPN"},
			"en": {"title": "1 month", "description": "VPN subscription"}
		}
	}`
	var cfg ServiceConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Display.RU.Title != "1 месяц" || cfg.Display.RU.Description != "Подписка VPN" {
		t.Fatalf("ru: %#v", cfg.Display.RU)
	}
	if cfg.Display.EN.Title != "1 month" || cfg.Display.EN.Description != "VPN subscription" {
		t.Fatalf("en: %#v", cfg.Display.EN)
	}
}

func TestBuildCatalogServiceTexts_RU_DisplayOverridesRemnawave(t *testing.T) {
	s := &Service{
		Name:  "ignored",
		Descr: "ignored descr",
		Config: &ServiceConfig{
			Remnawave: ServiceRemnawaveConfig{
				Bot: ServiceBotConfig{Title: "Bot T", Description: "Bot D"},
			},
			Display: ServiceDisplayConfig{
				RU: ServiceDisplayLocaleConfig{Title: "RU Title", Description: "RU Desc"},
			},
		},
	}
	title, desc := BuildCatalogServiceTexts(s, CatalogLocaleRU)
	if title != "RU Title" || desc != "RU Desc" {
		t.Fatalf("got %q / %q", title, desc)
	}
}

func TestBuildCatalogServiceTexts_RU_FallbackBuildServicePreview(t *testing.T) {
	s := &Service{
		Name:  "Base",
		Descr: "Descr",
		Config: &ServiceConfig{
			Remnawave: ServiceRemnawaveConfig{
				Bot: ServiceBotConfig{Title: "Bot T", Description: "Bot D"},
			},
		},
	}
	title, desc := BuildCatalogServiceTexts(s, CatalogLocaleRU)
	if title != "Bot T" || desc != "Bot D" {
		t.Fatalf("got %q / %q", title, desc)
	}
}

func TestBuildCatalogServiceTexts_EN_Display(t *testing.T) {
	s := &Service{
		Name:   "1 месяц",
		Period: 1,
		Config: &ServiceConfig{
			Display: ServiceDisplayConfig{
				EN: ServiceDisplayLocaleConfig{Title: "Premium VPN", Description: "English desc"},
			},
		},
	}
	title, desc := BuildCatalogServiceTexts(s, CatalogLocaleEN)
	if title != "Premium VPN" || desc != "English desc" {
		t.Fatalf("got %q / %q", title, desc)
	}
}

func TestBuildCatalogServiceTexts_EN_PeriodFallback(t *testing.T) {
	cases := []struct {
		period float32
		want   string
	}{
		{1, "1 month"},
		{3, "3 months"},
		{6, "6 months"},
		{12, "12 months"},
	}
	for _, tc := range cases {
		s := &Service{Name: "1 месяц", Period: tc.period}
		title, desc := BuildCatalogServiceTexts(s, CatalogLocaleEN)
		if title != tc.want {
			t.Fatalf("period=%v title=%q want %q", tc.period, title, tc.want)
		}
		if desc != defaultENCatalogDescription {
			t.Fatalf("period=%v desc=%q", tc.period, desc)
		}
	}
}

func TestBuildCatalogServiceTexts_EN_DisplayTitleOnlyUsesDefaultDescription(t *testing.T) {
	s := &Service{
		Period: 1,
		Config: &ServiceConfig{
			Display: ServiceDisplayConfig{
				EN: ServiceDisplayLocaleConfig{Title: "Custom"},
			},
		},
	}
	title, desc := BuildCatalogServiceTexts(s, CatalogLocaleEN)
	if title != "Custom" || desc != defaultENCatalogDescription {
		t.Fatalf("got %q / %q", title, desc)
	}
}
