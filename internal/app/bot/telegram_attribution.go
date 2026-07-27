package bot

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/config"
)

const telegramAttributionPendingTTL = 24 * time.Hour

var errTelegramAttributionPublicBaseURL = errors.New("telegram attribution registration domain requires public_base_url")

type pendingTelegramAttribution struct {
	Record    attribution.Record
	ExpiresAt time.Time
}

func (s *Service) initTelegramAttributionPending() {
	if s.telegramAttributionPending == nil {
		s.telegramAttributionPending = make(map[int64]pendingTelegramAttribution)
	}
}

// buildTelegramRegistrationAttribution builds first-touch Record for Telegram bot registration.
// registration_domain is always derived from cfg.PublicBaseURL(), never from Telegram payload.
func buildTelegramRegistrationAttribution(
	cfg *config.Config,
	startParam string,
	capturedAt time.Time,
) (attribution.Record, error) {
	domain, err := telegramRegistrationDomainFromConfig(cfg)
	if err != nil {
		return attribution.Record{}, err
	}
	return attribution.NewFirstTouch(
		attribution.ServerContext{
			RegistrationChannel: attribution.RegistrationChannelTelegram,
			RegistrationDomain:  domain,
			CapturedAt:          capturedAt,
		},
		attribution.MarketingInput{
			TelegramStartParam: startParam,
		},
	)
}

func telegramRegistrationDomainFromConfig(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", errTelegramAttributionPublicBaseURL
	}
	base := strings.TrimSpace(cfg.PublicBaseURL())
	if base == "" {
		return "", errTelegramAttributionPublicBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u == nil {
		return "", errTelegramAttributionPublicBaseURL
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", errTelegramAttributionPublicBaseURL
	}
	return host, nil
}

// rememberTelegramAttribution stores first-touch pending attribution.
// Non-expired existing entries are never overwritten.
func (s *Service) rememberTelegramAttribution(chatID int64, rec attribution.Record, now time.Time) {
	if chatID <= 0 || !rec.Valid() {
		return
	}
	s.telegramAttributionMu.Lock()
	defer s.telegramAttributionMu.Unlock()
	s.initTelegramAttributionPending()
	if existing, ok := s.telegramAttributionPending[chatID]; ok && existing.ExpiresAt.After(now) {
		return
	}
	s.telegramAttributionPending[chatID] = pendingTelegramAttribution{
		Record:    rec,
		ExpiresAt: now.Add(telegramAttributionPendingTTL),
	}
}

// peekTelegramAttribution returns a non-expired pending record without removing it.
func (s *Service) peekTelegramAttribution(chatID int64, now time.Time) (attribution.Record, bool) {
	s.telegramAttributionMu.Lock()
	defer s.telegramAttributionMu.Unlock()
	if s.telegramAttributionPending == nil {
		return attribution.Record{}, false
	}
	existing, ok := s.telegramAttributionPending[chatID]
	if !ok {
		return attribution.Record{}, false
	}
	if !existing.ExpiresAt.After(now) {
		delete(s.telegramAttributionPending, chatID)
		return attribution.Record{}, false
	}
	return existing.Record, true
}

// clearTelegramAttribution removes pending attribution for chatID.
func (s *Service) clearTelegramAttribution(chatID int64) {
	s.telegramAttributionMu.Lock()
	defer s.telegramAttributionMu.Unlock()
	if s.telegramAttributionPending == nil {
		return
	}
	delete(s.telegramAttributionPending, chatID)
}
