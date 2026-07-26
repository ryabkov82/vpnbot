package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
)

func sampleAttributionRecord(t *testing.T) attribution.Record {
	t.Helper()
	rec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelTelegram,
		RegistrationDomain:  "connect.friends-connect.club",
		CapturedAt:          time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC),
	}, attribution.MarketingInput{
		TelegramStartParam: "summer",
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestMergeAttributionSettings_InsertMatrix(t *testing.T) {
	rec := sampleAttributionRecord(t)

	t.Run("nil_settings", func(t *testing.T) {
		out, changed, err := mergeAttributionSettings(nil, rec)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		if out["attribution"] == nil {
			t.Fatal("missing attribution")
		}
	})

	t.Run("empty_settings", func(t *testing.T) {
		in := map[string]interface{}{}
		out, changed, err := mergeAttributionSettings(in, rec)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		if _, ok := in["attribution"]; ok {
			t.Fatal("input map must not gain attribution (result is a copy)")
		}
		if out["attribution"] == nil {
			t.Fatal("missing attribution on result")
		}
	})

	t.Run("absent_key", func(t *testing.T) {
		in := map[string]interface{}{
			"brand_id":             "fc",
			"telegram":             map[string]interface{}{"chat_id": float64(123)},
			"web":                  map[string]interface{}{"email": "a@b.c"},
			"custom_unknown_field": map[string]interface{}{"keep": true},
		}
		out, changed, err := mergeAttributionSettings(in, rec)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		if out["brand_id"] != "fc" {
			t.Fatalf("brand_id=%v", out["brand_id"])
		}
		if out["custom_unknown_field"] == nil {
			t.Fatal("unknown sibling lost")
		}
		if out["telegram"] == nil || out["web"] == nil {
			t.Fatal("telegram/web lost")
		}
		got, err := attributionRecordFromAny(out["attribution"])
		if err != nil || !attribution.Equal(got, rec) {
			t.Fatalf("attribution=%+v err=%v", got, err)
		}
	})

	t.Run("null_attribution", func(t *testing.T) {
		in := map[string]interface{}{
			"brand_id":    "fc",
			"attribution": nil,
			"custom_keep": true,
		}
		out, changed, err := mergeAttributionSettings(in, rec)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		if out["custom_keep"] != true {
			t.Fatal("sibling lost")
		}
		got, err := attributionRecordFromAny(out["attribution"])
		if err != nil || !attribution.Equal(got, rec) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

func TestMergeAttributionSettings_IdempotentAndConflict(t *testing.T) {
	rec := sampleAttributionRecord(t)
	attrMap, err := attributionRecordToMap(rec)
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]interface{}{
		"brand_id":    "fc",
		"attribution": attrMap,
	}

	out, changed, err := mergeAttributionSettings(in, rec)
	if err != nil || changed {
		t.Fatalf("identical: changed=%v err=%v", changed, err)
	}
	if out["attribution"] == nil {
		t.Fatal("existing attribution missing")
	}

	diffChannel := rec
	diffChannel.FirstTouch.RegistrationChannel = attribution.RegistrationChannelWebGoogle
	if !diffChannel.Valid() {
		// domain/time still valid
	}
	snapshot, _ := json.Marshal(in["attribution"])
	_, changed, err = mergeAttributionSettings(in, diffChannel)
	if !errors.Is(err, ErrAttributionConflict) || changed {
		t.Fatalf("channel conflict: changed=%v err=%v", changed, err)
	}
	after, _ := json.Marshal(in["attribution"])
	if string(snapshot) != string(after) {
		t.Fatal("input attribution mutated on conflict")
	}

	diffUTM := rec
	diffUTM.FirstTouch.UTMCampaign = "other"
	_, changed, err = mergeAttributionSettings(in, diffUTM)
	if !errors.Is(err, ErrAttributionConflict) || changed {
		t.Fatalf("utm conflict: changed=%v err=%v", changed, err)
	}
}

func TestMergeAttributionSettings_MalformedExisting(t *testing.T) {
	rec := sampleAttributionRecord(t)
	cases := []struct {
		name string
		val  interface{}
	}{
		{name: "string", val: "not-an-object"},
		{name: "empty_object", val: map[string]interface{}{}},
		{name: "version_2", val: map[string]interface{}{
			"version": 2,
			"first_touch": map[string]interface{}{
				"registration_channel": "telegram",
				"registration_domain":  "connect.friends-connect.club",
				"captured_at":          "2026-07-26T18:00:00Z",
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := map[string]interface{}{
				"brand_id":    "fc",
				"attribution": tc.val,
			}
			before, _ := json.Marshal(in)
			_, changed, err := mergeAttributionSettings(in, rec)
			if !errors.Is(err, ErrAttributionExistingInvalid) || changed {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("input mutated on malformed existing")
			}
		})
	}
}

func TestMergeAttributionSettings_InvalidNewRecord(t *testing.T) {
	in := map[string]interface{}{"brand_id": "fc"}
	before, _ := json.Marshal(in)
	_, changed, err := mergeAttributionSettings(in, attribution.Record{})
	if !errors.Is(err, ErrAttributionRequired) || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("input mutated on invalid new record")
	}
}
