package models

import "strings"

// IsPremiumAntiBlockService определяет Premium/AntiBlock по catalog service.config.remnawave.internal_squad_name
// и premium_squad_name из конфигурации (та же семантика, что UserServiceTopConfigIsPremium).
func IsPremiumAntiBlockService(s *Service, premiumSquadName string) bool {
	if s == nil || strings.TrimSpace(premiumSquadName) == "" || s.Config == nil {
		return false
	}
	top := UserServiceTopConfig{
		Remnawave: UserServiceTopConfigRemnawave{
			InternalSquadName: s.Config.Remnawave.InternalSquadName,
		},
	}
	return UserServiceTopConfigIsPremium(top, premiumSquadName)
}
