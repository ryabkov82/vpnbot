package payments

const shmCryptoCloudPath = "/shm/pay_systems/cryptocloud.cgi"

// BuildCryptoCloudPaymentURL собирает ссылку на создание платежа CryptoCloud/Trybit в SHM.
func BuildCryptoCloudPaymentURL(baseURL string, userID int, amount float64, ts int64) (string, error) {
	return buildSHMPaymentURL(baseURL, shmCryptoCloudPath, "cryptocloud", userID, amount, ts)
}
