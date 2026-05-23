package service

import (
	"testing"
)

func TestGetUserBalanceByUserID_InvalidID(t *testing.T) {
	s := NewService(nil)
	_, err := s.GetUserBalanceByUserID(0)
	if err == nil || err.Error() != "invalid user id" {
		t.Fatalf("want invalid user id, got %v", err)
	}
	_, err = s.GetUserBalanceByUserID(-5)
	if err == nil || err.Error() != "invalid user id" {
		t.Fatalf("want invalid user id, got %v", err)
	}
}
