package payments

import (
	"errors"
	"net/url"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

const shmYooKassaPath = "/shm/pay_systems/yookassa.cgi"

// BuildYooKassaPaymentURL собирает ссылку на создание платежа YooKassa в SHM.
// paySystem — ключ SHM config.pay_systems активного бренда (query ps=); пустой запрещён.
// brandID — активный brand.id (query brand_id=); пустой/невалидный — fail-closed.
func BuildYooKassaPaymentURL(baseURL string, userID int, amount float64, ts int64, paySystem, brandID string) (string, error) {
	brandID = strings.TrimSpace(brandID)
	if !config.IsValidBrandID(brandID) {
		if brandID == "" {
			return "", errors.New("brand id is empty")
		}
		return "", errors.New("brand id is invalid")
	}
	raw, err := buildSHMPaymentURL(baseURL, shmYooKassaPath, paySystem, userID, amount, ts)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("brand_id", brandID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
