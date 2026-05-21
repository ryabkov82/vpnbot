package service

import (
	"errors"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

type testWebUserRegistrar struct {
	getCalls  int
	firstGet  *models.User
	secondGet *models.User
	regErr    error
	lastReg   *models.UserRegistrationRequest
}

func (m *testWebUserRegistrar) GetUserByLogin(login string) (*models.User, error) {
	m.getCalls++
	if m.getCalls == 1 {
		return m.firstGet, nil
	}
	return m.secondGet, nil
}

func (m *testWebUserRegistrar) RegisterUser(user models.UserRegistrationRequest) error {
	cp := user
	m.lastReg = &cp
	return m.regErr
}

func TestFindOrCreateWebUser_FoundExisting(t *testing.T) {
	login := webuser.WebLoginFromEmail("known@example.com")
	existing := &models.User{ID: 7, Login: login}
	reg := &testWebUserRegistrar{firstGet: existing, secondGet: existing}

	u, err := findOrCreateWebUser(reg, "  Known@Example.COM ")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 7 || u.Login != login {
		t.Fatalf("user %#v", u)
	}
	if reg.getCalls != 1 {
		t.Fatalf("GetUserByLogin calls: %d", reg.getCalls)
	}
	if reg.lastReg != nil {
		t.Fatal("RegisterUser must not be called")
	}
}

func TestFindOrCreateWebUser_CreatesAndReloads(t *testing.T) {
	login := webuser.WebLoginFromEmail("new@example.com")
	created := &models.User{ID: 99, Login: login}
	reg := &testWebUserRegistrar{
		firstGet:  nil,
		secondGet: created,
	}

	u, err := findOrCreateWebUser(reg, "new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 99 {
		t.Fatalf("user %#v", u)
	}
	if reg.getCalls != 2 {
		t.Fatalf("GetUserByLogin calls: %d", reg.getCalls)
	}
	if reg.lastReg == nil {
		t.Fatal("expected RegisterUser call")
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
	_, err := findOrCreateWebUser(reg, "x@y.zz")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestFindOrCreateWebUser_NotFoundAfterRegister(t *testing.T) {
	reg := &testWebUserRegistrar{
		firstGet:  nil,
		secondGet: nil,
	}
	_, err := findOrCreateWebUser(reg, "gone@example.com")
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
