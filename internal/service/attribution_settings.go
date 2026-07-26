package service

import (
	"encoding/json"
	"errors"

	"github.com/ryabkov82/vpnbot/internal/attribution"
)

var (
	ErrAttributionRequired        = errors.New("attribution record is required")
	ErrAttributionExistingInvalid = errors.New("existing attribution record is invalid")
	ErrAttributionConflict        = errors.New("attribution first-touch already exists")
)

// mergeAttributionSettings adds settings["attribution"] write-once.
//
// Semantics:
//   - new record must be Valid() or ErrAttributionRequired;
//   - missing/null attribution → insert, changed=true;
//   - identical existing Valid record → changed=false;
//   - different existing Valid record → ErrAttributionConflict (no mutation);
//   - malformed/invalid existing → ErrAttributionExistingInvalid (no mutation).
//
// On success with changed=true, result is a shallow copy of settings with attribution set.
// On identical existing, result is the input map (or empty map if nil) unchanged.
// On error, result is nil and the input map is left unmodified.
func mergeAttributionSettings(
	settings map[string]interface{},
	record attribution.Record,
) (result map[string]interface{}, changed bool, err error) {
	if !record.Valid() {
		return nil, false, ErrAttributionRequired
	}

	if settings == nil {
		settings = map[string]interface{}{}
	}

	if existing, ok := settings["attribution"]; ok && existing != nil {
		existingRec, convErr := attributionRecordFromAny(existing)
		if convErr != nil || !existingRec.Valid() {
			return nil, false, ErrAttributionExistingInvalid
		}
		if attribution.Equal(existingRec, record) {
			return settings, false, nil
		}
		return nil, false, ErrAttributionConflict
	}

	attrObj, err := attributionRecordToMap(record)
	if err != nil {
		return nil, false, err
	}

	result = make(map[string]interface{}, len(settings)+1)
	for k, v := range settings {
		result[k] = v
	}
	result["attribution"] = attrObj
	return result, true, nil
}

func attributionRecordToMap(record attribution.Record) (map[string]interface{}, error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrAttributionRequired
	}
	return m, nil
}

func attributionRecordFromAny(v interface{}) (attribution.Record, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return attribution.Record{}, err
	}
	var rec attribution.Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return attribution.Record{}, err
	}
	return rec, nil
}
