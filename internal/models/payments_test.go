package models

import (
	"encoding/json"
	"testing"
)

func TestVisibleUserPays_AndCanceledZero(t *testing.T) {
	ev := func(e string) json.RawMessage {
		b, _ := json.Marshal(map[string]string{"event": e})
		return json.RawMessage(b)
	}

	tab := []struct {
		name string
		p    UserPay
		hide bool
	}{
		{name: "yookassa-canceled zero", p: UserPay{Money: 0, PaySystemID: "yookassa-canceled"}, hide: true},
		{name: "pay_system contains canceled zero", p: UserPay{Money: 0, PaySystemID: "foo-canceled-bar"}, hide: true},
		{name: "comment payment.canceled zero", p: UserPay{Money: 0, PaySystemID: "other", Comment: ev("payment.canceled")}, hide: true},
		{name: "money 150", p: UserPay{Money: 150}, hide: false},
		{name: "money negative real", p: UserPay{Money: -1318.74, PaySystemID: "yookassa-canceled"}, hide: false},
		{name: "money 0 not canceled", p: UserPay{Money: 0, PaySystemID: "yookassa_ok"}, hide: false},
		{name: "money 0 ambiguous comment", p: UserPay{Money: 0, PaySystemID: "x", Comment: json.RawMessage(`{}`)}, hide: false},
	}
	for _, tc := range tab {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			hidden := len(VisibleUserPays([]UserPay{tc.p})) == 0
			if hidden != tc.hide {
				t.Fatalf("hidden=%v want hide=%v", hidden, tc.hide)
			}
			if tc.hide != IsCanceledZeroPayment(tc.p) {
				t.Fatalf("IsCanceledZeroPayment inconsistent")
			}
		})
	}
}

func TestVisibleUserPays_PreservesOrder(t *testing.T) {
	a := UserPay{Money: 1, Date: "a"}
	b := UserPay{Money: 0, PaySystemID: "yookassa-canceled"}
	c := UserPay{Money: 2, Date: "c"}
	got := VisibleUserPays([]UserPay{b, c, a})
	if len(got) != 2 || got[0].Money != 2 || got[1].Money != 1 || got[0].Date != "c" || got[1].Date != "a" {
		t.Fatalf("%+v", got)
	}
}

func TestFormatRubAmount(t *testing.T) {
	tab := []struct {
		v    float64
		want string
	}{
		{150, "150 ₽"},
		{150.5, "150.50 ₽"},
		{-1318.74, "-1318.74 ₽"},
		{0, "0 ₽"},
	}
	for _, tc := range tab {
		if got := FormatRubAmount(tc.v); got != tc.want {
			t.Errorf("FormatRubAmount(%v)=%q want %q", tc.v, got, tc.want)
		}
	}
}
