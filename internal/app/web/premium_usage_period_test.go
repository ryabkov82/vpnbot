package web

import (
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestPremiumBandwidthQueryRangeExample(t *testing.T) {
	us := &models.UserService{
		Expire: "2026-05-19 22:00:00",
		Period: "1.0000",
	}
	expireLocal, err := parseSHMExpireMoscow(us.Expire)
	if err != nil {
		t.Fatal(err)
	}
	wantExpireUTC := time.Date(2026, 5, 19, 19, 0, 0, 0, time.UTC)
	if !expireLocal.UTC().Equal(wantExpireUTC) {
		t.Fatalf("expire UTC %v want %v", expireLocal.UTC(), wantExpireUTC)
	}

	now := time.Date(2026, 5, 15, 12, 0, 0, 0, shmMoscow)
	start, end, ok := PremiumBandwidthQueryRange(us, "MONTH", now)
	if !ok {
		t.Fatal("expected ok")
	}
	wantStart := time.Date(2026, 4, 19, 19, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Fatalf("start=%v want %v", start, wantStart)
	}
	wantEnd := now.UTC()
	if !end.Equal(wantEnd) {
		t.Fatalf("end=%v want %v", end, wantEnd)
	}
}

func TestPremiumBandwidthQueryRangeNonMonth(t *testing.T) {
	us := &models.UserService{Expire: "2026-05-19 22:00:00", Period: "1.0000"}
	_, _, ok := PremiumBandwidthQueryRange(us, "DAY", time.Now())
	if ok {
		t.Fatal("expected !ok for DAY")
	}
}

func TestPremiumBandwidthQueryRangeEndCappedToExpire(t *testing.T) {
	us := &models.UserService{
		Expire: "2026-05-19 22:00:00",
		Period: "1.0000",
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, shmMoscow)
	_, end, ok := PremiumBandwidthQueryRange(us, "MONTH", now)
	if !ok {
		t.Fatal("expected ok")
	}
	wantEnd := time.Date(2026, 5, 19, 19, 0, 0, 0, time.UTC)
	if !end.Equal(wantEnd) {
		t.Fatalf("end capped to expire UTC: got %v want %v", end, wantEnd)
	}
}

func TestPremiumBandwidthQueryRangeEmptyWindow(t *testing.T) {
	us := &models.UserService{
		Expire: "2026-05-19 22:00:00",
		Period: "1.0000",
	}
	now := time.Date(2026, 4, 10, 0, 0, 0, 0, shmMoscow)
	_, _, ok := PremiumBandwidthQueryRange(us, "MONTH", now)
	if ok {
		t.Fatal("expected !ok when now before period start (end before start after cap)")
	}
}
