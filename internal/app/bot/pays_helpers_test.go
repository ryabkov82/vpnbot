package bot

import (
	"testing"
)

func TestFormatRubAmount(t *testing.T) {
	tab := []struct {
		v    float64
		want string
	}{
		{150, "150 ₽"},
		{150.5, "150.50 ₽"},
		{-1318.74, "-1318.74 ₽"},
		{0, "0 ₽"},
	}
	for _, tc := range tab {
		if got := formatRubAmount(tc.v); got != tc.want {
			t.Errorf("formatRubAmount(%v)=%q want %q", tc.v, got, tc.want)
		}
	}
}
