package web

import (
	"bytes"
	"strings"
	"testing"
)

func TestAccountSessionEmbed_BalanceTopupAndHintsNoRenew(t *testing.T) {
	b := accountSessionPageHTML
	if !bytes.Contains(b, []byte("Баланс:")) {
		t.Fatal("balance label missing")
	}
	if !bytes.Contains(b, []byte("Пополнить баланс")) {
		t.Fatal("topup CTA missing")
	}
	if !bytes.Contains(b, []byte(`/api/account/balance/topup`)) {
		t.Fatal("topup endpoint missing")
	}
	if strings.Contains(string(b), "Продлить") {
		t.Fatal("renew button word must not appear")
	}
	if !bytes.Contains(b, []byte("автоматического продления")) {
		t.Fatal("balance explainer missing")
	}
	if !bytes.Contains(b, []byte("активирована автоматически")) {
		t.Fatal("NOT PAID hint missing")
	}
	if !bytes.Contains(b, []byte("Для автопродления заранее пополните баланс")) {
		t.Fatal("ACTIVE autopay hint missing")
	}
	if !bytes.Contains(b, []byte("Купить новую услугу")) {
		t.Fatal(`missing "Купить новую услугу" block`)
	}
	if !bytes.Contains(b, []byte("/api/account/catalog/services")) {
		t.Fatal("catalog endpoint missing")
	}
	if !bytes.Contains(b, []byte("/api/account/service/order")) {
		t.Fatal("service order endpoint missing")
	}
}
