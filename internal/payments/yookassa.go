package payments

const shmYooKassaPath = "/shm/pay_systems/yookassa.cgi"

// BuildYooKassaPaymentURL собирает ссылку на создание платежа YooKassa в SHM.
func BuildYooKassaPaymentURL(baseURL string, userID int, amount float64, ts int64) (string, error) {
	return buildSHMPaymentURL(baseURL, shmYooKassaPath, "yookassa", userID, amount, ts)
}
