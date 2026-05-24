package web

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sessionHTMLPath(t *testing.T) string {
	t.Helper()
	_, fname, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(fname), "static", "account", "session.html")
}

func TestAccountSessionStaticContainsPremiumHappCopy(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatalf("read session.html: %v", err)
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
	if !strings.Contains(s, "/api/account/session/start") {
		t.Fatal("session must call /api/account/session/start")
	}
	if !strings.Contains(s, "'/api/account/services?token='") {
		t.Fatal("session must fetch /api/account/services with exchanged token")
	}
	if !strings.Contains(s, "function bootFromRawToken") {
		t.Fatal("expected bootFromRawToken bootstrap")
	}
	if !strings.Contains(s, "Ссылка недействительна или устарела.") {
		t.Fatal("missing invalid magic-link message")
	}
	for _, forbid := range []string{"SHM", "Remnawave", "internal_squad_name"} {
		if strings.Contains(s, forbid) {
			t.Fatalf("session UI leak %q", forbid)
		}
	}
}
