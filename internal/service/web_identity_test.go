package service

import (
	"errors"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// testServiceBrand — явный brand для service-тестов web identity.
// Service layer больше не синтезирует defaults; тесты передают нужные поля явно.
func testServiceBrand() config.BrandConfig {
	// ID=vff: legacy empty brand_id допустим для web membership в сервисных тестах.
	return config.BrandConfig{
		ID:                 "vff",
		Name:               "Test Brand",
		AllowedHosts:       []string{"test.example.com"},
		PublicBaseURL:      "https://test.example.com",
		LandingURL:         "https://landing.test.example.com",
		ServiceCategory:    "vpn-test",
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
		PaymentProfile:     "test_payment",
	}
}

// 9.1 nil service — без defaults.
func TestWebIdentity_NilService(t *testing.T) {
	var s *Service
	if got := s.webLoginPrefix(); got != "" {
		t.Fatalf("nil webLoginPrefix=%q, want empty", got)
	}
	if got := s.webUserSource(); got != "" {
		t.Fatalf("nil webUserSource=%q, want empty", got)
	}
}

// 9.2 empty brand — без VFF defaults.
func TestWebIdentity_EmptyBrand(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	if got := s.webLoginPrefix(); got != "" {
		t.Fatalf("empty webLoginPrefix=%q, want empty", got)
	}
	if got := s.webUserSource(); got != "" {
		t.Fatalf("empty webUserSource=%q, want empty", got)
	}
}

// 9.3 explicit brand — значения из brand (с trim).
func TestWebIdentity_ExplicitBrandTrimmed(t *testing.T) {
	s := NewService(nil, config.BrandConfig{
		WebUserLoginPrefix: " customer_ ",
		WebUserSource:      " example.com ",
	})
	if got := s.webLoginPrefix(); got != "customer_" {
		t.Fatalf("webLoginPrefix=%q, want customer_", got)
	}
	if got := s.webUserSource(); got != "example.com" {
		t.Fatalf("webUserSource=%q, want example.com", got)
	}
}

// 9.4 VFF-значения появляются только если переданы явно.
func TestWebIdentity_VFFValuesOnlyWhenExplicit(t *testing.T) {
	s := NewService(nil, config.BrandConfig{
		WebUserLoginPrefix: "web_",
		WebUserSource:      "vpn-for-friends.com",
	})
	if got := s.webLoginPrefix(); got != "web_" {
		t.Fatalf("webLoginPrefix=%q, want web_", got)
	}
	if got := s.webUserSource(); got != "vpn-for-friends.com" {
		t.Fatalf("webUserSource=%q, want vpn-for-friends.com", got)
	}
}

// Empty BrandConfig: web-user operations fail before SHM side effects.
func TestEmptyBrand_FindOrCreateWebUser_NoRegister(t *testing.T) {
	reg := &testWebUserRegistrar{}
	_, _, err := findOrCreateWebUser(reg, "u@example.com", "", "vpn-for-friends.com", "vff")
	if !errors.Is(err, webuser.ErrWebLoginPrefixRequired) {
		t.Fatalf("want ErrWebLoginPrefixRequired, got %v", err)
	}
	if reg.getCalls != 0 || reg.login2Calls != 0 || reg.lastReg != nil {
		t.Fatalf("no SHM side effects: get=%d login2=%d reg=%v", reg.getCalls, reg.login2Calls, reg.lastReg)
	}
}

func TestEmptyBrand_FindOrCreateWebUser_EmptySourceNoRegister(t *testing.T) {
	reg := &testWebUserRegistrar{}
	_, _, err := findOrCreateWebUser(reg, "u@example.com", "web_", "", "vff")
	if !errors.Is(err, ErrWebUserSourceRequired) {
		t.Fatalf("want ErrWebUserSourceRequired, got %v", err)
	}
	if reg.getCalls != 0 || reg.lastReg != nil {
		t.Fatalf("no SHM side effects: get=%d reg=%v", reg.getCalls, reg.lastReg)
	}
}

func TestEmptyBrand_FindUserByWebEmail_NoAPI(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	_, err := s.FindUserByWebEmail("u@example.com")
	if !errors.Is(err, webuser.ErrWebLoginPrefixRequired) {
		t.Fatalf("want ErrWebLoginPrefixRequired, got %v", err)
	}
}

func TestEmptyBrand_LinkWebEmail_NoUpdate(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	_, err := s.LinkWebEmailForTelegramUser(42, 9001, "u@example.com", "telegram_link")
	if !errors.Is(err, ErrActiveBrandIDRequired) {
		t.Fatalf("want ErrActiveBrandIDRequired, got %v", err)
	}
}

func TestExplicitBrand_FindOrCreateWebUser_StillWorks(t *testing.T) {
	login := webuser.WebLoginFromEmail("new@example.com")
	reg := &testWebUserRegistrar{
		firstGet:       nil,
		secondAndLater: &models.User{ID: 11, Login: login},
	}
	u, created, err := findOrCreateWebUser(reg, "new@example.com", "web_", "vpn-for-friends.com", "vff")
	if err != nil || !created || u == nil || u.ID != 11 {
		t.Fatalf("u=%v created=%v err=%v", u, created, err)
	}
	if reg.lastReg == nil || reg.lastReg.Login != login {
		t.Fatalf("RegisterUser: %#v", reg.lastReg)
	}
}
