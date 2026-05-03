package web

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/models"
)

// shmMoscow — фиксированное смещение UTC+3 для дат SHM (как в интеграции).
var shmMoscow = time.FixedZone("Europe/Moscow", 3*3600)

const shmExpireLayout = "2006-01-02 15:04:05"

func parseSHMExpireMoscow(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty expire")
	}
	return time.ParseInLocation(shmExpireLayout, s, shmMoscow)
}

func parsePeriodMonths(period string) (int, error) {
	p := strings.TrimSpace(strings.ReplaceAll(period, ",", "."))
	if p == "" {
		return 0, fmt.Errorf("empty period")
	}
	f, err := strconv.ParseFloat(p, 64)
	if err != nil {
		return 0, err
	}
	m := int(f + 0.5)
	if m < 1 {
		m = 1
	}
	return m, nil
}

// PremiumBandwidthQueryRange возвращает границы [start, end] в UTC для запроса usage в Remnawave.
// ok=false — не запрашивать usage (неверная стратегия, парсинг, пустой интервал).
func PremiumBandwidthQueryRange(us *models.UserService, trafficLimitStrategy string, now time.Time) (startUTC, endUTC time.Time, ok bool) {
	if us == nil {
		return time.Time{}, time.Time{}, false
	}
	strat := strings.TrimSpace(strings.ToUpper(trafficLimitStrategy))
	if strat != "MONTH" {
		return time.Time{}, time.Time{}, false
	}

	expireLocal, err := parseSHMExpireMoscow(us.Expire)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	months, err := parsePeriodMonths(us.Period)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}

	periodStart := expireLocal.AddDate(0, -months, 0)
	endLocal := now.In(shmMoscow)
	if endLocal.After(expireLocal) {
		endLocal = expireLocal
	}
	if !endLocal.After(periodStart) {
		return time.Time{}, time.Time{}, false
	}

	return periodStart.UTC(), endLocal.UTC(), true
}
