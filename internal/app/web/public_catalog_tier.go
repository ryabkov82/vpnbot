package web

import (
	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
)

const (
	publicTierStandard = "standard"
	publicTierPremium  = "premium"
	publicConnectSubscription = "subscription"
	publicConnectHapp         = "happ"
)

// premiumHappBadges — клиентские бейджи (без внутренних названий платформы).
var premiumHappBadges = []string{"Premium", "AntiBlock", "Happ"}

func clonePremiumBadges() []string {
	return append([]string(nil), premiumHappBadges...)
}

// tierConnectBadgesFromCatalog услуги из каталога (SHM service).
func tierConnectBadgesFromCatalog(cfg *config.Config, s *models.Service) (tier, connectApp string, badges []string) {
	if cfg != nil && models.IsPremiumAntiBlockService(s, cfg.PremiumSquadName) {
		return publicTierPremium, publicConnectHapp, clonePremiumBadges()
	}
	return publicTierStandard, publicConnectSubscription, nil
}

func tierConnectBadgesFromUserService(cfg *config.Config, us *models.UserService) (tier, connectApp string, badges []string) {
	if cfg != nil && models.IsPremiumAntiBlockUserService(us, cfg.PremiumSquadName) {
		return publicTierPremium, publicConnectHapp, clonePremiumBadges()
	}
	return publicTierStandard, publicConnectSubscription, nil
}
