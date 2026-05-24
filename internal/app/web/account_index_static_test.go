package web

import (
	"os"
	"strings"
	"testing"
)

func TestAccountIndexMagicLinkSuccessCopy(t *testing.T) {
	b, err := os.ReadFile(accountIndexHTMLPath(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "Если такой email есть в системе, мы отправили ссылку для входа.") {
		t.Fatal("must not leak old ambiguous copy")
	}
	if !strings.Contains(s, "Мы отправили ссылку для входа на указанный email. Откройте письмо и перейдите по ссылке.") {
		t.Fatal("missing success copy for magic-link")
	}
	if !strings.Contains(s, "Если письма нет, проверьте папку «Спам» или попробуйте еще раз через пару минут.") {
		t.Fatal("missing spam/minutes hint")
	}
}
