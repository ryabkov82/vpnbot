package web

import (
	"fmt"
	"math"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func servicePricingFields(s *models.Service) (publicCode string, intlEnabled bool, intlCurrency string, intlCents int64) {
	if s == nil || s.Config == nil {
		return "", false, "", 0
	}
	p := s.Config.Pricing
	return strings.TrimSpace(p.PublicCode), p.InternationalEnabled, strings.TrimSpace(p.InternationalCurrency), p.InternationalAmountCents
}

func serviceInternationalUSDCents(s *models.Service) (int64, bool) {
	_, enabled, currency, cents := servicePricingFields(s)
	if !enabled || cents <= 0 {
		return 0, false
	}
	if strings.EqualFold(currency, "USD") {
		return cents, true
	}
	return 0, false
}

func formatUSDDollars(d float64) string {
	if d < 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return ""
	}
	rounded := math.Round(d*100) / 100
	if math.Abs(rounded-math.Round(rounded)) < 0.005 {
		return fmt.Sprintf("$%.0f", rounded)
	}
	return fmt.Sprintf("$%.2f", rounded)
}

func formatUSDFromCents(cents int64) string {
	if cents <= 0 {
		return ""
	}
	return formatUSDDollars(float64(cents) / 100.0)
}

func formatUSDMonthlyFromCents(totalCents int64, periodMonths float64) string {
	if totalCents <= 0 || periodMonths <= 0 {
		return ""
	}
	monthly := float64(totalCents) / 100.0 / periodMonths
	text := formatUSDDollars(monthly)
	if text == "" {
		return ""
	}
	return text + "/mo"
}

func formatRubCatalogDisplay(cost float64, locale accountLocale) string {
	if cost <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return ""
	}
	if locale == accountLocaleEN {
		if math.Abs(cost-math.Round(cost)) < 0.005 {
			return fmt.Sprintf("%.0f RUB", cost)
		}
		return fmt.Sprintf("%.2f RUB", cost)
	}
	return models.FormatRubAmount(cost)
}

func applyPublicServicePricing(row *publicServiceJSON, s *models.Service, cost float64, locale accountLocale) {
	publicCode, intlEnabled, intlCurrency, intlCents := servicePricingFields(s)

	row.ActualCurrency = "RUB"
	row.ActualAmount = cost
	if publicCode != "" {
		row.PublicCode = publicCode
	}
	row.InternationalEnabled = intlEnabled
	if intlCurrency != "" {
		row.InternationalCurrency = intlCurrency
	}
	if intlCents > 0 {
		row.InternationalAmountCents = intlCents
	}

	if locale == accountLocaleEN {
		if usdCents, ok := serviceInternationalUSDCents(s); ok {
			row.DisplayCurrency = "USD"
			row.DisplayAmountText = formatUSDFromCents(usdCents)
			row.DisplayMonthlyText = formatUSDMonthlyFromCents(usdCents, row.Period)
			return
		}
	}

	row.DisplayCurrency = "RUB"
	row.DisplayAmountText = formatRubCatalogDisplay(cost, locale)
}
