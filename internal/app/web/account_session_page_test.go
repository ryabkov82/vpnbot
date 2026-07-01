package web

import (
	"bytes"
	"strings"
	"testing"
)

func TestAccountSessionEmbed_BalanceTopupAndHintsNoRenew(t *testing.T) {
	b := []byte(accountSessionPageTemplateSrc)
	raw := string(b)
	if strings.Contains(strings.ToUpper(raw), "SHM") {
		t.Fatal("embedded session must not show SHM to users")
	}
	if strings.Contains(raw, "Remnawave") || strings.Contains(raw, "check_exists_unpaid") {
		t.Fatal("session embed must not contain internal terminology")
	}
	for _, footerNeedle := range []string{
		`<footer `,
		`account-footer`,
		`{{.I18n.FooterBrand}}`,
		`{{.I18n.FooterTagline}}`,
	} {
		if !strings.Contains(raw, footerNeedle) {
			t.Fatalf("embed session branded footer missing %q", footerNeedle)
		}
	}
	if !strings.Contains(raw, "safe-area-inset-bottom") || !strings.Contains(raw, "calc(1.5rem + env(") {
		t.Fatal("embed session must include .account-footer safe-area bottom margin")
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
	if !strings.Contains(raw, "t('signedInAs')") {
		t.Fatal("embed user-line must show «Вы вошли как» email only")
	}
	if strings.Contains(raw, `j.user.login + ' · id '`) || strings.Contains(raw, "' · ' + j.user.login") {
		t.Fatal("embed must not concatenate login or user_id into user-line")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "Баланс:") {
		t.Fatal("balance label missing")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "Пополнить баланс") {
		t.Fatal("topup CTA missing")
	}
	if !bytes.Contains(b, []byte(`/api/account/balance/topup`)) {
		t.Fatal("topup endpoint missing")
	}
	if strings.Contains(raw, `(50–10 000 ₽, до 2 знаков)</label>`) {
		t.Fatal("embed: ambiguous topup amount label removed")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "50–10 000 ₽") {
		t.Fatal("embed: topup label must clarify decimal places")
	}
	for _, fcNeedle := range []string{
		`id="topup-forecast-hint"`,
		`id="topup-no-forecast-msg"`,
		`var suppressNextTopupForecastApply`,
		`function openTopupModalSuggestingOrderAmount`,
		`{{.I18n.TopUpNoForecast}}`,
		`{{.I18n.TopUpForecastHint}}`,
		`var accountForecast = 0`,
		`setAccountForecastFromServicesPayload`,
		`applyTopupModalForecastDefaults`,
		`'shown.bs.modal'`,
		`hidden.bs.modal`,
		`if (suppressNextTopupForecastApply)`,
		`function openTopupModalForBillingForecast`,
	} {
		if !strings.Contains(raw, fcNeedle) {
			t.Fatalf("embed session forecast topup wiring missing %q", fcNeedle)
		}
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
	ru := mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU)
	for _, needle := range []string{
		`Страница оплаты открыта в новой вкладке`,
		`обновите баланс. Баланс должен обновиться в течение 1–2 минут`,
		`Если страница оплаты не открылась автоматически`,
		`Обновить баланс`,
	} {
		if !strings.Contains(ru, needle) {
			t.Fatalf("rendered topup-result missing %q", needle)
		}
	}
	for _, needle := range []string{
		`topup-result-pay-fallback`,
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
	if strings.Contains(tsSnip, `pendingDirectPaymentUrl`) {
		t.Fatal("embed topup-submit must only use /balance/topup")
	}
	iRawEmbed := strings.Index(tsSnip, `String(customIn.value`)
	tsF := strings.Index(tsSnip, `var topupEndpoint = selectedTopupBalanceURL()`)
	if iRawEmbed < 0 || tsF < 0 || iRawEmbed >= tsF {
		t.Fatal("embed topup-submit must read amount input before POST /balance/topup")
	}
	if !strings.Contains(tsSnip, `non_json_response`) {
		t.Fatal("embed topup-submit must handle non-JSON backend responses")
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
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "автоматического продления") {
		t.Fatal("balance explainer missing")
	}
	if !strings.Contains(raw, "t('notPaidHint1')") {
		t.Fatal("NOT PAID hint missing")
	}
	if !strings.Contains(raw, "t('blockedHint')") {
		t.Fatal("BLOCK balance renewal hint missing")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "Купить новую услугу") {
		t.Fatal(`missing catalog section title`)
	}
	if !bytes.Contains(b, []byte(`/api/account/catalog/services`)) {
		t.Fatal("catalog endpoint missing")
	}
	if !bytes.Contains(b, []byte("/api/account/service/order")) {
		t.Fatal("service order endpoint missing")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "Мои услуги") {
		t.Fatal("services tab missing")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "Купить VPN") {
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
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), ">Платежи</button>") {
		t.Fatal("payments tab nav label missing")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "История платежей") {
		t.Fatal("payments pane heading missing")
	}
	if strings.Count(raw, `data-bs-toggle="pill"`) != 4 {
		t.Fatal("embed: cabinet must have four pills (services + buy + payments + help)")
	}
	if !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "Помощь") || !strings.Contains(mustRenderAccountSessionHTML(t, orderStartTestCfg(), accountLocaleRU), "Как подключить VPN") {
		t.Fatal("help tab missing")
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
	if !strings.Contains(raw, "t('buyCreating')") || !strings.Contains(raw, "t('buyCreatingService')") {
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
	if !strings.Contains(raw, "t('neutralUnpaidFallback')") ||
		!strings.Contains(raw, "dupUnpaidFallback") {
		t.Fatal(`expected honest order success JS copy missing`)
	}
	for _, needle := range []string{
		`neutralMsgFallback`,
		`serverMsg`,
		`openServicesTab()`,
		`openTopupModalSuggestingOrderAmount`,
		`orderAmtNum`,
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("catalog order success embed must contain %q", needle)
		}
	}
	if strings.Contains(raw, `payA.href = payUrl`) {
		t.Fatal("embed catalog order must route YooKassa via top-up confirmation modal")
	}
	if !strings.Contains(raw, "t('buyAwaitPayment')") {
		t.Fatal("post-order button label missing")
	}
	if !bytes.Contains(b, []byte(`js-card-pay`)) {
		t.Fatal("per-card pay button missing")
	}
	if !strings.Contains(raw, "t('goToMyServices')") || !bytes.Contains(b, []byte(`js-card-goto-my-services`)) {
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
	if !strings.Contains(raw, "t('cancelService')") {
		t.Fatal("cancel service button missing")
	}
	if !strings.Contains(raw, "!active") {
		t.Fatal("cancel controls must branch on !active (ACTIVE hides cancel)")
	}
	if !strings.Contains(raw, "t('notPaidHint2')") {
		t.Fatal("NOT PAID reschedule hint missing")
	}
	if !strings.Contains(raw, "tNamed('deleteConfirm'") {
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
	if !strings.Contains(raw[iDeleteAPI:], "refreshAccountSnapshot(tok).then(function (services) {") {
		t.Fatal("delete success must chain refreshAccountSnapshot before catalog reset")
	}
	if !strings.Contains(raw[iDeleteAPI:], "openCatalogTabIfNoServices(services)") {
		t.Fatal("delete success must conditionally open catalog tab via openCatalogTabIfNoServices")
	}
	iAwait := strings.Index(raw, `t('buyAwaitPayment')`)
	if iAwait < 0 {
		t.Fatal("post-order button literal missing")
	}
	postEnd := iAwait + 2800
	if postEnd > len(raw) {
		postEnd = len(raw)
	}
	postOrderFrag := raw[iAwait:postEnd]
	if !strings.Contains(postOrderFrag, "afterOrderSnapshotReady()") {
		t.Fatal("embed order success handler missing afterOrderSnapshotReady")
	}
	if strings.Contains(postOrderFrag, "resetCatalogOrderState") {
		t.Fatal("order success fragment must not call resetCatalogOrderState")
	}
	if !strings.Contains(postOrderFrag, "openServicesTab()") ||
		!strings.Contains(postOrderFrag, "refreshAccountSnapshot(tok).then(function () {") {
		t.Fatal("embed successful order must refresh services and open services tab")
	}
	if !strings.Contains(postOrderFrag, "orderAmtNum > 0") ||
		(!strings.Contains(postOrderFrag, "openTopupModalSuggestingOrderAmount(orderAmtNum, orderMsg)") &&
			!strings.Contains(postOrderFrag, "isENAccount && paymentUrl")) {
		t.Fatal("embed catalog positive-amount flow must open top-up modal for RU or crypto payment for EN")
	}
	if !strings.Contains(postOrderFrag, "isENAccount") || !strings.Contains(postOrderFrag, "navigatePaymentWindow(orderPayWin, paymentUrl)") {
		t.Fatal("embed EN order success must open crypto payment URL from order response")
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
	if !strings.Contains(emPayOk, `t('svcPayPageOpened')`) ||
		!strings.Contains(emPayOk, `js-svc-pay-fallback`) ||
		!strings.Contains(emPayOk, `t('openPayment')`) {
		t.Fatal("embed svc-pay-ok copy/fallback mismatch")
	}
	if strings.Contains(raw, "js-svc-pay-open") {
		t.Fatal("embed must not retain js-svc-pay-open")
	}
	if strings.Contains(raw, `data-pay-amt`) {
		t.Fatal("embed must not use data-pay-amt for forecast balance billing amounts")
	}
	if strings.Contains(raw, `openTopupModalForNotPaidService`) {
		t.Fatal("embed must rename forecast modal opener away from openTopupModalForNotPaidService")
	}
	if !strings.Contains(raw, `var forecastBilling = notPaid || blocked`) {
		t.Fatal("embed renderServiceCards must group NOT PAID and BLOCK forecast billing")
	}
	if !strings.Contains(raw, `var blocked = stUp === 'BLOCK'`) {
		t.Fatal("embed must detect BLOCK for forecast billing cards")
	}
	for _, needle := range []string{
		`btn-success js-svc-balance-pay`,
		`t('topUpForActivation')`,
		`t('topUpForRenewal')`,
		`t('svcPayAfterPay')`,
		`t('refreshServices')`,
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("embed forecast billing card markup missing %q", needle)
		}
	}
	idxEmbPay := strings.Index(raw, `var payBtn = cardRoot.querySelector('.js-svc-balance-pay')`)
	if idxEmbPay < 0 {
		t.Fatal("embed: NOT PAID/BLOCK forecast pay handler anchor missing")
	}
	emSnip := raw[idxEmbPay:]
	if len(emSnip) > 1200 {
		emSnip = emSnip[:1200]
	}
	for _, needle := range []string{
		`openTopupModalForBillingForecast`,
		`payBtn.addEventListener('click'`,
	} {
		if !strings.Contains(emSnip, needle) {
			t.Fatalf("embed forecast billing pay handler missing %q", needle)
		}
	}
	if strings.Contains(emSnip, `getAttribute('data-pay-amt')`) {
		t.Fatal("embed forecast billing pay must not derive amount from tariff data-pay-amt")
	}
	if strings.Contains(emSnip, `fetch('/api/account/balance/topup'`) {
		t.Fatal("embed forecast billing pay must not POST topup from card handler")
	}
	if strings.Contains(emSnip, "/api/account/service/order") {
		t.Fatal("embed forecast billing strip must not call service/order path")
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
	if !strings.Contains(raw, "{{.SupportLinkHTML}}") {
		t.Fatal("embed session must include support link placeholder")
	}
	for _, needle := range []string{
		`id="logout-btn"`,
		`localStorage.removeItem(STORAGE)`,
		`t('logoutRedirect')`,
		`if (!rawTok)`,
		`show('no-token', true)`,
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("embed session missing %q", needle)
		}
	}
	for _, forbidEmb := range []string{
		`openTopupModalForPreparedPayment`,
		`pendingDirectPaymentUrl`,
		`topup-prepared-msg`,
	} {
		if strings.Contains(raw, forbidEmb) {
			t.Fatalf("embed session must not retain prepared-payment artefact %q", forbidEmb)
		}
	}
	if strings.Count(raw, `id="user-line"`) != 1 {
		t.Fatal("embed session must have single #user-line")
	}
}
