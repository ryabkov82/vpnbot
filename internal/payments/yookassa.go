package payments

const shmYooKassaPath = "/shm/pay_systems/yookassa.cgi"

// BuildYooKassaPaymentURL собирает ссылку на создание платежа YooKassa в SHM.
// paySystem — ключ SHM config.pay_systems активного бренда (query ps=); пустой запрещён.
func BuildYooKassaPaymentURL(baseURL string, userID int, amount float64, ts int64, paySystem string) (string, error) {
	return buildSHMPaymentURL(baseURL, shmYooKassaPath, paySystem, userID, amount, ts)
}
