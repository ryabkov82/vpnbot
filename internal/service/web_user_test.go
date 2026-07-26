package service

import (
	"errors"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func sampleWebMagicLinkAttribution(t *testing.T, domain string) attribution.Record {
	t.Helper()
	if domain == "" {
		domain = "connect.vpn-for-friends.com"
	}
	rec, err := attribution.NewFirstTouch(attribution.ServerContext{
		RegistrationChannel: attribution.RegistrationChannelWebMagicLink,
		RegistrationDomain:  domain,
		CapturedAt:          time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
	}, attribution.MarketingInput{
		LandingPath: "/account",
		Referrer:    "https://vpn-for-friends.com/x",
		UTMSource:   "telegram",
		UTMMedium:   "post",
		UTMCampaign: "summer",
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

const (
	testWebLoginPrefix = "web_"
	testWebUserSource  = "vpn-for-friends.com"
)

type testWebUserRegistrar struct {
	login2User *models.User

	getCalls       int          // GetUserByLogin
	login2Calls    int          // GetUserByLogin2
	firstGet       *models.User // call 1
	secondAndLater *models.User // reload / later calls by GetUserByLogin
	regErr         error
	lastReg        *models.UserRegistrationRequest
}

func (m *testWebUserRegistrar) GetUserByLogin(login string) (*models.User, error) {
	m.getCalls++
	if m.getCalls == 1 {
		return m.firstGet, nil
	}
	return m.secondAndLater, nil
}

func (m *testWebUserRegistrar) GetUserByLogin2(login2 string) (*models.User, error) {
	m.login2Calls++
	return m.login2User, nil
}

func (m *testWebUserRegistrar) RegisterUser(user models.UserRegistrationRequest) error {
	cp := user
	m.lastReg = &cp
	return m.regErr
}

func TestFindOrCreateWebUser_FoundExistingPrimaryLogin(t *testing.T) {
	login := webuser.WebLoginFromEmail("known@example.com")
	existing := &models.User{ID: 7, Login: login}
	reg := &testWebUserRegistrar{firstGet: existing, secondAndLater: existing}

	u, created, err := findOrCreateWebUser(reg, "  Known@Example.COM ", testWebLoginPrefix, testWebUserSource, "vff")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 7 || u.Login != login {
		t.Fatalf("user %#v", u)
	}
	if reg.getCalls != 1 {
		t.Fatalf("GetUserByLogin calls: %d", reg.getCalls)
	}
	if reg.login2Calls != 0 {
		t.Fatalf("GetUserByLogin2 must not run: %d", reg.login2Calls)
	}
	if created {
		t.Fatal("want created=false")
	}
	if reg.lastReg != nil {
		t.Fatal("RegisterUser must not be called")
	}
}

// Сценарий «обычного» Google/email входа после привязки: пользователь уже найден по login2 (Telegram-профиль), новый SHM-only web-user не создаётся.
func TestFindOrCreateWebUser_FoundViaLogin2(t *testing.T) {
	wl := webuser.WebLoginFromEmail("linked@Example.COM")
	linked := &models.User{ID: 140, Login: "@tg", Login2: wl}
	reg := &testWebUserRegistrar{
		firstGet:   nil,
		login2User: linked,
	}

	u, created, err := findOrCreateWebUser(reg, "linked@Example.COM", testWebLoginPrefix, testWebUserSource, "vff")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != linked.ID || u.Login != linked.Login || created {
		t.Fatalf("want telegram row %+v created=%v", u, created)
	}
	if reg.getCalls != 1 {
		t.Fatalf("GetUserByLogin calls: %d", reg.getCalls)
	}
	if reg.login2Calls != 1 {
		t.Fatalf("GetUserByLogin2 calls: want 1 got %d", reg.login2Calls)
	}
	if reg.lastReg != nil {
		t.Fatal("RegisterUser must not run")
	}
}

func TestFindOrCreateWebUser_CreatesAndReloads(t *testing.T) {
	login := webuser.WebLoginFromEmail("new@example.com")
	newUser := &models.User{ID: 99, Login: login}
	reg := &testWebUserRegistrar{
		firstGet:       nil,
		login2User:     nil,
		secondAndLater: newUser,
	}

	u, registered, err := findOrCreateWebUser(reg, "new@example.com", testWebLoginPrefix, testWebUserSource, "vff")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 99 {
		t.Fatalf("user %#v", u)
	}
	if reg.getCalls != 2 {
		t.Fatalf("GetUserByLogin calls: %d", reg.getCalls)
	}
	if reg.login2Calls != 1 {
		t.Fatalf("GetUserByLogin2 during discovery: %d", reg.login2Calls)
	}
	if !registered {
		t.Fatal("want registered=true")
	}
	if reg.lastReg.Login != login {
		t.Fatalf("login: %q", reg.lastReg.Login)
	}
	if reg.lastReg.FullName != "new@example.com" {
		t.Fatalf("full_name: %q", reg.lastReg.FullName)
	}
	if reg.lastReg.Password == "" {
		t.Fatal("expected non-empty password")
	}
	if reg.lastReg.Settings.Web.Email != "new@example.com" || reg.lastReg.Settings.Web.Source != testWebUserSource {
		t.Fatalf("web settings: %#v", reg.lastReg.Settings.Web)
	}
	if reg.lastReg.Settings.BrandID != "vff" {
		t.Fatalf("brand_id: %q", reg.lastReg.Settings.BrandID)
	}
	if reg.lastReg.Settings.Attribution != nil {
		t.Fatalf("plain FindOrCreateWebUser must not set attribution: %#v", reg.lastReg.Settings.Attribution)
	}
}

func TestFindOrCreateWebUserWithAttribution_CreatesWithExactRecord(t *testing.T) {
	login := webuser.WebLoginFromEmail("attr@example.com")
	newUser := &models.User{ID: 42, Login: login}
	reg := &testWebUserRegistrar{secondAndLater: newUser}
	rec := sampleWebMagicLinkAttribution(t, "connect.vpn-for-friends.com")

	u, created, err := findOrCreateWebUserWithAttribution(reg, "attr@example.com", testWebLoginPrefix, testWebUserSource, "vff", rec)
	if err != nil {
		t.Fatal(err)
	}
	if !created || u.ID != 42 {
		t.Fatalf("created=%v user=%#v", created, u)
	}
	if reg.lastReg == nil || reg.lastReg.Settings.Attribution == nil {
		t.Fatal("Settings.Attribution required")
	}
	if !attribution.Equal(*reg.lastReg.Settings.Attribution, rec) {
		t.Fatalf("attribution %#v want %#v", reg.lastReg.Settings.Attribution, rec)
	}
	if reg.lastReg.Settings.BrandID != "vff" {
		t.Fatalf("brand_id %q", reg.lastReg.Settings.BrandID)
	}
	if reg.lastReg.Settings.Web.Email != "attr@example.com" || reg.lastReg.Settings.Web.Source != testWebUserSource {
		t.Fatalf("web %#v", reg.lastReg.Settings.Web)
	}
	// Pointer must be a copy, not the caller's address.
	if reg.lastReg.Settings.Attribution == &rec {
		t.Fatal("must store a copy, not caller pointer")
	}
}

func TestFindOrCreateWebUserWithAttribution_FCIdentity(t *testing.T) {
	em := "fc-attr@example.com"
	fcLogin, err := webuser.WebLoginFromEmailWithPrefix(em, "web_fc_")
	if err != nil {
		t.Fatal(err)
	}
	reg := &testWebUserRegistrar{secondAndLater: &models.User{ID: 8, Login: fcLogin}}
	rec := sampleWebMagicLinkAttribution(t, "connect.friends-connect.club")
	_, created, err := findOrCreateWebUserWithAttribution(reg, em, "web_fc_", "friends-connect.club", "fc", rec)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if reg.lastReg.Settings.BrandID != "fc" || reg.lastReg.Settings.Web.Source != "friends-connect.club" {
		t.Fatalf("fc identity %#v", reg.lastReg.Settings)
	}
	if !attribution.Equal(*reg.lastReg.Settings.Attribution, rec) {
		t.Fatal("fc attribution mismatch")
	}
}

func TestFindOrCreateWebUserWithAttribution_InvalidRecord(t *testing.T) {
	reg := &testWebUserRegistrar{}
	_, created, err := findOrCreateWebUserWithAttribution(reg, "x@y.zz", testWebLoginPrefix, testWebUserSource, "vff", attribution.Record{})
	if !errors.Is(err, ErrAttributionRequired) || created {
		t.Fatalf("want ErrAttributionRequired, got created=%v err=%v", created, err)
	}
	if reg.getCalls != 0 || reg.login2Calls != 0 || reg.lastReg != nil {
		t.Fatal("lookup/register must not run for invalid attribution")
	}
}

func TestFindOrCreateWebUserWithAttribution_ExistingUserUnchanged(t *testing.T) {
	login := webuser.WebLoginFromEmail("known-attr@example.com")
	existingRec := sampleWebMagicLinkAttribution(t, "connect.vpn-for-friends.com")
	existing := &models.User{
		ID:    7,
		Login: login,
		Settings: models.UserSettings{
			BrandID:     "vff",
			Attribution: &existingRec,
		},
	}
	reg := &testWebUserRegistrar{firstGet: existing}
	newRec := sampleWebMagicLinkAttribution(t, "connect.friends-connect.club")
	newRec.FirstTouch.UTMSource = "other"

	u, created, err := findOrCreateWebUserWithAttribution(reg, "known-attr@example.com", testWebLoginPrefix, testWebUserSource, "vff", newRec)
	if err != nil {
		t.Fatal(err)
	}
	if created || u.ID != 7 {
		t.Fatalf("want existing user created=false, got created=%v id=%d", created, u.ID)
	}
	if reg.lastReg != nil {
		t.Fatal("RegisterUser must not run")
	}
	if u.Settings.Attribution == nil || !attribution.Equal(*u.Settings.Attribution, existingRec) {
		t.Fatalf("existing attribution must stay: %#v", u.Settings.Attribution)
	}
}

func TestFindOrCreateWebUser_RegisterError(t *testing.T) {
	reg := &testWebUserRegistrar{
		firstGet: nil,
		regErr:   errors.New("api down"),
	}
	_, _, err := findOrCreateWebUser(reg, "x@y.zz", testWebLoginPrefix, testWebUserSource, "vff")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestFindOrCreateWebUser_NotFoundAfterRegister(t *testing.T) {
	reg := &testWebUserRegistrar{
		firstGet: nil,
	}
	_, _, err := findOrCreateWebUser(reg, "gone@example.com", testWebLoginPrefix, testWebUserSource, "vff")
	if err == nil {
		t.Fatal("want error when reload returns nil")
	}
}

func TestFindOrCreateWebUser_ExplicitOtherPrefix(t *testing.T) {
	em := "fc@example.com"
	fcLogin, err := webuser.WebLoginFromEmailWithPrefix(em, "web_fc_")
	if err != nil {
		t.Fatal(err)
	}
	vffLogin := webuser.WebLoginFromEmail(em)
	if fcLogin == vffLogin {
		t.Fatal("fc and vff logins must differ")
	}
	reg := &testWebUserRegistrar{
		firstGet:       nil,
		secondAndLater: &models.User{ID: 55, Login: fcLogin},
	}
	u, created, err := findOrCreateWebUser(reg, em, "web_fc_", "friends-connect.club", "fc")
	if err != nil {
		t.Fatal(err)
	}
	if !created || u.Login != fcLogin {
		t.Fatalf("want created with fc login, got created=%v login=%q", created, u.Login)
	}
	if reg.lastReg == nil || reg.lastReg.Login != fcLogin {
		t.Fatalf("RegisterUser login: %#v", reg.lastReg)
	}
	if reg.lastReg.Settings.Web.Source != "friends-connect.club" {
		t.Fatalf("source: %q", reg.lastReg.Settings.Web.Source)
	}
	if reg.getCalls < 1 {
		t.Fatal("expected GetUserByLogin with fc prefix")
	}
}

func TestServiceOrderByUserID_Validation(t *testing.T) {
	s := &Service{}
	if _, err := s.ServiceOrderByUserID(0, 1); err == nil || err.Error() != "invalid user id" {
		t.Fatalf("want invalid user id, got %v", err)
	}
	if _, err := s.ServiceOrderByUserID(1, 0); err == nil || err.Error() != "invalid service id" {
		t.Fatalf("want invalid service id, got %v", err)
	}
}
