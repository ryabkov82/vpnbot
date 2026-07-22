package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/models"
	appService "github.com/ryabkov82/vpnbot/internal/service"
)

func TestAuthenticateWebAccount_WrongBrandToken_NoValidate(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "fc", "a@b.c", 42, "web_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{}
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments?token="+tok, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if st.validateWebAccountCalls != 0 {
		t.Fatalf("ValidateWebAccountUser must not run on wrong-brand token, calls=%d", st.validateWebAccountCalls)
	}
}

func TestAccountPayments_IdentityMismatch_BeforePays(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@b.c", 42, "web_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{validateWebAccountErr: appService.ErrUserIdentityMismatch}
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments?token="+tok, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "invalid_token" {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if st.validateWebAccountCalls != 1 {
		t.Fatalf("validate calls=%d", st.validateWebAccountCalls)
	}
}

func TestAccountBalance_IdentityMismatch_NoTopup(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@b.c", 42, "web_x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{validateWebAccountErr: appService.ErrUserIdentityMismatch}
	rec := httptest.NewRecorder()
	body := `{"token":"` + tok + `","amount":100}`
	serveAccountBalanceTopup(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAccountLoginStart_IdentityMismatch_NoEmailSent(t *testing.T) {
	mailCalls := 0
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		mailCalls++
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	st := &stubAccountWeb{findUserByWebEmailErr: appService.ErrUserIdentityMismatch}
	rec := httptest.NewRecorder()
	serveAccountLoginStart(cfg, st, rl).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"x@y.zz","website":""}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"email_sent"`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if mailCalls != 0 {
		t.Fatalf("must not send magic link, mailCalls=%d", mailCalls)
	}
}

func TestAccountServices_TelegramFieldsFromValidatedUser(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "tg@example.com", 12, "web_tg12", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		balance: &models.UserBalance{Balance: 1, Forecast: 0},
		validateWebAccountRet: &models.User{
			ID:    12,
			Login: "web_tg12",
			Settings: models.UserSettings{
				BrandID: "vff",
				Web:     models.WebInfo{Email: "tg@example.com"},
				Telegram: models.TelegramInfo{
					ChatID:   99,
					Username: "tguser",
				},
			},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServices(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if st.getUserByIDCalls != 0 {
		t.Fatalf("GetUserByID should not be called separately, calls=%d", st.getUserByIDCalls)
	}
	if !strings.Contains(rec.Body.String(), `"telegram_linked":true`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
