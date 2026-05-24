package bot

import "github.com/ryabkov82/vpnbot/internal/models"

func paysListCaption(visible []models.UserPay, rawCount int) string {
	if len(visible) == 0 {
		if rawCount == 0 {
			return "Платежей пока нет."
		}
		return "Оплаченных платежей пока нет."
	}
	return "Платежи"
}
