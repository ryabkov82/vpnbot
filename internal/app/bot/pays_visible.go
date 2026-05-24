package bot

import (
	"encoding/json"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/models"
)

// userPayIsHiddenCanceledZero — техническая отмена YooKassa: money==0 и признаки canceled.
func userPayIsHiddenCanceledZero(p models.UserPay) bool {
	if p.Money != 0 {
		return false
	}
	ps := strings.ToLower(strings.TrimSpace(p.PaySystemID))
	if ps == "yookassa-canceled" || strings.Contains(ps, "canceled") {
		return true
	}
	if len(p.Comment) == 0 {
		return false
	}
	var meta struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(p.Comment, &meta); err != nil {
		return false
	}
	return strings.TrimSpace(meta.Event) == "payment.canceled"
}

// visibleUserPaysForBot убирает из истории отменённые YooKassa с суммой 0 для UX в боте.
func visibleUserPaysForBot(pays []models.UserPay) []models.UserPay {
	if len(pays) == 0 {
		return nil
	}
	out := make([]models.UserPay, 0, len(pays))
	for i := range pays {
		p := pays[i]
		if userPayIsHiddenCanceledZero(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func paysListCaption(visible []models.UserPay, rawCount int) string {
	if len(visible) == 0 {
		if rawCount == 0 {
			return "Платежей пока нет."
		}
		return "Оплаченных платежей пока нет."
	}
	return "Платежи"
}
