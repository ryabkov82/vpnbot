package payments

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

func buildSHMPaymentURL(baseURL string, path string, paySystem string, userID int, amount float64, ts int64) (string, error) {
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
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", errors.New("payment path is invalid")
	}
	paySystem = strings.TrimSpace(paySystem)
	if paySystem == "" {
		return "", errors.New("payment system is empty")
	}

	amountStr := strconv.FormatFloat(amount, 'f', -1, 64)
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("action", "create")
	q.Set("user_id", strconv.Itoa(userID))
	q.Set("ts", strconv.FormatInt(ts, 10))
	q.Set("ps", paySystem)
	q.Set("amount", amountStr)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
