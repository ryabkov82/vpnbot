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
	if !strings.Contains(s, "var suppressNextTopupForecastApply") {
		t.Fatal("session must declare suppressNextTopupForecastApply for catalog top-up prefill")
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
	if !strings.Contains(s, ">Платежи</button>") {
		t.Fatal("payments tab nav label must be «Платежи»")
	}
	if !strings.Contains(s, `<h2 class="h5 mb-0">История платежей</h2>`) {
		t.Fatal("session heading for payment history missing inside pane")
	}
	if !strings.Contains(s, `id="tab-help-tab"`) || !strings.Contains(s, `id="tab-pane-help"`) ||
		!strings.Contains(s, ">Помощь</button>") {
		t.Fatal("help tab nav or pane missing")
	}
	if !strings.Contains(s, "Как подключить VPN") ||
		!strings.Contains(s, "«Купить VPN»") ||
		!strings.Contains(s, "«Мои услуги»") ||
		!strings.Contains(s, "«Подключить»") {
		t.Fatal("help tab must include VPN setup instructions")
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
	if !strings.Contains(s, "<!--ACCOUNT_SESSION_SUPPORT_LINK_BLOCK-->") {
		t.Fatal("session.html must include support link placeholder for server-side injection")
	}
	for _, needle := range []string{
		`<footer `,
		`account-footer`,
		`VPN for Friends</div>`,
		`Безопасный доступ к вашим VPN-услугам`,
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("session.html branded footer missing %q", needle)
		}
	}
	if !strings.Contains(s, ".account-footer") || !strings.Contains(s, "safe-area-inset-bottom") {
		t.Fatal("session footer must include .account-footer CSS with safe-area bottom inset")
	}
	if !strings.Contains(s, "flex-wrap: nowrap") {
		t.Fatal("account-tabs css should force single-line tabs")
	}
	if !strings.Contains(s, "overflow-x: auto") {
		t.Fatal("account-tabs css should allow horizontal scroll")
	}
	if !strings.Contains(s, "@media (max-width: 420px)") ||
		!strings.Contains(s, ".account-tabs .nav-link") ||
		!strings.Contains(s, "padding-left: 0.45rem") ||
		!strings.Contains(s, "font-size: 0.875rem") ||
		!strings.Contains(s, "gap: 0.15rem") {
		t.Fatal("account-tabs must include compact mobile css for narrow screens")
	}
	if !strings.Contains(s, "scrollbar-gutter: stable") || !strings.Contains(s, "overflow-y: scroll") {
		t.Fatal("session css should reserve vertical scrollbar gutter (scrollbar-gutter + overflow-y fallback)")
	}
	if !strings.Contains(s, `Вы вошли как ' + String(j.user.email`) {
		t.Fatal("user-line must show human-friendly email only (Вы вошли как …)")
	}
	if !strings.Contains(s, `id="account-telegram"`) || !strings.Contains(s, "function updateAccountTelegramLine(user)") {
		t.Fatal("session must expose linked Telegram line via updateAccountTelegramLine")
	}
	if !strings.Contains(s, "user.telegram_linked") || !strings.Contains(s, "'Telegram: ' + uname") ||
		!strings.Contains(s, "'Telegram: ID ' + String(user.telegram_chat_id)") {
		t.Fatal("updateAccountTelegramLine must branch on telegram_linked / username / chat_id")
	}
	if !strings.Contains(s, "updateAccountTelegramLine(j.user)") {
		t.Fatal("services payload must refresh account-telegram line")
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
	if !strings.Contains(s, "function openServicesTab()") || !strings.Contains(s, "showMyServicesTabScrollCards") {
		t.Fatal("expected openServicesTab helper and showMyServicesTabScrollCards")
	}
	if !strings.Contains(s, "refreshAccountSnapshot(tok).then(function () {") {
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
	iAfterRefresh := strings.Index(s[iDelete:], "refreshAccountSnapshot(tok).then(function (services) {")
	if iAfterRefresh < 0 {
		t.Fatal("delete flow must refresh snapshot before resetting catalog UI")
	}
	deleteRefreshBlock := s[iDelete+iAfterRefresh:]
	if !strings.Contains(deleteRefreshBlock, "resetCatalogOrderState()") {
		t.Fatal("resetCatalogOrderState must appear after delete refresh snapshot chain")
	}
	if !strings.Contains(s[iDelete:], "deletingServiceIDs.add(idNum)") ||
		!strings.Contains(s[iDelete:], "pendingPostDeleteCatalog = true") {
		t.Fatal("delete flow must track deleting service id and pending catalog transition")
	}
	if !strings.Contains(deleteRefreshBlock, "openCatalogTabIfNoServices(services)") {
		t.Fatal("delete flow must open catalog tab only when snapshot has no services")
	}
	if strings.Contains(deleteRefreshBlock, `getElementById('tab-buy-tab')`) {
		t.Fatal("delete flow must not switch buy tab unconditionally; use openCatalogTabIfNoServices")
	}
	iOrderFetch := strings.Index(s, `fetch('/api/account/service/order',`)
	if iOrderFetch < 0 {
		t.Fatal("catalog service/order fetch missing")
	}
	iOrderAwait := strings.Index(s[iOrderFetch:], `buyBtn.textContent = 'Ожидает оплаты'`)
	if iOrderAwait < 0 {
		t.Fatal("expected post-order buy button label in session")
	}
	iOrderAwait += iOrderFetch
	iOrderEnd := strings.Index(s[iOrderAwait:], "afterOrderSnapshotReady()")
	if iOrderEnd < 0 {
		t.Fatal("order success handler missing afterOrderSnapshotReady")
	}
	iOrderCatch := strings.Index(s[iOrderAwait+iOrderEnd:], `}).catch(function () {`)
	if iOrderCatch < 0 {
		t.Fatal("order success handler boundary missing")
	}
	orderOkFragment := s[iOrderAwait : iOrderAwait+iOrderEnd+iOrderCatch]
	if strings.Contains(orderOkFragment, "resetCatalogOrderState") {
		t.Fatal("order success must not reset catalog order state immediately after purchase")
	}
	if !strings.Contains(orderOkFragment, "refreshAccountSnapshot(tok).then(function () {") ||
		!strings.Contains(orderOkFragment, "openServicesTab()") {
		t.Fatal("successful order must refresh snapshot then open services tab")
	}
	if !strings.Contains(orderOkFragment, "orderAmtNum > 0") ||
		!strings.Contains(orderOkFragment, "openTopupModalSuggestingOrderAmount(orderAmtNum, orderMsg)") {
		t.Fatal("catalog order with positive amount must open top-up modal only when amount > 0")
	}
	if !strings.Contains(orderOkFragment, "showOrderSuccessHint(orderMsg)") {
		t.Fatal("catalog order without top-up amount must show order-success hint on services tab")
	}
	if !strings.Contains(orderOkFragment, "creatingServiceIDs.add(newUsid)") {
		t.Fatal("order success must register creating context for returned user_service_id")
	}
	if strings.Contains(orderOkFragment, "openCatalogTabIfNoServices") {
		t.Fatal("order success must not call openCatalogTabIfNoServices when services exist")
	}
	if strings.Contains(orderOkFragment, "new-catalog-wrap") {
		t.Fatal("order success must not scroll catalog wrap")
	}
	if strings.Contains(orderOkFragment, `openTopupModalForPreparedPayment`) ||
		strings.Contains(orderOkFragment, `pendingDirectPaymentUrl`) {
		t.Fatal("catalog order must not use prepared-payment direct URL flow")
	}
	if strings.Contains(orderOkFragment, `payA.href = payUrl`) {
		t.Fatal("catalog order success must not assign pay link href directly; reuse top-up modal confirmation")
	}
	iOrderErr := strings.Index(s[iOrderFetch:], `if (!y.ok) {`)
	if iOrderErr < 0 {
		t.Fatal("order error branch missing")
	}
	orderErrFragment := s[iOrderFetch+iOrderErr : iOrderAwait]
	if strings.Contains(orderErrFragment, "openServicesTab()") {
		t.Fatal("failed order must stay on catalog tab without openServicesTab")
	}
	if strings.Contains(s, `(50–10 000 ₽, до 2 знаков)</label>`) {
		t.Fatal("ambiguous topup custom amount label must clarify decimal digits")
	}
	if !strings.Contains(s, `50–10 000 ₽, до 2 знаков после запятой`) {
		t.Fatal("topup modal must say «после запятой» for fractional amounts")
	}
	for _, fcNeedle := range []string{
		`id="topup-forecast-hint"`,
		`id="topup-no-forecast-msg"`,
		`var suppressNextTopupForecastApply`,
		`function openTopupModalSuggestingOrderAmount`,
		`Не удалось рассчитать сумму оплаты`,
		`Сумма рассчитана по данным биллинга для оплаты/продления услуг`,
		`var accountForecast = 0`,
		`function setAccountForecastFromServicesPayload`,
		`function applyTopupModalForecastDefaults`,
		`'shown.bs.modal'`,
		`hidden.bs.modal`,
		`if (suppressNextTopupForecastApply)`,
		`setAccountForecastFromServicesPayload(j)`,
		`function openTopupModalForBillingForecast`,
	} {
		if !strings.Contains(s, fcNeedle) {
			t.Fatalf("session forecast topup wiring missing %q", fcNeedle)
		}
	}
	if strings.Contains(s, `openTopupModalForNotPaidService`) {
		t.Fatal("session.html must rename openTopupModalForNotPaidService to openTopupModalForBillingForecast")
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
	if !strings.Contains(s[iPanePayments:], `id="payments-list"`) {
		t.Fatal("payments-list must live inside payments tab-pane")
	}
	if strings.Count(s, `data-bs-toggle="pill"`) != 4 {
		t.Fatal("cabinet must have exactly four pills (services + buy + payments + help)")
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
	if strings.Contains(topSubmitSnip, `pendingDirectPaymentUrl`) {
		t.Fatal("topup-submit must only create payment via /balance/topup, not prepared direct URL")
	}
	iRawTop := strings.Index(topSubmitSnip, `String(customIn.value`)
	iFetchTop := strings.Index(topSubmitSnip, `fetch('/api/account/balance/topup'`)
	if iRawTop < 0 || iFetchTop < 0 || iRawTop >= iFetchTop {
		t.Fatal("topup-submit must read #topup-custom before POST /balance/topup")
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
	if strings.Contains(s, `data-pay-amt`) {
		t.Fatal("session must not use data-pay-amt for balance forecast billing (use SHM forecast via modal)")
	}
	if !strings.Contains(s, `var forecastBilling = notPaid || blocked`) {
		t.Fatal("session must tie NOT PAID and BLOCK cards to forecast-based balance top-up")
	}
	if !strings.Contains(s, `var blocked = stUp === 'BLOCK'`) {
		t.Fatal("session renderServiceCards must detect BLOCK status for forecast billing")
	}
	if !strings.Contains(s, `Пополнить для активации`) || !strings.Contains(s, `Пополнить для продления`) {
		t.Fatal("session must expose distinct forecast top-up labels for NOT PAID vs BLOCK")
	}
	if !strings.Contains(s, `продлена автоматически, когда средств будет достаточно`) {
		t.Fatal("session BLOCK helper copy for balance renewal missing")
	}
	for _, needle := range []string{
		`btn-success js-svc-balance-pay`,
		`После оплаты баланс будет пополнен`,
		`Обновить услуги`,
		`Отменить услугу`,
		`svcTopupAmountUsable`,
		`openBalanceTabWithTopupModal`,
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("session forecast-billing card markup missing %q", needle)
		}
	}
	idxSvcPayHandler := strings.Index(s, `var payBtn = cardRoot.querySelector('.js-svc-balance-pay')`)
	if idxSvcPayHandler < 0 {
		t.Fatal("service cards with forecast billing missing pay-button handler anchor")
	}
	svcPaySnip := s[idxSvcPayHandler:]
	if len(svcPaySnip) > 1500 {
		svcPaySnip = svcPaySnip[:1500]
	}
	for _, needle := range []string{
		`openTopupModalForBillingForecast`,
		`payBtn.addEventListener('click'`,
	} {
		if !strings.Contains(svcPaySnip, needle) {
			t.Fatalf("NOT PAID/BLOCK forecast pay handler missing %q", needle)
		}
	}
	if strings.Contains(svcPaySnip, `getAttribute('data-pay-amt')`) {
		t.Fatal("forecast billing pay must not read amount from tariff data-pay-amt")
	}
	if strings.Contains(svcPaySnip, `fetch('/api/account/balance/topup'`) {
		t.Fatal("forecast billing pay must not POST topup from card handler; use balance modal")
	}
	if strings.Contains(svcPaySnip, "/api/account/service/order") {
		t.Fatal("service card forecast pay flow must not call service/order")
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
	for _, forbidSession := range []string{
		`openTopupModalForPreparedPayment`,
		`pendingDirectPaymentUrl`,
		`topup-prepared-msg`,
	} {
		if strings.Contains(s, forbidSession) {
			t.Fatalf("session must not retain prepared-payment artefact %q", forbidSession)
		}
	}
	if strings.Count(s, `id="user-line"`) != 1 {
		t.Fatal("session must expose exactly one #user-line for dashboard email")
	}
	if !strings.Contains(s, `if (!rawTok)`) || !strings.Contains(s, `show('no-token', true)`) {
		t.Fatal("session must reveal no-token when URL and storage lack a token")
	}
}

func TestAccountSessionCatalogTabIfNoServices(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatalf("read session.html: %v", err)
	}
	s := string(b)

	if !strings.Contains(s, "function openCatalogTabIfNoServices(services)") {
		t.Fatal("missing openCatalogTabIfNoServices helper")
	}
	iHelper := strings.Index(s, "function openCatalogTabIfNoServices(services)")
	helperBlock := s[iHelper:]
	if j := strings.Index(helperBlock, "function "); j > 0 {
		helperBlock = helperBlock[:j]
	}
	if !strings.Contains(helperBlock, "list.length > 0") {
		t.Fatal("helper must skip catalog tab when services remain")
	}
	if !strings.Contains(helperBlock, `getElementById('tab-buy-tab')`) ||
		!strings.Contains(helperBlock, "bootstrap.Tab.getOrCreateInstance(buyTabBtn).show()") {
		t.Fatal("helper must switch catalog tab via Bootstrap Tab API")
	}

	iBoot := strings.Index(s, "function bootDashboardAfterExchange")
	if iBoot < 0 {
		t.Fatal("bootDashboardAfterExchange missing")
	}
	bootBlock := s[iBoot:]
	if j := strings.Index(bootBlock, "function bootFromRawToken"); j > 0 {
		bootBlock = bootBlock[:j]
	}
	if !strings.Contains(bootBlock, "renderServiceCards(accountTok, j.services || [])") {
		t.Fatal("initial load must render services from /api/account/services payload")
	}
	if !strings.Contains(bootBlock, "openCatalogTabIfNoServices(j.services || [])") {
		t.Fatal("initial dashboard load must call openCatalogTabIfNoServices after services fetch")
	}
	iBootOpen := strings.Index(bootBlock, "openCatalogTabIfNoServices(j.services || [])")
	iBootRender := strings.Index(bootBlock, "renderServiceCards(accountTok, j.services || [])")
	if iBootOpen < iBootRender {
		t.Fatal("initial load must render services before optional catalog tab switch")
	}

	iDelete := strings.Index(s, `'/api/account/service/delete'`)
	if iDelete < 0 {
		t.Fatal("delete handler missing")
	}
	iDelRefresh := strings.Index(s[iDelete:], "refreshAccountSnapshot(tok).then(function (services) {")
	if iDelRefresh < 0 {
		t.Fatal("delete must refresh snapshot and receive services list")
	}
	deleteBlock := s[iDelete+iDelRefresh:]
	if j := strings.Index(deleteBlock, "function showInvalidSessionLink"); j > 0 {
		deleteBlock = deleteBlock[:j]
	}
	if !strings.Contains(deleteBlock, "openCatalogTabIfNoServices(services)") {
		t.Fatal("delete flow must call openCatalogTabIfNoServices after refresh")
	}
	if strings.Contains(deleteBlock, `getElementById('tab-buy-tab')`) {
		t.Fatal("delete flow must not unconditionally switch to buy tab")
	}
	if !strings.Contains(deleteBlock, "services.length === 0") {
		t.Fatal("post-delete hint must show only when no services remain")
	}
}

func TestAccountSessionPostOrderGoesToServicesTab(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatalf("read session.html: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `id="order-success-hint"`) || !strings.Contains(s, "function showOrderSuccessHint(message)") {
		t.Fatal("session must expose order-success hint on services tab")
	}
	iOrder := strings.Index(s, `fetch('/api/account/service/order',`)
	if iOrder < 0 {
		t.Fatal("service/order handler missing")
	}
	orderBlock := s[iOrder:]
	if j := strings.Index(orderBlock, "function loadAccountPayments"); j > 0 {
		orderBlock = orderBlock[:j]
	}
	if !strings.Contains(orderBlock, "openServicesTab()") {
		t.Fatal("post-order flow must open services tab")
	}
	if strings.Contains(orderBlock, "openCatalogTabIfNoServices") {
		t.Fatal("catalog order handler must not call openCatalogTabIfNoServices")
	}
}

func TestAccountSessionProgressPolling(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatalf("read session.html: %v", err)
	}
	s := string(b)
	for _, needle := range []string{
		"var progressPollingTimer = null",
		"function hasProgressServices(services)",
		"function updateProgressPolling(services)",
		"function startProgressPolling()",
		"function stopProgressPolling()",
		"function startProgressPollingIfNeeded(services)",
		"progressPollingIntervalMs = 5000",
		"document.hidden",
		"visibilitychange",
		"document.visibilityState !== 'visible'",
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("progress polling helper missing %q", needle)
		}
	}
	if !strings.Contains(s, "Услуга создаётся. Обычно это занимает до 1–2 минут.") ||
		!strings.Contains(s, "Услуга удаляется. Обычно это занимает до 1–2 минут.") ||
		!strings.Contains(s, "Выполняется операция с услугой. Обычно это занимает до 1–2 минут.") ||
		!strings.Contains(s, "function progressHintHtmlForService(usid)") ||
		!strings.Contains(s, "deletingServiceIDs.has(id)") ||
		!strings.Contains(s, "creatingServiceIDs.has(id)") ||
		!strings.Contains(s, "!active && !inProgress") {
		t.Fatal("renderServiceCards must branch PROGRESS copy and hide cancel for inProgress")
	}
	iRefresh := strings.Index(s, "function refreshAccountSnapshot(tok)")
	if iRefresh < 0 {
		t.Fatal("refreshAccountSnapshot missing")
	}
	refreshBlock := s[iRefresh:]
	if j := strings.Index(refreshBlock, "function hasProgressServices"); j > 0 {
		refreshBlock = refreshBlock[:j]
	}
	if !strings.Contains(refreshBlock, "syncProgressContextSets(services)") ||
		!strings.Contains(refreshBlock, "updateProgressPolling(services)") ||
		!strings.Contains(refreshBlock, "maybeOpenCatalogAfterPendingDeletion(services)") {
		t.Fatal("refreshAccountSnapshot must sync progress context, polling, and pending delete catalog")
	}
	iPollStart := strings.Index(s, "function startProgressPolling() {")
	if iPollStart < 0 {
		t.Fatal("startProgressPolling missing")
	}
	pollBlock := s[iPollStart:]
	if j := strings.Index(pollBlock, "function startProgressPollingIfNeeded"); j > 0 {
		pollBlock = pollBlock[:j]
	}
	if !strings.Contains(pollBlock, "if (progressPollingTimer !== null)") {
		t.Fatal("startProgressPolling must not create duplicate timers")
	}
	iPollStop := strings.Index(s, "function stopProgressPolling()")
	if iPollStop < 0 || !strings.Contains(s[iPollStop:], "clearInterval(progressPollingTimer)") {
		t.Fatal("stopProgressPolling must clear interval timer")
	}
	iPollUpdate := strings.Index(s, "function updateProgressPolling(services)")
	updateBlock := s[iPollUpdate:]
	if j := strings.Index(updateBlock, "function startProgressPollingIfNeeded"); j > 0 {
		updateBlock = updateBlock[:j]
	}
	if !strings.Contains(updateBlock, "hasProgressServices(services)") ||
		!strings.Contains(updateBlock, "startProgressPolling()") ||
		!strings.Contains(updateBlock, "stopProgressPolling()") {
		t.Fatal("updateProgressPolling must start/stop polling based on PROGRESS services")
	}
	iBoot := strings.Index(s, "function bootDashboardAfterExchange")
	if iBoot < 0 || !strings.Contains(s[iBoot:], "startProgressPollingIfNeeded(j.services || [])") {
		t.Fatal("initial dashboard load must start progress polling when needed")
	}
}

func TestAccountSessionProgressDeleteContext(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatalf("read session.html: %v", err)
	}
	s := string(b)
	for _, needle := range []string{
		"var deletingServiceIDs = new Set()",
		"var creatingServiceIDs = new Set()",
		"var pendingPostDeleteCatalog = false",
		"function maybeOpenCatalogAfterPendingDeletion(services)",
		"function syncProgressContextSets(services)",
		"creatingServiceIDs.add(newUsid)",
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("progress delete/create context missing %q", needle)
		}
	}
	iRender := strings.Index(s, "function renderServiceCards(tok, services)")
	if iRender < 0 {
		t.Fatal("renderServiceCards missing")
	}
	renderBlock := s[iRender:]
	if j := strings.Index(renderBlock, "function showInvalidSessionLink"); j > 0 {
		renderBlock = renderBlock[:j]
	}
	if !strings.Contains(renderBlock, "!active && !inProgress") {
		t.Fatal("PROGRESS cards must not render cancel or payment controls")
	}
	iDelete := strings.Index(s, `'/api/account/service/delete'`)
	iDelRefresh := strings.Index(s[iDelete:], "refreshAccountSnapshot(tok).then(function (services) {")
	deleteBlock := s[iDelete+iDelRefresh:]
	if j := strings.Index(deleteBlock, "function showInvalidSessionLink"); j > 0 {
		deleteBlock = deleteBlock[:j]
	}
	if strings.Contains(deleteBlock, "openCatalogTabIfNoServices(services)") &&
		strings.Contains(deleteBlock, "pendingPostDeleteCatalog = false") {
		// immediate empty list clears pending flag
	} else {
		t.Fatal("delete refresh must clear pendingPostDeleteCatalog when services already empty")
	}
	iMaybe := strings.Index(s, "function maybeOpenCatalogAfterPendingDeletion(services)")
	maybeBlock := s[iMaybe:]
	if j := strings.Index(maybeBlock, "function hasProgressServices"); j > 0 {
		maybeBlock = maybeBlock[:j]
	}
	if !strings.Contains(maybeBlock, "pendingPostDeleteCatalog") ||
		!strings.Contains(maybeBlock, "openCatalogTabIfNoServices(list)") ||
		!strings.Contains(maybeBlock, "post-delete-buy-hint") {
		t.Fatal("pending deletion must open buy tab after polling yields empty services")
	}
	iOrderFetch := strings.Index(s, `fetch('/api/account/service/order',`)
	iOrderAwait2 := strings.Index(s[iOrderFetch:], `buyBtn.textContent = 'Ожидает оплаты'`)
	if iOrderFetch < 0 || iOrderAwait2 < 0 {
		t.Fatal("order handler anchors missing")
	}
	iOrderAwait2 += iOrderFetch
	iOrderEnd2 := strings.Index(s[iOrderAwait2:], "refreshAccountSnapshot(tok).then(function () {")
	if iOrderEnd2 < 0 {
		t.Fatal("order refresh chain missing")
	}
	orderFrag := s[iOrderAwait2 : iOrderAwait2+iOrderEnd2]
	if !strings.Contains(orderFrag, "creatingServiceIDs.add(newUsid)") {
		t.Fatal("order flow must mark new user_service_id as creating context")
	}
	if strings.Contains(orderFrag, "pendingPostDeleteCatalog") {
		t.Fatal("order flow must not touch pendingPostDeleteCatalog")
	}
}
