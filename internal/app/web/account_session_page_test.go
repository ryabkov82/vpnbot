package web

import (
	"bytes"
	"strings"
	"testing"
)

func TestAccountSessionEmbed_BalanceTopupAndHintsNoRenew(t *testing.T) {
	b := accountSessionPageHTML
	raw := string(b)
	if strings.Contains(strings.ToUpper(raw), "SHM") {
		t.Fatal("embedded session must not show SHM to users")
	}
	if strings.Contains(raw, "Remnawave") || strings.Contains(raw, "check_exists_unpaid") {
		t.Fatal("session embed must not contain internal terminology")
	}
	if !bytes.Contains(b, []byte("Баланс:")) {
		t.Fatal("balance label missing")
	}
	if !bytes.Contains(b, []byte("Пополнить баланс")) {
		t.Fatal("topup CTA missing")
	}
	if !bytes.Contains(b, []byte(`/api/account/balance/topup`)) {
		t.Fatal("topup endpoint missing")
	}
	if strings.Contains(raw, "Продлить") {
		t.Fatal("renew button word must not appear")
	}
	if !bytes.Contains(b, []byte("автоматического продления")) {
		t.Fatal("balance explainer missing")
	}
	if !bytes.Contains(b, []byte("активирована автоматически")) {
		t.Fatal("NOT PAID hint missing")
	}
	if !bytes.Contains(b, []byte("Купить новую услугу")) {
		t.Fatal(`missing catalog section title`)
	}
	if !bytes.Contains(b, []byte(`/api/account/catalog/services`)) {
		t.Fatal("catalog endpoint missing")
	}
	if !bytes.Contains(b, []byte("/api/account/service/order")) {
		t.Fatal("service order endpoint missing")
	}
	if !bytes.Contains(b, []byte(`Мои услуги`)) {
		t.Fatal("services tab missing")
	}
	if !bytes.Contains(b, []byte(`Купить VPN`)) {
		t.Fatal("buy tab missing")
	}
	if !bytes.Contains(b, []byte(`Баланс</button>`)) || !bytes.Contains(b, []byte(`>Баланс<`)) {
		t.Fatal("balance tab pill missing")
	}
	if !bytes.Contains(b, []byte(`Создаем...`)) || !bytes.Contains(b, []byte(`Создаем услугу`)) {
		t.Fatal(`buy-flow loading strings missing`)
	}
	if !bytes.Contains(b, []byte(`spinner-border`)) {
		t.Fatal("spinner markup missing")
	}
	if strings.Contains(raw, `order-success-block`) {
		t.Fatal(`global order-success-block should not be used for purchase result`)
	}
	if !bytes.Contains(b, []byte(`js-catalog-success-text`)) {
		t.Fatal("per-card success paragraph hook missing")
	}
	if strings.Contains(raw, `Услуга создана или уже ожидает оплаты`) || strings.Contains(raw, `Услуга создана`) {
		t.Fatal(`must not use misleading "Услуга создана" copy in embed`)
	}
	if !bytes.Contains(b, []byte(`Услуга ожидает оплаты. Пополните баланс`)) ||
		!bytes.Contains(b, []byte(`Новая выбранная услуга не создана`)) {
		t.Fatal(`expected honest order success JS copy missing`)
	}
	if !bytes.Contains(b, []byte(`Ожидает оплаты`)) {
		t.Fatal("post-order button label missing")
	}
	if !bytes.Contains(b, []byte(`js-card-pay`)) {
		t.Fatal("per-card pay button missing")
	}
	if !strings.Contains(raw, "/api/account/service/delete") {
		t.Fatal("delete endpoint missing")
	}
	if !strings.Contains(raw, "Отменить услугу") {
		t.Fatal("cancel service button missing")
	}
	if !strings.Contains(raw, "!active") {
		t.Fatal("cancel controls must branch on !active (ACTIVE hides cancel)")
	}
	if !strings.Contains(raw, "Если хотите выбрать другой тариф") {
		t.Fatal("NOT PAID reschedule hint missing")
	}
	if !strings.Contains(raw, `Удалить услугу «`) {
		t.Fatal("delete confirm prompt missing")
	}
	if !strings.Contains(raw, "post-delete-buy-hint") {
		t.Fatal("post-delete buy tab hint missing")
	}
}