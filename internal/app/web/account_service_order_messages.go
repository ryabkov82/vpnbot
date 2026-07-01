package web

import (
	"fmt"
	"math"
)

const (
	accountServiceOrderExistingUnpaidMessageEN   = "You already have a service awaiting payment. The newly selected service was not created. Complete crypto payment for the pending service. The invoice is calculated from the internal RUB amount."
	accountServiceOrderPendingMessageEN          = "Service is awaiting payment. Top up your balance — the service will activate automatically after payment."
	accountServiceOrderCreatedNoPaymentMessageEN = "Service created. It will activate automatically if your balance is sufficient."
)

func accountServiceOrderMessage(locale accountLocale, existingUnpaid, noPaymentNeeded bool, amount float64, hasCryptoURL bool) string {
	if locale == accountLocaleEN {
		return accountServiceOrderMessageEN(existingUnpaid, noPaymentNeeded, amount, hasCryptoURL)
	}
	if existingUnpaid {
		return accountServiceOrderExistingUnpaidMessage
	}
	if noPaymentNeeded {
		return accountServiceOrderCreatedNoPaymentMessage
	}
	return accountServiceOrderPendingMessage
}

func accountServiceOrderMessageEN(existingUnpaid, noPaymentNeeded bool, amount float64, hasCryptoURL bool) string {
	if existingUnpaid {
		return accountServiceOrderExistingUnpaidMessageEN
	}
	if noPaymentNeeded {
		return accountServiceOrderCreatedNoPaymentMessageEN
	}
	if hasCryptoURL {
		return fmt.Sprintf(
			"Service is awaiting payment. Pay via crypto using the link. The invoice is calculated from %s RUB (internal balance currency).",
			formatServiceOrderRUBAmountEN(amount),
		)
	}
	return accountServiceOrderPendingMessageEN
}

func formatServiceOrderRUBAmountEN(amount float64) string {
	if math.Abs(amount-math.Round(amount)) < 0.005 {
		return fmt.Sprintf("%.0f", amount)
	}
	return fmt.Sprintf("%.2f", amount)
}
