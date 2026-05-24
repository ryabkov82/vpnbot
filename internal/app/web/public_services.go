package web

import (
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

// publicServicesApp — контракт для публичного списка тарифов (в т.ч. тестовый stub).
type publicServicesApp interface {
	GetServices() ([]models.Service, error)
}

type publicServiceJSON struct {
	ServiceID   int      `json:"service_id"`
	Name        string   `json:"name"`
	Cost        float64  `json:"cost"`
	Period      float64  `json:"period"`
	Description string   `json:"description"`
	Tier        string   `json:"tier"`
	ConnectApp  string   `json:"connect_app"`
	Badges      []string `json:"badges"`
}

type publicServicesListJSON struct {
	Services []publicServiceJSON `json:"services"`
}

// buildPublicServiceRowsFromList — публичные поля тарифов (BuildServicePreview), trial из cfg исключается.
func buildPublicServiceRowsFromList(cfg *config.Config, list []models.Service) []publicServiceJSON {
	trialID := 0
	if cfg != nil && cfg.Features.Trial.Enabled && cfg.Features.Trial.BaseServiceID > 0 {
		trialID = cfg.Features.Trial.BaseServiceID
	}

	out := make([]publicServiceJSON, 0, len(list))
	for i := range list {
		s := &list[i]
		if trialID > 0 && s.ServiceID == trialID {
			continue
		}
		preview := models.BuildServicePreview(s)
		name := strings.TrimSpace(preview.Title)
		if name == "" {
			name = "Тариф"
		}
		tier, conn, badges := tierConnectBadgesFromCatalog(cfg, s)
		if badges == nil {
			badges = []string{}
		}
		out = append(out, publicServiceJSON{
			ServiceID:   s.ServiceID,
			Name:        name,
			Cost:        preview.Cost,
			Period:      float64(s.Period),
			Description: preview.Description,
			Tier:        tier,
			ConnectApp:  conn,
			Badges:      badges,
		})
	}
	sortPublicTariffRowsPremiumLast(out)
	return out
}

// sortPublicTariffRowsPremiumLast: сначала standard (tier ≠ premium), затем premium.
// Внутри группы: period ↑, затем cost ↑, затем service_id ↑.
func sortPublicTariffRowsPremiumLast(rows []publicServiceJSON) {
	sort.SliceStable(rows, func(i, j int) bool {
		pi := isPublicTariffRowPremium(rows[i])
		pj := isPublicTariffRowPremium(rows[j])
		if pi != pj {
			return !pi && pj
		}
		a, b := rows[i], rows[j]
		if a.Period != b.Period {
			return a.Period < b.Period
		}
		if a.Cost != b.Cost {
			return a.Cost < b.Cost
		}
		return a.ServiceID < b.ServiceID
	})
}

func isPublicTariffRowPremium(row publicServiceJSON) bool {
	return strings.TrimSpace(row.Tier) == publicTierPremium
}

func servePublicServices(cfg *config.Config, app publicServicesApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/services" {
			http.NotFound(w, r)
			return
		}

		log.Printf("api/public/services: %s %s", r.Method, r.URL.Path)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		list, err := app.GetServices()
		if err != nil {
			log.Printf("api/public/services GetServices: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "services_unavailable")
			return
		}

		out := buildPublicServiceRowsFromList(cfg, list)

		writeJSON(w, http.StatusOK, publicServicesListJSON{Services: out})
	}
}
