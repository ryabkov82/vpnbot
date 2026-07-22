package service

import (
	"errors"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func TestWebUserBelongsToBrand_VFF(t *testing.T) {
	em := "legacy@example.com"
	login := webuser.WebLoginFromEmail(em)

	cases := []struct {
		name string
		u    *models.User
		want bool
	}{
		{"nil", nil, false},
		{"brand_vff", &models.User{Login: login, Settings: models.UserSettings{BrandID: "vff"}}, true},
		{"legacy_empty", &models.User{Login: login}, true},
		{"legacy_login2", &models.User{Login: "@1", Login2: login}, true},
		{"fc_rejected", &models.User{Login: login, Settings: models.UserSettings{BrandID: "fc"}}, false},
		{"wrong_login", &models.User{Login: "web_other", Settings: models.UserSettings{BrandID: "vff"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := webUserBelongsToBrand(tc.u, "vff", login); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestWebUserBelongsToBrand_FC(t *testing.T) {
	em := "fcuser@example.com"
	login := webuser.WebLoginFromEmail(em) // transitional shared prefix web_

	cases := []struct {
		name string
		u    *models.User
		want bool
	}{
		{"brand_fc", &models.User{Login: login, Settings: models.UserSettings{BrandID: "fc"}}, true},
		{"login2_fc", &models.User{Login: "@fc_9", Login2: login, Settings: models.UserSettings{BrandID: "fc"}}, true},
		{"empty_rejected", &models.User{Login: login}, false},
		{"vff_rejected", &models.User{Login: login, Settings: models.UserSettings{BrandID: "vff"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := webUserBelongsToBrand(tc.u, "fc", login); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestFindOrCreateWebUser_WritesBrandID(t *testing.T) {
	em := "newbrand@example.com"
	login := webuser.WebLoginFromEmail(em)
	reg := &testWebUserRegistrar{
		secondAndLater: &models.User{ID: 3, Login: login, Settings: models.UserSettings{BrandID: "vff"}},
	}
	_, created, err := findOrCreateWebUser(reg, em, "web_", "vpn-for-friends.com", "vff")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if reg.lastReg == nil || reg.lastReg.Settings.BrandID != "vff" {
		t.Fatalf("BrandID=%#v", reg.lastReg)
	}
}

func TestFindOrCreateWebUser_FCWritesBrandID(t *testing.T) {
	em := "fcnew@example.com"
	login := webuser.WebLoginFromEmail(em)
	reg := &testWebUserRegistrar{
		secondAndLater: &models.User{ID: 4, Login: login, Settings: models.UserSettings{BrandID: "fc"}},
	}
	_, created, err := findOrCreateWebUser(reg, em, "web_", "vpn-for-friends.com", "fc")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if reg.lastReg.Settings.BrandID != "fc" {
		t.Fatalf("BrandID=%q", reg.lastReg.Settings.BrandID)
	}
}

func TestFindOrCreateWebUser_SharedPrefixOtherBrand_MismatchNoRegister(t *testing.T) {
	em := "shared@example.com"
	login := webuser.WebLoginFromEmail(em)

	// VFF user exists; FC runtime with same prefix must mismatch and not register.
	regFC := &testWebUserRegistrar{
		firstGet: &models.User{ID: 10, Login: login, Settings: models.UserSettings{BrandID: "vff"}},
	}
	_, _, err := findOrCreateWebUser(regFC, em, "web_", "vpn-for-friends.com", "fc")
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("want ErrUserIdentityMismatch, got %v", err)
	}
	if regFC.lastReg != nil {
		t.Fatal("RegisterUser must not be called")
	}

	// FC user exists; VFF runtime must mismatch.
	regVFF := &testWebUserRegistrar{
		firstGet: &models.User{ID: 11, Login: login, Settings: models.UserSettings{BrandID: "fc"}},
	}
	_, _, err = findOrCreateWebUser(regVFF, em, "web_", "vpn-for-friends.com", "vff")
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("want ErrUserIdentityMismatch, got %v", err)
	}
	if regVFF.lastReg != nil {
		t.Fatal("RegisterUser must not be called")
	}
}

func TestFindUserByWebLoginKeys_LegacyVFFAccepted(t *testing.T) {
	em := "old@example.com"
	login := webuser.WebLoginFromEmail(em)
	reg := &testWebUserRegistrar{firstGet: &models.User{ID: 1, Login: login}}
	u, err := findUserByWebLoginKeys(reg, em, "web_", "vff")
	if err != nil || u == nil || u.ID != 1 {
		t.Fatalf("u=%v err=%v", u, err)
	}
}

func TestFindUserByWebLoginKeys_FCRequiresBrandID(t *testing.T) {
	em := "x@y.zz"
	login := webuser.WebLoginFromEmail(em)
	reg := &testWebUserRegistrar{firstGet: &models.User{ID: 1, Login: login}}
	_, err := findUserByWebLoginKeys(reg, em, "web_", "fc")
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("want mismatch, got %v", err)
	}
}

func TestSyntheticFCTelegramLinkedWeb_SharedPrefix(t *testing.T) {
	// Модель существующего FC Telegram user с login2=web_<hash> при active prefix=web_.
	em := "synthetic@example.com"
	norm, err := webuser.NormalizeEmail(em)
	if err != nil {
		t.Fatal(err)
	}
	wLogin := webuser.WebLoginFromEmail(norm)
	const chatID int64 = 100200300
	u := &models.User{
		ID:     9001,
		Login:  telegramSHMLogin("fc", chatID),
		Login2: wLogin,
		Settings: models.UserSettings{
			BrandID:  "fc",
			Telegram: models.TelegramInfo{ChatID: chatID},
			Web:      models.WebInfo{Email: norm, Source: "telegram_link"},
		},
	}
	if !webUserBelongsToBrand(u, "fc", wLogin) {
		t.Fatal("FC linked user must belong with shared prefix")
	}
	reg := &testWebUserRegistrar{firstGet: nil, login2User: u}
	got, err := findUserByWebLoginKeys(reg, norm, "web_", "fc")
	if err != nil || got == nil || got.ID != u.ID {
		t.Fatalf("FindUserByWebEmail path: got=%v err=%v", got, err)
	}
	_, created, err := findOrCreateWebUser(reg, norm, "web_", "vpn-for-friends.com", "fc")
	if err != nil || created {
		t.Fatalf("FindOrCreate must reuse, created=%v err=%v", created, err)
	}
}

func TestEmptyBrand_FindOrCreateWebUser_BrandIDRequired(t *testing.T) {
	reg := &testWebUserRegistrar{}
	_, _, err := findOrCreateWebUser(reg, "u@example.com", "web_", "vpn-for-friends.com", "")
	if !errors.Is(err, ErrActiveBrandIDRequired) {
		t.Fatalf("want ErrActiveBrandIDRequired, got %v", err)
	}
	if reg.lastReg != nil {
		t.Fatal("no register")
	}
}
