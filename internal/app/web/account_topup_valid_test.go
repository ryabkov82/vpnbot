package web

import "testing"

func TestAccountTopupAmountValid(t *testing.T) {
	cases := []struct {
		v     float64
		valid bool
	}{
		{49.99, false},
		{50, true},
		{150, true},
		{150.5, true},
		{9999.99, true},
		{10000, true},
		{10000.01, false},
		{150.001, false},
	}
	for _, c := range cases {
		got := accountTopupAmountValid(c.v)
		if got != c.valid {
			t.Fatalf("%v → %v want %v", c.v, got, c.valid)
		}
	}
}
