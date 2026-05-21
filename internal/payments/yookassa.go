package payments

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// BuildYooKassaPaymentURL собирает ссылку на создание платежа YooKassa в SHM.
func BuildYooKassaPaymentURL(baseURL string, userID int, amount float64, ts int64) (string, error) {
	if userID <= 0 {
		return "", errors.New("user id must be positive")
	}
	if amount <= 0 {
		return "", errors.New("amount must be positive")
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", errors.New("base url is empty")
	}

	amountStr := strconv.FormatFloat(amount, 'f', -1, 64)
	u, err := url.Parse(base + "/shm/pay_systems/yookassa.cgi")
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("action", "create")
	q.Set("user_id", strconv.Itoa(userID))
	q.Set("ts", strconv.FormatInt(ts, 10))
	q.Set("ps", "yookassa")
	q.Set("amount", amountStr)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
