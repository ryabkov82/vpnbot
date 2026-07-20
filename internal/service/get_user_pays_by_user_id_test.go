package service

import (
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func TestGetUserPaysByUserID_InvalidUserID(t *testing.T) {
	s := NewService(nil, config.BrandConfig{})
	_, err := s.GetUserPaysByUserID(0)
	if err == nil || err.Error() != "invalid user id" {
		t.Fatalf("want invalid user id, got %v", err)
	}
	_, err = s.GetUserPaysByUserID(-5)
	if err == nil || err.Error() != "invalid user id" {
		t.Fatalf("want invalid user id, got %v", err)
	}
}
