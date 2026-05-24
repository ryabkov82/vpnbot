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
	for _, needle := range []string{
		`function openPaymentWindow`,
		`navigatePaymentWindow(`,
		`closePaymentWindow(`,
		`window.open('about:blank', '_blank')`,
		`win.opener = null`,
		`win.location.href = u`,
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("embed session missing payment helper %q", needle)
		}
	}
	if strings.Contains(raw, `window.open('', '_blank', 'noopener')`) {
		t.Fatal("embed: pre-open must not use noopener third argument")
	}
	if !strings.Contains(raw, `Вы вошли как ' + String(j.user.email`) {
		t.Fatal("embed user-line must show «Вы вошли как» email only")
	}
	if strings.Contains(raw, `j.user.login + ' · id '`) || strings.Contains(raw, "' · ' + j.user.login") {
		t.Fatal("embed must not concatenate login or user_id into user-line")
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
	if strings.Contains(raw, `(50–10 000 ₽, до 2 знаков)</label>`) {
		t.Fatal("embed: ambiguous topup amount label removed")
	}
	if !strings.Contains(raw, `50–10 000 ₽, до 2 знаков после запятой`) {
		t.Fatal("embed: topup label must clarify decimal places")
	}
	iER := strings.Index(raw, `id="topup-result" class="alert`)
	if iER < 0 {
		t.Fatal("embed topup-result missing")
	}
	jEmbModal := strings.Index(raw[iER:], `<div class="modal fade" id="topupModal"`)
	if jEmbModal < 0 {
		t.Fatal("embed topup modal missing")
	}
	erFrag := raw[iER : iER+jEmbModal]
	if strings.Contains(erFrag, `id="topup-pay-open"`) || strings.Contains(erFrag, `>Перейти к оплате<`) {
		t.Fatal("embed topup-result must not show post-success Перейти к оплате")
	}
	for _, needle := range []string{
		`Страница оплаты открыта в новой вкладке`,
		`обновите баланс. Баланс должен обновиться в течение 1–2 минут`,
		`Если страница оплаты не открылась автоматически`,
		`topup-result-pay-fallback`,
		`Обновить баланс`,
	} {
		if !strings.Contains(erFrag, needle) {
			t.Fatalf("embed topup-result missing %q", needle)
		}
	}
	iTS := strings.Index(raw, `getElementById('topup-submit').addEventListener`)
	if iTS < 0 {
		t.Fatal("embed topup submit handler missing")
	}
	tsSnip := raw[iTS:]
	jEmbedTopEnd := strings.Index(tsSnip, "\n\t\tfunction refreshAccountSnapshot")
	if jEmbedTopEnd > 0 {
		tsSnip = tsSnip[:jEmbedTopEnd]
	}
	if !strings.Contains(tsSnip, `openPaymentWindow()`) ||
		!strings.Contains(tsSnip, `navigatePaymentWindow(payWin, urlRaw)`) ||
		!strings.Contains(tsSnip, `closePaymentWindow(payWin)`) {
		t.Fatal("embed balance topup must use blank-window + navigatePaymentWindow pattern")
	}
	if strings.Contains(tsSnip, `getElementById('tab-balance-tab')`) {
		t.Fatal("embed topup success must not switch removed balance pill")
	}
	if !strings.Contains(tsSnip, `getElementById('balance-wrap')`) ||
		!strings.Contains(tsSnip, `scrollIntoView`) {
		t.Fatal("embed topup success must scroll the balance card after showing result")
	}
	iEmbTR := strings.Index(raw, `topupRefreshBtn.addEventListener`)
	if iEmbTR < 0 {
		t.Fatal("embed topup refresh btn bind missing")
	}
	embTRSnip := raw[iEmbTR:]
	if idx := strings.Index(embTRSnip, `document.querySelectorAll('.amt-quick')`); idx > 0 {
		embTRSnip = embTRSnip[:idx]
	}
	if !strings.Contains(embTRSnip, `refreshAccountSnapshot(tok)`) {
		t.Fatal("embed «Обновить баланс» must refresh via refreshAccountSnapshot(tok)")
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
	iBw := strings.Index(raw, `id="balance-wrap"`)
	if iBw < 0 {
		t.Fatal("embed balance-wrap missing")
	}
	iCt := strings.Index(raw, `id="cabinet-tabs"`)
	iPaneSvc := strings.Index(raw, `id="tab-pane-services"`)
	if iCt < 0 || iPaneSvc < 0 || !(iBw < iCt && iCt < iPaneSvc) {
		t.Fatal("embed balance-wrap must sit above tabs and outside tab panes")
	}
	if !bytes.Contains(b, []byte(`История платежей`)) {
		t.Fatal("payments tab heading missing")
	}
	if strings.Count(raw, `data-bs-toggle="pill"`) != 3 {
		t.Fatal("embed: cabinet must have three pills (services + buy + payments)")
	}
	for _, forbid := range []string{
		`id="tab-balance-tab"`,
		`id="tab-pane-balance"`,
		`aria-controls="tab-pane-balance"`,
		`data-bs-target="#tab-pane-balance"`,
		`getElementById('tab-balance-tab')`,
	} {
		if strings.Contains(raw, forbid) {
			t.Fatalf("embed must not retain balance tab %q", forbid)
		}
	}
	iOpenBalMod := strings.Index(raw, `function openBalanceTabWithTopupModal()`)
	if iOpenBalMod < 0 {
		t.Fatal("openBalanceTabWithTopupModal missing in embed")
	}
	openBalModSnip := raw[iOpenBalMod:]
	if k := strings.Index(openBalModSnip, `function renderServiceCards`); k > 0 {
		openBalModSnip = openBalModSnip[:k]
	}
	if strings.Contains(openBalModSnip, `tab-balance-tab`) ||
		strings.Contains(openBalModSnip, `bootstrap.Tab`) {
		t.Fatal("openBalanceTabWithTopupModal must only open modal, not switch pills")
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
	if !bytes.Contains(b, []byte(`Перейти к моим услугам`)) || !bytes.Contains(b, []byte(`js-card-goto-my-services`)) {
		t.Fatal("post-order must link to my services tab without full reload")
	}
	if strings.Contains(raw, "location.reload") {
		t.Fatal("session page must not use location.reload for refreshing services after purchase")
	}
	if !strings.Contains(raw, "refreshAccountSnapshot(tok).catch(function () {})") {
		t.Fatal("success order handler should refresh snapshot in background")
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
	if !strings.Contains(raw, "function resetCatalogOrderState") {
		t.Fatal("embedded session must define resetCatalogOrderState for catalog staleness fix")
	}
	if !strings.Contains(raw, "resetCatalogOrderState()") {
		t.Fatal("embedded session must call resetCatalogOrderState after delete refresh")
	}
	iDeleteAPI := strings.Index(raw, `'/api/account/service/delete'`)
	if iDeleteAPI < 0 {
		t.Fatal("delete endpoint string missing")
	}
	if !strings.Contains(raw[iDeleteAPI:], "refreshAccountSnapshot(tok).then(function () {") {
		t.Fatal("delete success must chain refreshAccountSnapshot before catalog reset")
	}
	iAwait := strings.Index(raw, `'Ожидает оплаты'`)
	if iAwait < 0 {
		t.Fatal("post-order button literal missing")
	}
	iScr := strings.Index(raw[iAwait:], "wrap.scrollIntoView")
	if iScr < 0 {
		t.Fatal("expected order-success scroll anchor")
	}
	if strings.Contains(raw[iAwait:iAwait+iScr], "resetCatalogOrderState") {
		t.Fatal("order success fragment must not call resetCatalogOrderState")
	}
	idxPayOk := strings.Index(raw, `"svc-pay-ok mt-2 d-none"`)
	if idxPayOk < 0 {
		t.Fatal("embed svc-pay-ok missing")
	}
	emPayOk := raw[idxPayOk:]
	if len(emPayOk) > 920 {
		emPayOk = emPayOk[:920]
	}
	if strings.Contains(emPayOk, `>Перейти к оплате`) {
		t.Fatal("embed svc-pay-ok must not include duplicate Перейти к оплате")
	}
	if !strings.Contains(emPayOk, `Страница оплаты открыта в новой вкладке`) ||
		!strings.Contains(emPayOk, `js-svc-pay-fallback`) ||
		!strings.Contains(emPayOk, `Открыть оплату`) {
		t.Fatal("embed svc-pay-ok copy/fallback mismatch")
	}
	if strings.Contains(raw, "js-svc-pay-open") {
		t.Fatal("embed must not retain js-svc-pay-open")
	}
	for _, needle := range []string{
		`btn-success js-svc-balance-pay`,
		`Перейти к оплате</button>`,
		`После оплаты баланс будет пополнен`,
		`Обновить услуги`,
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("embed NOT PAID markup missing %q", needle)
		}
	}
	idxEmbPay := strings.Index(raw, `var payBtn = cardRoot.querySelector('.js-svc-balance-pay')`)
	if idxEmbPay < 0 {
		t.Fatal("embed: NOT PAID pay handler anchor missing")
	}
	emSnip := raw[idxEmbPay:]
	if len(emSnip) > 4500 {
		emSnip = emSnip[:4500]
	}
	for _, needle := range []string{
		"fetch('/api/account/balance/topup'",
		"Готовим оплату...",
		`JSON.stringify({ token: baseTok, amount: amt })`,
		`navigatePaymentWindow(payWin, u)`,
		`closePaymentWindow(payWin)`,
	} {
		if !strings.Contains(emSnip, needle) {
			t.Fatalf("embed NOT PAID pay handler missing %q", needle)
		}
	}
	if strings.Contains(emSnip, "/api/account/service/order") {
		t.Fatal("embed NOT PAID strip must not call service/order path")
	}
	idxEmbCatBuy := strings.Index(raw, `buyBtn.addEventListener('click',`)
	if idxEmbCatBuy < 0 {
		t.Fatal("embed catalog buy handler anchor missing")
	}
	idxEmbCatFetch := strings.Index(raw[idxEmbCatBuy:], `fetch('/api/account/service/order',`)
	if idxEmbCatFetch < 0 {
		t.Fatal("embed catalog service/order fetch missing")
	}
	idxEmbCatFetch += idxEmbCatBuy
	embCatPreorder := raw[idxEmbCatBuy:idxEmbCatFetch]
	if strings.Contains(embCatPreorder, `openPaymentWindow`) {
		t.Fatal("embed catalog buy must not reference openPaymentWindow")
	}
	for _, needle := range []string{
		`id="logout-btn"`,
		`localStorage.removeItem(STORAGE)`,
		`'/account?logged_out=1'`,
		`if (!rawTok)`,
		`show('no-token', true)`,
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("embed session missing %q", needle)
		}
	}
	if strings.Count(raw, `id="user-line"`) != 1 {
		t.Fatal("embed session must have single #user-line")
	}
}
