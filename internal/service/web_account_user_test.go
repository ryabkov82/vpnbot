package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func TestValidateWebAccountUser_OK_LegacyVFF(t *testing.T) {
	em := "cabinet@example.com"
	norm, _ := webuser.NormalizeEmail(em)
	login := webuser.WebLoginFromEmail(norm)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string][]models.User{"data": {{
			ID:    5,
			Login: login,
			Settings: models.UserSettings{
				Web: models.WebInfo{Email: norm},
			},
		}}})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	u, err := svc.ValidateWebAccountUser(5, login, norm)
	if err != nil || u == nil || u.ID != 5 {
		t.Fatalf("u=%v err=%v", u, err)
	}
}

func TestValidateWebAccountUser_FCLinkedLogin2(t *testing.T) {
	em := "synthetic@example.com"
	norm, _ := webuser.NormalizeEmail(em)
	wLogin := webuser.WebLoginFromEmail(norm)
	const chatID int64 = 100200300
	tgLogin := telegramSHMLogin("fc", chatID)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string][]models.User{"data": {{
			ID:     9001,
			Login:  tgLogin,
			Login2: wLogin,
			Settings: models.UserSettings{
				BrandID:  "fc",
				Telegram: models.TelegramInfo{ChatID: chatID},
				Web:      models.WebInfo{Email: norm},
			},
		}}})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("fc"))
	u, err := svc.ValidateWebAccountUser(9001, tgLogin, norm)
	if err != nil || u == nil {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateWebAccountUser_WrongBrand(t *testing.T) {
	em := "x@y.zz"
	norm, _ := webuser.NormalizeEmail(em)
	login := webuser.WebLoginFromEmail(norm)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string][]models.User{"data": {{
			ID:    3,
			Login: login,
			Settings: models.UserSettings{
				BrandID: "fc",
				Web:     models.WebInfo{Email: norm},
			},
		}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	_, err := svc.ValidateWebAccountUser(3, login, norm)
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateWebAccountUser_LoginMismatch(t *testing.T) {
	em := "x@y.zz"
	norm, _ := webuser.NormalizeEmail(em)
	login := webuser.WebLoginFromEmail(norm)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string][]models.User{"data": {{
			ID:       3,
			Login:    login,
			Settings: models.UserSettings{BrandID: "vff", Web: models.WebInfo{Email: norm}},
		}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	_, err := svc.ValidateWebAccountUser(3, "other_login", norm)
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %v", err)
	}
}
