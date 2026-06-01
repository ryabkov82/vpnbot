package web

import (
	"os"
	"strings"
	"testing"
)

func accountLinkStartHTMLPath(t *testing.T) string {
	t.Helper()
	return "static/account/link_start.html"
}

func TestAccountLinkStart_ErrReplaceStateKeepsToken(t *testing.T) {
	b, err := os.ReadFile(accountLinkStartHTMLPath(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "searchParams.delete('err')") {
		t.Fatal("must delete only err query param")
	}
	if strings.Contains(s, "replaceState({}, document.title, '/account')") {
		t.Fatal("link_start must not replace URL with /account")
	}
	if !strings.Contains(s, "u.pathname + u.search + u.hash") {
		t.Fatal("replaceState must preserve pathname and remaining query (token)")
	}
	if !strings.Contains(s, "case 'email_already_linked':") {
		t.Fatal("must handle err=email_already_linked from OAuth redirect")
	}
}
