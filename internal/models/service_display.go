package models

import (
	"fmt"
	"math"
	"strings"
)

// CatalogLocale — язык каталога услуг (web account / public offer).
type CatalogLocale string

const (
	CatalogLocaleRU CatalogLocale = "ru"
	CatalogLocaleEN CatalogLocale = "en"
)

const defaultENCatalogDescription = "VPN subscription for the selected period."

// BuildCatalogServiceTexts возвращает title/description для каталога с учётом config.display.
func BuildCatalogServiceTexts(s *Service, locale CatalogLocale) (title, description string) {
	preview := BuildServicePreview(s)

	var displayTitle, displayDesc string
	if s != nil && s.Config != nil {
		loc := s.Config.Display.RU
		if locale == CatalogLocaleEN {
			loc = s.Config.Display.EN
		}
		displayTitle = strings.TrimSpace(loc.Title)
		displayDesc = strings.TrimSpace(loc.Description)
	}

	if locale == CatalogLocaleEN {
		title = displayTitle
		if title == "" && s != nil {
			title = formatENPeriodTitle(s.Period)
		}
		description = displayDesc
		if description == "" {
			description = defaultENCatalogDescription
		}
		return title, description
	}

	title = displayTitle
	if title == "" {
		title = preview.Title
	}
	description = displayDesc
	if description == "" {
		description = preview.Description
	}
	return title, description
}

func formatENPeriodTitle(period float32) string {
	if period <= 0 {
		return ""
	}
	n := float64(period)
	rounded := math.Round(n*10) / 10
	if math.Abs(rounded-1) < 0.001 {
		return "1 month"
	}
	if math.Abs(rounded-math.Round(rounded)) < 0.001 {
		return fmt.Sprintf("%d months", int(math.Round(rounded)))
	}
	return fmt.Sprintf("%g months", rounded)
}
