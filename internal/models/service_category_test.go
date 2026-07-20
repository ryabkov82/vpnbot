package models

import "testing"

func TestServiceCategoryAllowed(t *testing.T) {
	cases := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{"empty expected allows anything (legacy)", "", "vpn-mz-other", true},
		{"empty expected allows empty", "", "", true},
		{"whitespace expected is legacy", "   ", "vpn-mz-other", true},
		{"exact match", "vpn-mz-main", "vpn-mz-main", true},
		{"actual with spaces trimmed", "vpn-mz-main", "  vpn-mz-main  ", true},
		{"other category denied", "vpn-mz-main", "vpn-mz-other", false},
		{"prefix is not enough", "vpn-mz-", "vpn-mz-main", false},
		{"empty actual denied when expected set", "vpn-mz-main", "", false},
		{"case sensitive", "vpn-mz-main", "VPN-MZ-MAIN", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ServiceCategoryAllowed(tc.expected, tc.actual); got != tc.want {
				t.Fatalf("ServiceCategoryAllowed(%q, %q) = %v, want %v", tc.expected, tc.actual, got, tc.want)
			}
		})
	}
}
