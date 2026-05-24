package web

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sessionHTMLPath(t *testing.T) string {
	t.Helper()
	_, fname, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(fname), "static", "account", "session.html")
}

func TestAccountSessionStaticContainsPremiumHappCopy(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatalf("read session.html: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "Для Premium используйте приложение Happ.") {
		t.Fatal("missing premium Happ copy")
	}
	if !strings.Contains(s, "connect_app") || !strings.Contains(s, "'happ'") {
		t.Fatal("session JS must branch on connect_app")
	}
	if !strings.Contains(s, "Подключить Premium") {
		t.Fatal(`missing Premium connect button label`)
	}
	if !strings.Contains(s, "/api/account/session/start") {
		t.Fatal("session must call /api/account/session/start")
	}
	if !strings.Contains(s, "'/api/account/services?token='") {
		t.Fatal("session must fetch /api/account/services with exchanged token")
	}
	if !strings.Contains(s, "function bootFromRawToken") {
		t.Fatal("expected bootFromRawToken bootstrap")
	}
	if !strings.Contains(s, "Ссылка недействительна или устарела.") {
		t.Fatal("missing invalid magic-link message")
	}
	if !strings.Contains(s, "account-tabs") {
		t.Fatal("cabinet tabs should use account-tabs class for responsive layout")
	}
	if !strings.Contains(s, "flex-wrap: nowrap") {
		t.Fatal("account-tabs css should force single-line tabs")
	}
	if !strings.Contains(s, "overflow-x: auto") {
		t.Fatal("account-tabs css should allow horizontal scroll")
	}
	if !strings.Contains(s, "scrollbar-gutter: stable") || !strings.Contains(s, "overflow-y: scroll") {
		t.Fatal("session css should reserve vertical scrollbar gutter (scrollbar-gutter + overflow-y fallback)")
	}
	if !strings.Contains(s, "Перейти к моим услугам") || !strings.Contains(s, "js-card-goto-my-services") {
		t.Fatal("catalog success must offer 'go to my services' instead of full reload")
	}
	if !strings.Contains(s, "showMyServicesTabScrollCards") {
		t.Fatal("expected helper to switch to services tab and scroll to #cards")
	}
	if !strings.Contains(s, "refreshAccountSnapshot(tok).catch(function () {})") {
		t.Fatal("after successful order, services list should refresh via refreshAccountSnapshot")
	}
	if strings.Contains(s, "location.reload") {
		t.Fatal("session must not use location.reload for catalog / post-purchase refresh")
	}
	if !strings.Contains(s, "function resetCatalogOrderState") {
		t.Fatal("session.js must reset catalog cards after unpaid service deletion")
	}
	if !strings.Contains(s, ".querySelectorAll('.js-buy-catalog')") ||
		!strings.Contains(s, ".querySelectorAll('.catalog-card-success')") ||
		!strings.Contains(s, ".querySelectorAll('.js-catalog-success-text')") ||
		!strings.Contains(s, ".querySelectorAll('.js-card-pay')") ||
		!strings.Contains(s, ".querySelectorAll('.catalog-card-err')") {
		t.Fatal("resetCatalogOrderState must target catalog card hooks")
	}
	if !strings.Contains(s, `btn.textContent = 'Купить'`) ||
		!strings.Contains(s, `btn.disabled = false`) {
		t.Fatal("resetCatalogOrderState must restore buy buttons from post-order pending state")
	}
	iDelete := strings.Index(s, `'/api/account/service/delete'`)
	if iDelete < 0 || !strings.Contains(s[iDelete:], "resetCatalogOrderState()") {
		t.Fatal("delete-success flow must call resetCatalogOrderState")
	}
	iAfterRefresh := strings.Index(s[iDelete:], "refreshAccountSnapshot(tok).then(function () {")
	if iAfterRefresh < 0 {
		t.Fatal("delete flow must refresh snapshot before resetting catalog UI")
	}
	deleteRefreshBlock := s[iDelete+iAfterRefresh:]
	if !strings.Contains(deleteRefreshBlock, "resetCatalogOrderState()") {
		t.Fatal("resetCatalogOrderState must appear after delete refresh snapshot chain")
	}
	iOrderAwait := strings.Index(s, `buyBtn.textContent = 'Ожидает оплаты'`)
	if iOrderAwait < 0 {
		t.Fatal("expected post-order buy button label in session")
	}
	iOrderScroll := strings.Index(s[iOrderAwait:], "wrap.scrollIntoView")
	if iOrderScroll < 0 {
		t.Fatal("expected catalog scroll after successful order")
	}
	orderOkFragment := s[iOrderAwait : iOrderAwait+iOrderScroll]
	if strings.Contains(orderOkFragment, "resetCatalogOrderState") {
		t.Fatal("order success must not reset catalog order state immediately after purchase")
	}
	if strings.Contains(s, `(50–10 000 ₽, до 2 знаков)</label>`) {
		t.Fatal("ambiguous topup custom amount label must clarify decimal digits")
	}
	if !strings.Contains(s, `50–10 000 ₽, до 2 знаков после запятой`) {
		t.Fatal("topup modal must say «после запятой» for fractional amounts")
	}
	iTopRes := strings.Index(s, `id="topup-result" class="alert`)
	if iTopRes < 0 {
		t.Fatal("topup-result block missing")
	}
	jTopModal := strings.Index(s[iTopRes:], `<div class="modal fade" id="topupModal"`)
	if jTopModal < 0 {
		t.Fatal("topup modal markup anchor missing")
	}
	topResFrag := s[iTopRes : iTopRes+jTopModal]
	if strings.Contains(topResFrag, `id="topup-pay-open"`) {
		t.Fatal("topup-result must not use legacy topup-pay-open CTA")
	}
	if strings.Contains(topResFrag, `>Перейти к оплате<`) {
		t.Fatal("topup-result success must not duplicate modal «Перейти к оплате» button")
	}
	for _, needle := range []string{
		`Страница оплаты открыта в новой вкладке`,
		`обновите баланс. Баланс должен обновиться в течение 1–2 минут`,
		`topup-result-pay-fallback`,
		`Открыть оплату`,
		`topup-result-refresh`,
		`Обновить баланс`,
	} {
		if !strings.Contains(topResFrag, needle) {
			t.Fatalf("topup-result markup missing %q", needle)
		}
	}
	iTopSubmit := strings.Index(s, `getElementById('topup-submit').addEventListener`)
	if iTopSubmit < 0 {
		t.Fatal("topup submit handler missing")
	}
	topSubmitSnip := s[iTopSubmit:]
	if len(topSubmitSnip) > 2600 {
		topSubmitSnip = topSubmitSnip[:2600]
	}
	if !strings.Contains(topSubmitSnip, `window.open(urlRaw, '_blank', 'noopener')`) {
		t.Fatal("balance topup success must auto-open payment in a new tab")
	}
	iTopRefBind := strings.Index(s, `topupRefreshBtn.addEventListener`)
	if iTopRefBind < 0 {
		t.Fatal("topup balance refresh binding missing")
	}
	topRefSnip := s[iTopRefBind:]
	if idx := strings.Index(topRefSnip, `document.querySelectorAll('.amt-quick')`); idx > 0 {
		topRefSnip = topRefSnip[:idx]
	}
	if !strings.Contains(topRefSnip, `refreshAccountSnapshot(tok)`) {
		t.Fatal("«Обновить баланс» must call refreshAccountSnapshot(tok)")
	}
	idxSvcPayBlock := strings.Index(s, `"svc-pay-ok mt-2 d-none"`)
	if idxSvcPayBlock < 0 {
		t.Fatal("svc-pay-ok block missing")
	}
	svcPayBlock := s[idxSvcPayBlock:]
	if len(svcPayBlock) > 920 {
		svcPayBlock = svcPayBlock[:920]
	}
	if strings.Contains(svcPayBlock, `>Перейти к оплате`) {
		t.Fatal("svc-pay-ok block must not show a duplicate Перейти к оплате link")
	}
	if !strings.Contains(svcPayBlock, `Страница оплаты открыта в новой вкладке`) ||
		!strings.Contains(svcPayBlock, `js-svc-pay-fallback`) ||
		!strings.Contains(svcPayBlock, `Открыть оплату`) {
		t.Fatal("svc-pay-ok must include auto-open messaging and Открыть оплату fallback")
	}
	if strings.Contains(s, "js-svc-pay-open") {
		t.Fatal("legacy js-svc-pay-open link removed from unpaid service payment block")
	}
	for _, needle := range []string{
		`btn-success js-svc-balance-pay`,
		`Перейти к оплате</button>`,
		`После оплаты баланс будет пополнен`,
		`Обновить услуги`,
		`Отменить услугу`,
		`svcTopupAmountUsable`,
		`openBalanceTabWithTopupModal`,
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("session NOT PAID markup missing %q", needle)
		}
	}
	idxSvcPayHandler := strings.Index(s, `var payBtn = cardRoot.querySelector('.js-svc-balance-pay')`)
	if idxSvcPayHandler < 0 {
		t.Fatal("NOT PAID service card missing pay-button handler anchor")
	}
	svcPaySnip := s[idxSvcPayHandler:]
	if len(svcPaySnip) > 4500 {
		svcPaySnip = svcPaySnip[:4500]
	}
	for _, needle := range []string{
		"fetch('/api/account/balance/topup'",
		"Готовим оплату...",
		`JSON.stringify({ token: baseTok, amount: amt })`,
		"window.open(u, '_blank', 'noopener')",
	} {
		if !strings.Contains(svcPaySnip, needle) {
			t.Fatalf("NOT PAID pay handler missing %q", needle)
		}
	}
	if strings.Contains(svcPaySnip, "/api/account/service/order") {
		t.Fatal("NOT PAID service pay flow must not call service/order")
	}
	for _, forbid := range []string{"SHM", "Remnawave", "internal_squad_name"} {
		if strings.Contains(s, forbid) {
			t.Fatalf("session UI leak %q", forbid)
		}
	}
}
