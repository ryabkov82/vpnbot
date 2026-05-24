package models

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// IsCanceledZeroPayment — техническая отмена YooKassa: money == 0 и признаки canceled.
func IsCanceledZeroPayment(pay UserPay) bool {
	if pay.Money != 0 {
		return false
	}
	ps := strings.ToLower(strings.TrimSpace(pay.PaySystemID))
	if ps == "yookassa-canceled" || strings.Contains(ps, "canceled") {
		return true
	}
	if len(pay.Comment) == 0 {
		return false
	}
	var meta struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(pay.Comment, &meta); err != nil {
		return false
	}
	return strings.TrimSpace(meta.Event) == "payment.canceled"
}

// VisibleUserPays убирает из истории отменённые записи с нулевой суммой (бот, веб-кабинет).
func VisibleUserPays(pays []UserPay) []UserPay {
	if len(pays) == 0 {
		return nil
	}
	out := make([]UserPay, 0, len(pays))
	for i := range pays {
		p := pays[i]
		if IsCanceledZeroPayment(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// FormatRubAmount форматирует сумму в рублях для пользовательского UI.
func FormatRubAmount(v float64) string {
	if math.Abs(v-math.Round(v)) < 0.005 {
		return fmt.Sprintf("%.0f ₽", v)
	}
	return fmt.Sprintf("%.2f ₽", v)
}
