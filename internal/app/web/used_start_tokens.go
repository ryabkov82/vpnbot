package web

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// UsedStartTokenStore хеши уже использованных start-токенов (in-memory, до exp).
type UsedStartTokenStore struct {
	mu sync.Mutex
	m  map[string]int64 // sha256(raw token) hex -> exp unix
}

func NewUsedStartTokenStore() *UsedStartTokenStore {
	return &UsedStartTokenStore{m: map[string]int64{}}
}

func startTokenKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *UsedStartTokenStore) pruneLocked(now int64) {
	for k, exp := range s.m {
		if exp <= now {
			delete(s.m, k)
		}
	}
}

// IsUsed возвращает true, если start-токен уже был успешно обработан.
func (s *UsedStartTokenStore) IsUsed(rawToken string, now int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	_, ok := s.m[startTokenKey(rawToken)]
	return ok
}

// MarkUsed фиксирует успешное создание заказа по start-токену.
func (s *UsedStartTokenStore) MarkUsed(rawToken string, expUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[startTokenKey(rawToken)] = expUnix
}
