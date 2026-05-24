package bot

import (
	"encoding/json"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestVisibleUserPaysForBot(t *testing.T) {
	ev := func(e string) json.RawMessage {
		b, _ := json.Marshal(map[string]string{"event": e})
		return json.RawMessage(b)
	}

	tab := []struct {
		name string
		p    models.UserPay
		hide bool
	}{
		{name: "yookassa-canceled zero", p: models.UserPay{Money: 0, PaySystemID: "yookassa-canceled"}, hide: true},
		{name: "pay_system contains canceled zero", p: models.UserPay{Money: 0, PaySystemID: "foo-canceled-bar"}, hide: true},
		{name: "comment payment.canceled zero", p: models.UserPay{Money: 0, PaySystemID: "other", Comment: ev("payment.canceled")}, hide: true},
		{name: "money 150", p: models.UserPay{Money: 150}, hide: false},
		{name: "money negative real", p: models.UserPay{Money: -1318.74, PaySystemID: "yookassa-canceled"}, hide: false},
		{name: "money 0 not canceled", p: models.UserPay{Money: 0, PaySystemID: "yookassa_ok"}, hide: false},
		{name: "money 0 ambiguous comment", p: models.UserPay{Money: 0, PaySystemID: "x", Comment: json.RawMessage(`{}`)}, hide: false},
	}
	for _, tc := range tab {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := visibleUserPaysForBot([]models.UserPay{tc.p})
			isHidden := len(got) == 0
			if isHidden != tc.hide {
				t.Fatalf("hidden=%v want hide=%v, got %+v", isHidden, tc.hide, got)
			}
		})
	}
}

func TestVisibleUserPaysForBot_PreservesOrder(t *testing.T) {
	a := models.UserPay{Money: 1, Date: "a"}
	b := models.UserPay{Money: 0, PaySystemID: "yookassa-canceled"}
	c := models.UserPay{Money: 2, Date: "c"}
	got := visibleUserPaysForBot([]models.UserPay{b, c, a})
	if len(got) != 2 || got[0].Money != 2 || got[1].Money != 1 || got[0].Date != "c" || got[1].Date != "a" {
		t.Fatalf("%+v", got)
	}
}

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
	visible := visibleUserPaysForBot(canceledOnly)
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
	vis := visibleUserPaysForBot(raw)
	if len(vis) != 1 || vis[0].Money != 42 {
		t.Fatal("filter sanity")
	}
	btnCount := len(vis)
	if btnCount != 1 {
		t.Fatalf("keyboard rows for pays should be %d (+ back added in handler)", btnCount)
	}
}
