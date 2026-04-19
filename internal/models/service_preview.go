package models

import "strings"

const defaultServicePreviewDescription = "Подписка VPN на выбранный период."

// PreviewData — тексты и цена для экрана выбора услуги в боте.
type PreviewData struct {
	Title       string
	Description string
	Cost        float64
}

// BuildServicePreview собирает данные preview из ответа SHM (config.remnawave.bot, descr, name, cost).
func BuildServicePreview(s *Service) PreviewData {
	if s == nil {
		return PreviewData{Description: defaultServicePreviewDescription}
	}

	title := strings.TrimSpace(s.Name)
	if s.Config != nil {
		if t := strings.TrimSpace(s.Config.Remnawave.Bot.Title); t != "" {
			title = t
		}
	}

	description := ""
	if s.Config != nil {
		description = strings.TrimSpace(s.Config.Remnawave.Bot.Description)
	}
	if description == "" {
		description = strings.TrimSpace(s.Descr)
	}
	if description == "" {
		description = defaultServicePreviewDescription
	}

	return PreviewData{
		Title:       title,
		Description: description,
		Cost:        s.Cost,
	}
}
