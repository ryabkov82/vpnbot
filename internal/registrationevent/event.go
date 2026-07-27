package registrationevent

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/attribution"
)

const (
	Name          = "user_registered"
	SchemaVersion = 1
)

var (
	ErrBrandIDRequired = errors.New("registration event brand_id is required")
	ErrUserIDRequired  = errors.New("registration event user_id must be positive")
	ErrRecordInvalid   = errors.New("registration event attribution record is invalid")
)

// Event is the brand-neutral structured registration observability payload.
// It intentionally excludes marketing fields and PII; those live only in settings.attribution.
type Event struct {
	Event               string
	EventVersion        int
	AttributionVersion  int
	BrandID             string
	UserID              int
	RegistrationChannel attribution.RegistrationChannel
	RegistrationDomain  string
	CapturedAt          string
	Organic             bool
}

// New builds a validated Event from an already-persisted attribution Record.
func New(brandID string, userID int, record attribution.Record) (Event, error) {
	brandID = strings.TrimSpace(brandID)
	if brandID == "" {
		return Event{}, ErrBrandIDRequired
	}
	if userID <= 0 {
		return Event{}, ErrUserIDRequired
	}
	if !record.Valid() {
		return Event{}, ErrRecordInvalid
	}
	ft := record.FirstTouch
	return Event{
		Event:               Name,
		EventVersion:        SchemaVersion,
		AttributionVersion:  record.Version,
		BrandID:             brandID,
		UserID:              userID,
		RegistrationChannel: ft.RegistrationChannel,
		RegistrationDomain:  ft.RegistrationDomain,
		CapturedAt:          ft.CapturedAt,
		Organic:             record.IsOrganic(),
	}, nil
}

// Log writes Event as flat slog attributes. Nil logger uses slog.Default().
func Log(logger *slog.Logger, event Event) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info(
		"user registration",
		"event", event.Event,
		"event_version", event.EventVersion,
		"attribution_version", event.AttributionVersion,
		"brand_id", event.BrandID,
		"user_id", event.UserID,
		"registration_channel", string(event.RegistrationChannel),
		"registration_domain", event.RegistrationDomain,
		"captured_at", event.CapturedAt,
		"organic", event.Organic,
	)
}

// Emit validates and logs a registration event. Failures are returned to the caller
// and must not abort user creation.
func Emit(logger *slog.Logger, brandID string, userID int, record attribution.Record) error {
	ev, err := New(brandID, userID, record)
	if err != nil {
		return err
	}
	Log(logger, ev)
	return nil
}
