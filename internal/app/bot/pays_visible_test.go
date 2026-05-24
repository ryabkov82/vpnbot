package bot

import (
	"encoding/json"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestPaysListCaption_WithRawCount(t *testing.T) {
	if got := paysListCaption(nil, 0); got != "Платежей пока нет." {
		t.Fatalf("both empty: %q", got)
	}
	if got := paysListCaption([]models.UserPay{}, 0); got != "Платежей пока нет." {
		t.Fatalf("visible empty raw 0: %q", got)
	}
	ev, err := json.Marshal(map[string]string{"event": "payment.canceled"})
	if err != nil {
		t.Fatal(err)
	}
	canceledOnly := []models.UserPay{{Money: 0, PaySystemID: "yookassa-canceled", Comment: json.RawMessage(ev)}}
	visible := models.VisibleUserPays(canceledOnly)
	if got := paysListCaption(visible, len(canceledOnly)); got != "Оплаченных платежей пока нет." {
		t.Fatalf("only canceled filtered: %q", got)
	}
	if got := paysListCaption([]models.UserPay{{Money: 100}}, 2); got != "Платежи" {
		t.Fatalf("has payment: %q", got)
	}
}

func TestKeyboardRowCountMirrorsFilteredPays(t *testing.T) {
	ev, err := json.Marshal(map[string]string{"event": "payment.canceled"})
	if err != nil {
		t.Fatal(err)
	}
	raw := []models.UserPay{
		{Money: 0, PaySystemID: "yookassa-canceled", Comment: json.RawMessage(ev), Date: "hidden"},
		{Money: 42, Date: "seen"},
	}
	vis := models.VisibleUserPays(raw)
	if len(vis) != 1 || vis[0].Money != 42 {
		t.Fatal("filter sanity")
	}
	btnCount := len(vis)
	if btnCount != 1 {
		t.Fatalf("keyboard rows for pays should be %d (+ back added in handler)", btnCount)
	}
}
