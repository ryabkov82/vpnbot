package service

import (
	"errors"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
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

	u, created, err := findOrCreateWebUser(reg, "  Known@Example.COM ")
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

	u, created, err := findOrCreateWebUser(reg, "linked@Example.COM")
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

	u, registered, err := findOrCreateWebUser(reg, "new@example.com")
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
	if reg.lastReg.Settings.Web.Email != "new@example.com" || reg.lastReg.Settings.Web.Source != webUserSource {
		t.Fatalf("web settings: %#v", reg.lastReg.Settings.Web)
	}
}

func TestFindOrCreateWebUser_RegisterError(t *testing.T) {
	reg := &testWebUserRegistrar{
		firstGet: nil,
		regErr:   errors.New("api down"),
	}
	_, _, err := findOrCreateWebUser(reg, "x@y.zz")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestFindOrCreateWebUser_NotFoundAfterRegister(t *testing.T) {
	reg := &testWebUserRegistrar{
		firstGet: nil,
	}
	_, _, err := findOrCreateWebUser(reg, "gone@example.com")
	if err == nil {
		t.Fatal("want error when reload returns nil")
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
