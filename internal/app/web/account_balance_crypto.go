package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/payments"
)

const accountBalanceCryptoTopupMessage = "После оплаты баланс будет пополнен. Если средств достаточно, сервис автоматически активирует неоплаченные услуги или использует баланс для будущего продления. При частичной оплате доступ может не активироваться автоматически. Если платеж не зачислился, обратитесь в поддержку."

func serveAccountBalanceTopupCrypto(cfg *config.Config, _ accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/balance/topup/cryptocloud" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req accountBalanceTopupRequestJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		raw := strings.TrimSpace(req.Token)
		if raw == "" {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		claims, err := ParseAndVerifyAccountToken(secret, raw)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")
			return
		}

		if !accountTopupAmountValid(req.Amount) {
			writeJSONError(w, http.StatusBadRequest, "invalid_amount")
			return
		}

		baseURL := ""
		if cfg != nil {
			baseURL = cfg.API.BaseURL
		}
		amountRounded := math.Round(req.Amount*100) / 100
		paymentURL, err := payments.BuildCryptoCloudPaymentURL(baseURL, claims.UserID, amountRounded, time.Now().Unix())
		if err != nil {
			slog.Error("account balance topup cryptocloud: BuildCryptoCloudPaymentURL", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "payment_url_failed")
			return
		}

		writeJSON(w, http.StatusOK, accountBalanceTopupOKJSON{
			Status:     "payment_required",
			Amount:     amountRounded,
			PaymentURL: paymentURL,
			Message:    accountBalanceCryptoTopupMessage,
		})
	}
}
