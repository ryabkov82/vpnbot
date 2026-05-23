package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountSessionStaticContainsPremiumHappCopy(t *testing.T) {
	p := filepath.Join("static", "account", "session.html")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	s := string(b)
	if !strings.Contains(s, "Для Premium используйте приложение Happ.") {
		t.Fatal("missing premium Happ copy")
	}
	if !strings.Contains(s, "connect_app") || !strings.Contains(s, "'happ'") {
		t.Fatal("session JS must branch on connect_app")
	}
	if !strings.Contains(s, "Подключить Premium") {
		t.Fatal(`missing Premium connect button label`)
	}
	for _, forbid := range []string{"SHM", "Remnawave", "internal_squad_name"} {
		if strings.Contains(s, forbid) {
			t.Fatalf("session UI leak %q", forbid)
		}
	}
}
