// Package attribution defines the acquisition first-touch domain model for M8.
//
// Attribution records describe immutable marketing first-touch data captured at
// SHM user creation. They are not used for authentication, brand membership,
// session decisions, or payment routing. Authoritative brand identity remains
// settings.brand_id outside this package.
//
// A missing Record (absent settings.attribution block) means unknown acquisition
// source. Malformed optional marketing input is normalized or dropped and must
// never fail registration; only invalid server-derived fields return errors.
package attribution

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// SchemaVersion is the persisted attribution JSON schema version.
const SchemaVersion = 1

// Marketing field Unicode rune limits (truncate; do not fail registration).
const (
	MaxLandingPathRunes        = 512
	MaxReferrerHostRunes       = 253
	MaxUTMSourceRunes          = 100
	MaxUTMMediumRunes          = 100
	MaxUTMCampaignRunes        = 200
	MaxUTMContentRunes         = 200
	MaxUTMTermRunes            = 200
	MaxTelegramStartParamRunes = 128
)

const maxRegistrationDomainBytes = 253

// RegistrationChannel is the server-derived mechanism that created the SHM user.
type RegistrationChannel string

const (
	RegistrationChannelTelegram     RegistrationChannel = "telegram"
	RegistrationChannelWebMagicLink RegistrationChannel = "web_magic_link"
	RegistrationChannelWebGoogle    RegistrationChannel = "web_google"
)

// Valid reports whether c is an approved user-creation channel.
func (c RegistrationChannel) Valid() bool {
	switch c {
	case RegistrationChannelTelegram, RegistrationChannelWebMagicLink, RegistrationChannelWebGoogle:
		return true
	default:
		return false
	}
}

// Record is the persisted settings.attribution payload (versioned).
type Record struct {
	Version    int        `json:"version"`
	FirstTouch FirstTouch `json:"first_touch"`
}

// FirstTouch is the immutable acquisition snapshot written once at user create.
// brand_id is intentionally omitted; settings.brand_id remains authoritative.
type FirstTouch struct {
	RegistrationChannel RegistrationChannel `json:"registration_channel"`
	RegistrationDomain  string              `json:"registration_domain"`
	LandingPath         string              `json:"landing_path,omitempty"`
	ReferrerHost        string              `json:"referrer_host,omitempty"`
	UTMSource           string              `json:"utm_source,omitempty"`
	UTMMedium           string              `json:"utm_medium,omitempty"`
	UTMCampaign         string              `json:"utm_campaign,omitempty"`
	UTMContent          string              `json:"utm_content,omitempty"`
	UTMTerm             string              `json:"utm_term,omitempty"`
	TelegramStartParam  string              `json:"telegram_start_param,omitempty"`
	CapturedAt          string              `json:"captured_at"`
}

// ServerContext holds server-derived fields. Invalid values fail NewFirstTouch.
type ServerContext struct {
	RegistrationChannel RegistrationChannel
	RegistrationDomain  string
	CapturedAt          time.Time
}

// MarketingInput holds untrusted client/Telegram marketing fields.
// Invalid values are normalized or dropped; they never return an error.
type MarketingInput struct {
	LandingPath        string
	Referrer           string
	UTMSource          string
	UTMMedium          string
	UTMCampaign        string
	UTMContent         string
	UTMTerm            string
	TelegramStartParam string
}

var (
	errChannelRequired = errors.New("attribution: registration channel is required")
	errChannelInvalid  = errors.New("attribution: unsupported registration channel")
	errDomainRequired  = errors.New("attribution: registration domain is required")
	errDomainInvalid   = errors.New("attribution: registration domain is invalid")
	errCapturedAtZero  = errors.New("attribution: captured_at is required")
)

// NewFirstTouch builds a versioned Record from server context and marketing input.
// Only server-derived validation failures return an error.
func NewFirstTouch(server ServerContext, marketing MarketingInput) (Record, error) {
	if strings.TrimSpace(string(server.RegistrationChannel)) == "" {
		return Record{}, errChannelRequired
	}
	if !server.RegistrationChannel.Valid() {
		return Record{}, fmt.Errorf("%w %q", errChannelInvalid, server.RegistrationChannel)
	}
	domain, err := normalizeRegistrationDomain(server.RegistrationDomain)
	if err != nil {
		return Record{}, err
	}
	if server.CapturedAt.IsZero() {
		return Record{}, errCapturedAtZero
	}
	captured := server.CapturedAt.UTC().Truncate(time.Second).Format(time.RFC3339)

	ft := FirstTouch{
		RegistrationChannel: server.RegistrationChannel,
		RegistrationDomain:  domain,
		LandingPath:         normalizeLandingPath(marketing.LandingPath),
		ReferrerHost:        normalizeReferrerHost(marketing.Referrer),
		UTMSource:           sanitizeMarketingText(marketing.UTMSource, MaxUTMSourceRunes),
		UTMMedium:           sanitizeMarketingText(marketing.UTMMedium, MaxUTMMediumRunes),
		UTMCampaign:         sanitizeMarketingText(marketing.UTMCampaign, MaxUTMCampaignRunes),
		UTMContent:          sanitizeMarketingText(marketing.UTMContent, MaxUTMContentRunes),
		UTMTerm:             sanitizeMarketingText(marketing.UTMTerm, MaxUTMTermRunes),
		TelegramStartParam:  sanitizeMarketingText(marketing.TelegramStartParam, MaxTelegramStartParamRunes),
		CapturedAt:          captured,
	}
	return Record{Version: SchemaVersion, FirstTouch: ft}, nil
}

// Valid reports whether r satisfies the persisted first-touch contract.
func (r Record) Valid() bool {
	if r.Version != SchemaVersion {
		return false
	}
	ft := r.FirstTouch
	if !ft.RegistrationChannel.Valid() {
		return false
	}
	dom, err := normalizeRegistrationDomain(ft.RegistrationDomain)
	if err != nil || dom != ft.RegistrationDomain {
		return false
	}
	captured, err := time.Parse(time.RFC3339, ft.CapturedAt)
	if err != nil || captured.UTC().Format(time.RFC3339) != ft.CapturedAt {
		return false
	}
	if ft.LandingPath != "" {
		if !validPersistedLandingPath(ft.LandingPath) {
			return false
		}
	}
	if ft.ReferrerHost != "" {
		if _, err := normalizeRegistrationDomain(ft.ReferrerHost); err != nil {
			return false
		}
		if utf8.RuneCountInString(ft.ReferrerHost) > MaxReferrerHostRunes {
			return false
		}
	}
	if !utmWithinLimit(ft.UTMSource, MaxUTMSourceRunes) ||
		!utmWithinLimit(ft.UTMMedium, MaxUTMMediumRunes) ||
		!utmWithinLimit(ft.UTMCampaign, MaxUTMCampaignRunes) ||
		!utmWithinLimit(ft.UTMContent, MaxUTMContentRunes) ||
		!utmWithinLimit(ft.UTMTerm, MaxUTMTermRunes) ||
		!utmWithinLimit(ft.TelegramStartParam, MaxTelegramStartParamRunes) {
		return false
	}
	if containsControlRunes(ft.UTMSource) || containsControlRunes(ft.UTMMedium) ||
		containsControlRunes(ft.UTMCampaign) || containsControlRunes(ft.UTMContent) ||
		containsControlRunes(ft.UTMTerm) || containsControlRunes(ft.TelegramStartParam) ||
		containsControlRunes(ft.LandingPath) || containsControlRunes(ft.ReferrerHost) {
		return false
	}
	return true
}

// IsOrganic reports a valid first-touch with no UTM and no Telegram start payload.
// ReferrerHost alone does not disqualify organic. Absence of a Record is unknown
// and is decided by storage callers, not by this method.
func (r Record) IsOrganic() bool {
	if !r.Valid() {
		return false
	}
	ft := r.FirstTouch
	return ft.UTMSource == "" && ft.UTMMedium == "" && ft.UTMCampaign == "" &&
		ft.UTMContent == "" && ft.UTMTerm == "" && ft.TelegramStartParam == ""
}

// Equal reports whether two records are byte-for-byte equal on all fields.
func Equal(a, b Record) bool {
	return a == b
}

func normalizeRegistrationDomain(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errDomainRequired
	}
	if strings.Contains(raw, "://") || strings.ContainsAny(raw, "/?#@") {
		return "", errDomainInvalid
	}
	// Port or IPv6 brackets → reject (hostname only, no port).
	if strings.Contains(raw, ":") {
		return "", errDomainInvalid
	}
	raw = strings.ToLower(raw)
	if strings.HasSuffix(raw, ".") {
		raw = strings.TrimSuffix(raw, ".")
		if raw == "" || strings.HasSuffix(raw, ".") {
			return "", errDomainInvalid
		}
	}
	if len(raw) > maxRegistrationDomainBytes {
		return "", errDomainInvalid
	}
	if net.ParseIP(raw) != nil {
		return "", errDomainInvalid
	}
	if raw == "localhost" {
		return raw, nil
	}
	if !validDNSHostname(raw) {
		return "", errDomainInvalid
	}
	return raw, nil
}

func validDNSHostname(host string) bool {
	if host == "" || len(host) > maxRegistrationDomainBytes {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 1 {
		return false
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			isALNum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
			if i == 0 || i == len(label)-1 {
				if !isALNum {
					return false
				}
				continue
			}
			if !isALNum && c != '-' {
				return false
			}
		}
	}
	return true
}

func normalizeLandingPath(raw string) string {
	raw = sanitizeMarketingText(raw, MaxLandingPathRunes)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.User != nil {
		return ""
	}
	// Prefer escaped path so % sequences are not re-decoded for storage.
	path := u.EscapedPath()
	if path == "" {
		path = u.Path
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return ""
	}
	if strings.ContainsAny(path, "?#") {
		return ""
	}
	return truncateRunes(path, MaxLandingPathRunes)
}

func normalizeReferrerHost(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = stripControlRunes(raw)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return ""
	}
	if net.ParseIP(host) != nil {
		return ""
	}
	host = strings.TrimSuffix(host, ".")
	if host == "localhost" {
		return truncateRunes(host, MaxReferrerHostRunes)
	}
	if !validDNSHostname(host) {
		return ""
	}
	return truncateRunes(host, MaxReferrerHostRunes)
}

func sanitizeMarketingText(raw string, maxRunes int) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ToValidUTF8(raw, "")
	raw = stripControlRunes(raw)
	raw = strings.TrimSpace(raw)
	return truncateRunes(raw, maxRunes)
}

func stripControlRunes(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isControlRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isControlRune(r rune) bool {
	if r < 0x20 || r == 0x7F {
		return true
	}
	return r >= 0x80 && r <= 0x9F
}

func containsControlRunes(s string) bool {
	for _, r := range s {
		if isControlRune(r) {
			return true
		}
	}
	return false
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

func utmWithinLimit(s string, maxRunes int) bool {
	return utf8.RuneCountInString(s) <= maxRunes
}

func validPersistedLandingPath(path string) bool {
	if path == "" || !strings.HasPrefix(path, "/") {
		return false
	}
	if utf8.RuneCountInString(path) > MaxLandingPathRunes {
		return false
	}
	if strings.ContainsAny(path, "?#") || strings.Contains(path, "://") {
		return false
	}
	u, err := url.Parse(path)
	if err != nil || u.Scheme != "" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return true
}
