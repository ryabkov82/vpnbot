package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/config"
)

const (
	googleOAuthStatePrefix = "g1."
	googleOAuthStateTyp    = "google_oauth_state"

	googleOAuthModeLogin = "login"
	googleOAuthModeLink  = "link"

	googleOAuthMaxStateBytes = 3800
)

// googleOAuthMaxStateBytesOverride, when > 0, replaces the size limit (tests only).
var googleOAuthMaxStateBytesOverride int

var ErrGoogleOAuthState = errors.New("invalid google oauth state")

func googleOAuthStateSizeLimit() int {
	if googleOAuthMaxStateBytesOverride > 0 {
		return googleOAuthMaxStateBytesOverride
	}
	return googleOAuthMaxStateBytes
}

type googleOAuthStateClaims struct {
	Typ         string              `json:"typ"`
	BrandID     string              `json:"brand_id"`
	Mode        string              `json:"mode"`
	Nonce       string              `json:"nonce"`
	Attribution *attribution.Record `json:"attribution,omitempty"`
	Exp         int64               `json:"exp"`
}

func googleOAuthStateTTL() time.Duration {
	return time.Duration(googleOAuthCookieMaxAgeSecs) * time.Second
}

func newGoogleOAuthNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isLegacyGoogleOAuthState(state string) bool {
	state = strings.TrimSpace(state)
	return state != "" && !strings.HasPrefix(state, googleOAuthStatePrefix)
}

func createGoogleOAuthLoginState(secret, brandID string, record attribution.Record, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrAccountTokenEmptySecret
	}
	brandID, err := requireAccountTokenBrandID(brandID)
	if err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", ErrGoogleOAuthState
	}
	if !record.Valid() || record.FirstTouch.RegistrationChannel != attribution.RegistrationChannelWebGoogle {
		return "", ErrGoogleOAuthState
	}
	nonce, err := newGoogleOAuthNonce()
	if err != nil {
		return "", err
	}
	recCopy := record
	payload := googleOAuthStateClaims{
		Typ:         googleOAuthStateTyp,
		BrandID:     brandID,
		Mode:        googleOAuthModeLogin,
		Nonce:       nonce,
		Attribution: &recCopy,
		Exp:         time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	signed, err := signAndEncodeAccountPayload(secret, payloadJSON)
	if err != nil {
		return "", err
	}
	return googleOAuthStatePrefix + signed, nil
}

func createGoogleOAuthLinkState(secret, brandID string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrAccountTokenEmptySecret
	}
	brandID, err := requireAccountTokenBrandID(brandID)
	if err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", ErrGoogleOAuthState
	}
	nonce, err := newGoogleOAuthNonce()
	if err != nil {
		return "", err
	}
	payload := googleOAuthStateClaims{
		Typ:     googleOAuthStateTyp,
		BrandID: brandID,
		Mode:    googleOAuthModeLink,
		Nonce:   nonce,
		Exp:     time.Now().Add(ttl).Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	signed, err := signAndEncodeAccountPayload(secret, payloadJSON)
	if err != nil {
		return "", err
	}
	return googleOAuthStatePrefix + signed, nil
}

// createGoogleOAuthLoginStateForStart builds a size-bounded login state.
// Oversized optional marketing falls back to a server-derived organic record.
func createGoogleOAuthLoginStateForStart(
	cfg *config.Config,
	marketing attribution.MarketingInput,
	capturedAt time.Time,
) (state string, record attribution.Record, err error) {
	if cfg == nil {
		return "", attribution.Record{}, ErrGoogleOAuthState
	}
	secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
	brandID := cfgBrandID(cfg)
	ttl := googleOAuthStateTTL()

	rec, err := buildWebGoogleAttribution(cfg, marketing, capturedAt)
	if err != nil {
		return "", attribution.Record{}, err
	}
	state, err = createGoogleOAuthLoginState(secret, brandID, rec, ttl)
	if err != nil {
		return "", attribution.Record{}, err
	}
	limit := googleOAuthStateSizeLimit()
	if len(state) <= limit {
		return state, rec, nil
	}

	organic, err := buildWebGoogleAttribution(cfg, attribution.MarketingInput{}, capturedAt)
	if err != nil {
		return "", attribution.Record{}, err
	}
	state, err = createGoogleOAuthLoginState(secret, brandID, organic, ttl)
	if err != nil {
		return "", attribution.Record{}, err
	}
	if len(state) > limit {
		return "", attribution.Record{}, ErrGoogleOAuthState
	}
	return state, organic, nil
}

func parseAndVerifyGoogleOAuthState(secret, expectedBrandID, state string) (*googleOAuthStateClaims, error) {
	state = strings.TrimSpace(state)
	if state == "" || !strings.HasPrefix(state, googleOAuthStatePrefix) {
		return nil, ErrGoogleOAuthState
	}
	raw := strings.TrimPrefix(state, googleOAuthStatePrefix)
	payloadJSON, err := verifyAccountMagicTokenPayload(secret, raw)
	if err != nil {
		if errors.Is(err, ErrAccountTokenEmptySecret) {
			return nil, err
		}
		return nil, ErrGoogleOAuthState
	}
	var claims googleOAuthStateClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrGoogleOAuthState
	}
	if claims.Typ != googleOAuthStateTyp {
		return nil, ErrGoogleOAuthState
	}
	if err := matchAccountTokenBrand(claims.BrandID, expectedBrandID); err != nil {
		return nil, ErrGoogleOAuthState
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, ErrGoogleOAuthState
	}
	if strings.TrimSpace(claims.Nonce) == "" {
		return nil, ErrGoogleOAuthState
	}
	switch claims.Mode {
	case googleOAuthModeLogin:
		if claims.Attribution == nil || !claims.Attribution.Valid() {
			return nil, ErrGoogleOAuthState
		}
		if claims.Attribution.FirstTouch.RegistrationChannel != attribution.RegistrationChannelWebGoogle {
			return nil, ErrGoogleOAuthState
		}
	case googleOAuthModeLink:
		if claims.Attribution != nil {
			return nil, ErrGoogleOAuthState
		}
	default:
		return nil, ErrGoogleOAuthState
	}
	return &claims, nil
}
