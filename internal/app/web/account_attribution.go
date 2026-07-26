package web

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/config"
)

var errAttributionPublicBaseURL = errors.New("attribution registration domain requires public_base_url")

// buildWebMagicLinkAttribution builds a first-touch Record for email magic-link signup.
// registration_domain is always derived from cfg.PublicBaseURL(), never from the request.
func buildWebMagicLinkAttribution(
	cfg *config.Config,
	req accountLoginStartRequestJSON,
	capturedAt time.Time,
) (attribution.Record, error) {
	domain, err := registrationDomainFromConfig(cfg)
	if err != nil {
		return attribution.Record{}, err
	}
	return attribution.NewFirstTouch(
		attribution.ServerContext{
			RegistrationChannel: attribution.RegistrationChannelWebMagicLink,
			RegistrationDomain:  domain,
			CapturedAt:          capturedAt,
		},
		attribution.MarketingInput{
			LandingPath: req.LandingPath,
			Referrer:    req.Referrer,
			UTMSource:   req.UTMSource,
			UTMMedium:   req.UTMMedium,
			UTMCampaign: req.UTMCampaign,
			UTMContent:  req.UTMContent,
			UTMTerm:     req.UTMTerm,
		},
	)
}

// buildWebGoogleAttribution builds a first-touch Record for Google OAuth registration.
// registration_domain is always derived from cfg.PublicBaseURL(), never from the request.
func buildWebGoogleAttribution(
	cfg *config.Config,
	marketing attribution.MarketingInput,
	capturedAt time.Time,
) (attribution.Record, error) {
	domain, err := registrationDomainFromConfig(cfg)
	if err != nil {
		return attribution.Record{}, err
	}
	return attribution.NewFirstTouch(
		attribution.ServerContext{
			RegistrationChannel: attribution.RegistrationChannelWebGoogle,
			RegistrationDomain:  domain,
			CapturedAt:          capturedAt,
		},
		marketing,
	)
}

func registrationDomainFromConfig(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", errAttributionPublicBaseURL
	}
	base := strings.TrimSpace(cfg.PublicBaseURL())
	if base == "" {
		return "", errAttributionPublicBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u == nil {
		return "", errAttributionPublicBaseURL
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", errAttributionPublicBaseURL
	}
	return host, nil
}
