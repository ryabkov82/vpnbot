package service

import (
	"testing"
)

func TestGetUserPaysByUserID_InvalidUserID(t *testing.T) {
	s := NewService(nil)
	_, err := s.GetUserPaysByUserID(0)
	if err == nil || err.Error() != "invalid user id" {
		t.Fatalf("want invalid user id, got %v", err)
	}
	_, err = s.GetUserPaysByUserID(-5)
	if err == nil || err.Error() != "invalid user id" {
		t.Fatalf("want invalid user id, got %v", err)
	}
}
