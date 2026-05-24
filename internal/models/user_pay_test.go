package models

import (
	"encoding/json"
	"testing"
)

func TestUserPayJSON_DecimalsAndComment(t *testing.T) {
	const raw = `{
	  "data": [
	    {
	      "id": 1,
	      "user_id": 99,
	      "date": "2025-06-01 12:00:00",
	      "money": -1318.74,
	      "pay_system_id": "yoomoney",
	      "uniq_key": "abc",
	      "comment": {"foo": "bar", "n": 1}
	    },
	    {
	      "id": 2,
	      "user_id": 99,
	      "date": "2025-06-02",
	      "money": 150,
	      "pay_system_id": "qr",
	      "uniq_key": "def"
	    }
	  ]
	}`

	var got struct {
		Data []UserPay `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Data) != 2 {
		t.Fatalf("len data=%d", len(got.Data))
	}
	a, b := got.Data[0], got.Data[1]
	if a.Money != -1318.74 || len(a.Comment) == 0 {
		t.Fatalf("pay0 money/comment %+v [%q]", a, string(a.Comment))
	}
	if b.Money != 150 {
		t.Fatalf("pay1 money=%v", b.Money)
	}
}

func TestUserPayJSON_EmptyData(t *testing.T) {
	var got struct {
		Data []UserPay `json:"data"`
	}
	if err := json.Unmarshal([]byte(`{"data":[]}`), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data) != 0 {
		t.Fatal("expected empty slice")
	}
}
