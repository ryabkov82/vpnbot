package web

import (
	"os"
	"strings"
	"testing"
)

func TestAccountIndexStatic_CapturesAttributionBeforeReplaceState(t *testing.T) {
	b, err := os.ReadFile(accountIndexHTMLPath(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Single script IIFE: capture must appear before replaceState.
	attrIdx := strings.Index(s, "var attrParams = new URLSearchParams(window.location.search);")
	if attrIdx < 0 {
		t.Fatal("attribution snapshot from initial location.search missing")
	}
	pathIdx := strings.Index(s, "var attrLandingPath = window.location.pathname")
	if pathIdx < 0 || pathIdx < attrIdx {
		t.Fatal("landing_path capture from pathname missing or out of order")
	}
	refIdx := strings.Index(s, "var attrReferrer = document.referrer")
	if refIdx < 0 || refIdx < pathIdx {
		t.Fatal("referrer capture missing or out of order")
	}
	for _, utm := range []string{
		`attrParams.get('utm_source')`,
		`attrParams.get('utm_medium')`,
		`attrParams.get('utm_campaign')`,
		`attrParams.get('utm_content')`,
		`attrParams.get('utm_term')`,
	} {
		if !strings.Contains(s, utm) {
			t.Fatalf("missing UTM extract: %s", utm)
		}
	}
	replaceIdx := strings.Index(s, "window.history.replaceState")
	if replaceIdx < 0 || replaceIdx < refIdx {
		t.Fatal("replaceState must run after attribution snapshot")
	}

	// JSON body fields for login/start.
	for _, field := range []string{
		"landing_path: attrLandingPath",
		"referrer: attrReferrer",
		"utm_source: attrUTMSource",
		"utm_medium: attrUTMMedium",
		"utm_campaign: attrUTMCampaign",
		"utm_content: attrUTMContent",
		"utm_term: attrUTMTerm",
	} {
		if !strings.Contains(s, field) {
			t.Fatalf("login/start body missing %q", field)
		}
	}

	banned := []string{
		"localStorage",
		"sessionStorage",
		"document.cookie",
		"brand_id",
		"captured_at",
		"registration_domain",
		"registration_channel",
		"window.location.href",
		"telegram_start_param",
	}
	for _, b := range banned {
		if strings.Contains(s, b) {
			t.Fatalf("account index must not use/send %q", b)
		}
	}
}
