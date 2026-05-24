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
	if !strings.Contains(s, `'/api/account/payments?token='`) || !strings.Contains(s, "function loadAccountPayments") || !strings.Contains(s, "function renderAccountPayments") {
		t.Fatal("session must load payments via loadAccountPayments /api/account/payments")
	}
	if !strings.Contains(s, `id="tab-payments-tab"`) || !strings.Contains(s, `data-bs-target="#tab-payments"`) ||
		!strings.Contains(s, `id="tab-payments"`) || !strings.Contains(s, `id="payments-list"`) ||
		!strings.Contains(s, `id="payments-refresh"`) {
		t.Fatal("session payments tab-pane or payments-list / refresh anchor missing")
	}
	iRfPay := strings.Index(s, `function refreshAccountSnapshot`)
	if iRfPay >= 0 {
		refreshBody := s[iRfPay:]
		if k := strings.Index(refreshBody, `function showMyServicesTabScrollCards`); k > 0 {
			refreshBody = refreshBody[:k]
		}
		if strings.Contains(refreshBody, "loadAccountPayments") {
			t.Fatal("refreshAccountSnapshot must not load payments unconditionally")
		}
	}
	if !strings.Contains(s, `addEventListener('shown.bs.tab'`) ||
		!strings.Contains(s, `getElementById('tab-payments-tab')`) {
		t.Fatal("session must wire shown.bs.tab on tab-payments-tab for lazy payments load")
	}
	if !strings.Contains(s, "var paymentsLoaded") || !strings.Contains(s, "var accountPaymentsCache") || !strings.Contains(s, "var dashboardToken") {
		t.Fatal("session must declare paymentsLoaded, accountPaymentsCache, dashboardToken")
	}
	if !strings.Contains(s, "function bindDashboardPayments") || !strings.Contains(s, "bindDashboardPayments(") {
		t.Fatal("expected bindDashboardPayments for payments tab/refresher wiring")
	}
	if !strings.Contains(s, "Загружаем платежи…") {
		t.Fatal("session payments load copy missing")
	}
	if !strings.Contains(s, "История платежей") {
		t.Fatal("session heading for payment history missing")
	}
	if !strings.Contains(s, "Откройте вкладку, чтобы загрузить историю платежей.") {
		t.Fatal("payments tab initial placeholder missing")
	}
	if !strings.Contains(s, "Оплаченных платежей пока нет.") {
		t.Fatal("session must include empty-state copy for filtered payments")
	}
	if strings.Contains(s, `uniq_key`) {
		t.Fatal("session UI must not reference uniq_key from payments API")
	}
	if strings.Contains(strings.ToLower(s), `function bindpaymentsrefresh`) || strings.Contains(s, `bindPaymentsRefresh`) {
		t.Fatal("session must bind payments tab via bindDashboardPayments, not bindPaymentsRefresh")
	}
	if !strings.Contains(s, "function bootFromRawToken") {
		t.Fatal("expected bootFromRawToken bootstrap")
	}
	for _, needle := range []string{
		`function openPaymentWindow`,
		`function navigatePaymentWindow`,
		`function closePaymentWindow`,
		`window.open('about:blank', '_blank')`,
		`win.opener = null`,
		`win.location.href = u`,
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("session payment window helpers missing %q", needle)
		}
	}
	if strings.Contains(s, `window.open('', '_blank', 'noopener')`) {
		t.Fatal("pre-open must not use noopener (returns null in some browsers)")
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
	if !strings.Contains(s, `Вы вошли как ' + String(j.user.email`) {
		t.Fatal("user-line must show human-friendly email only (Вы вошли как …)")
	}
	if strings.Contains(s, `j.user.login + ' · id '`) || strings.Contains(s, "' · ' + j.user.login") {
		t.Fatal("session UI must not concatenate login or user_id into user-line")
	}
	if strings.Contains(s, `getElementById('user-line').textContent =`) && strings.Contains(s, ` · id `) {
		t.Fatal("user-line assignment must not expose internal id label")
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
	iBalWrap := strings.Index(s, `id="balance-wrap"`)
	iCabTabs := strings.Index(s, `id="cabinet-tabs"`)
	iPanePayments := strings.Index(s, `id="tab-payments"`)
	iPaneSvcEarly := strings.Index(s, `id="tab-pane-services"`)
	if iBalWrap < 0 || iCabTabs < 0 || iPanePayments < 0 || iPaneSvcEarly < 0 {
		t.Fatal("balance-wrap, cabinet-tabs, tab-payments, or tab-pane-services anchor missing")
	}
	if !(iBalWrap < iCabTabs && iCabTabs < iPaneSvcEarly && iPaneSvcEarly < iPanePayments) {
		t.Fatal("balance and tab nav must precede tab panes; payments pane must be inside tab-content after services pane")
	}
	if strings.Index(s[iPanePayments:], `id="payments-list"`) < 0 {
		t.Fatal("payments-list must live inside payments tab-pane")
	}
	if strings.Count(s, `data-bs-toggle="pill"`) != 3 {
		t.Fatal("cabinet must have exactly three pills (services + buy + payments)")
	}
	for _, forbid := range []string{
		`id="tab-balance-tab"`,
		`id="tab-pane-balance"`,
		`aria-controls="tab-pane-balance"`,
		`data-bs-target="#tab-pane-balance"`,
		`getElementById('tab-balance-tab')`,
	} {
		if strings.Contains(s, forbid) {
			t.Fatalf("session must not retain balance-tab artefact %q", forbid)
		}
	}
	iOpenBal := strings.Index(s, `function openBalanceTabWithTopupModal()`)
	if iOpenBal < 0 {
		t.Fatal("openBalanceTabWithTopupModal definition missing")
	}
	openBalFunc := s[iOpenBal:]
	if jOpen := strings.Index(openBalFunc, `function renderServiceCards`); jOpen > 0 {
		openBalFunc = openBalFunc[:jOpen]
	}
	if strings.Contains(openBalFunc, `tab-balance-tab`) || strings.Contains(openBalFunc, `bootstrap.Tab`) {
		t.Fatal("openBalanceTabWithTopupModal must only open modal, not switch pills")
	}
	iTopSubmit := strings.Index(s, `getElementById('topup-submit').addEventListener`)
	if iTopSubmit < 0 {
		t.Fatal("topup submit handler missing")
	}
	topSubmitSnip := s[iTopSubmit:]
	jTopSubmitEnd := strings.Index(topSubmitSnip, "\n\t\tfunction refreshAccountSnapshot")
	if jTopSubmitEnd > 0 {
		topSubmitSnip = topSubmitSnip[:jTopSubmitEnd]
	}
	if !strings.Contains(topSubmitSnip, `openPaymentWindow()`) ||
		!strings.Contains(topSubmitSnip, `navigatePaymentWindow(payWin, urlRaw)`) ||
		!strings.Contains(topSubmitSnip, `closePaymentWindow(payWin)`) {
		t.Fatal("balance topup must open blank tab then navigatePaymentWindow after fetch")
	}
	if strings.Contains(topSubmitSnip, `getElementById('tab-balance-tab')`) {
		t.Fatal("topup success must not force-switch removed balance pill")
	}
	if !strings.Contains(topSubmitSnip, `getElementById('balance-wrap')`) ||
		!strings.Contains(topSubmitSnip, `scrollIntoView`) {
		t.Fatal("topup success must reveal result and scroll the balance card into view")
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
		`navigatePaymentWindow(payWin, u)`,
		`closePaymentWindow(payWin)`,
	} {
		if !strings.Contains(svcPaySnip, needle) {
			t.Fatalf("NOT PAID pay handler missing %q", needle)
		}
	}
	if strings.Contains(svcPaySnip, "/api/account/service/order") {
		t.Fatal("NOT PAID service pay flow must not call service/order")
	}
	idxCatBuy := strings.Index(s, `buyBtn.addEventListener('click',`)
	if idxCatBuy < 0 {
		t.Fatal("catalog buy handler anchor missing")
	}
	idxCatFetch := strings.Index(s[idxCatBuy:], `fetch('/api/account/service/order',`)
	if idxCatFetch < 0 {
		t.Fatal("catalog service/order fetch missing")
	}
	idxCatFetch += idxCatBuy
	catPreorder := s[idxCatBuy:idxCatFetch]
	if strings.Contains(catPreorder, `openPaymentWindow`) {
		t.Fatal("catalog buy must not use openPaymentWindow — payment opens via «Перейти к оплате» link")
	}
	for _, forbid := range []string{"SHM", "Remnawave", "internal_squad_name"} {
		if strings.Contains(s, forbid) {
			t.Fatalf("session UI leak %q", forbid)
		}
	}
	for _, needle := range []string{
		`id="logout-btn"`,
		`>Выйти</button>`,
		`localStorage.removeItem(STORAGE)`,
		`'/account?logged_out=1'`,
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("session logout UI/JS missing %q", needle)
		}
	}
	if strings.Count(s, `id="user-line"`) != 1 {
		t.Fatal("session must expose exactly one #user-line for dashboard email")
	}
	if !strings.Contains(s, `if (!rawTok)`) || !strings.Contains(s, `show('no-token', true)`) {
		t.Fatal("session must reveal no-token when URL and storage lack a token")
	}
}
