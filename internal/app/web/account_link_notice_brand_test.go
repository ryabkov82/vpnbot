package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestStandaloneLinkNoticePage_VFF(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.Name = "VPN for Friends"
	body, err := standaloneLinkNoticePage(cfg, "Привязка кабинета", "Текст ошибки.", "", "  ", "Второй абзац.")
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if !strings.Contains(html, "<title>Привязка кабинета — VPN for Friends</title>") {
		t.Fatalf("title missing: %s", html[:min(250, len(html))])
	}
	if strings.Contains(html, "— Friends Connect") {
		t.Fatal("VFF notice must not contain Friends Connect")
	}
	if !strings.Contains(html, `<h1 class="h4 mb-3">Привязка кабинета</h1>`) {
		t.Fatal("H1 missing")
	}
	if !strings.Contains(html, "Текст ошибки.") || !strings.Contains(html, "Второй абзац.") {
		t.Fatal("paragraphs missing")
	}
	if strings.Count(html, `<p class="text-secondary mb-3">`) != 2 {
		t.Fatal("empty paragraphs must be skipped")
	}
	if !strings.Contains(html, `href="/account"`) {
		t.Fatal("missing /account link")
	}
}

func TestStandaloneLinkNoticePage_FriendsConnect(t *testing.T) {
	cfg := friendsConnectAccountTestCfg()
	cfg.Brand.Name = "Friends Connect"
	body, err := standaloneLinkNoticePage(cfg, "Привязка кабинета", "Текст ошибки.")
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if !strings.Contains(html, "<title>Привязка кабинета — Friends Connect</title>") {
		t.Fatalf("title missing: %s", html[:min(250, len(html))])
	}
	if strings.Contains(html, "— VPN for Friends") {
		t.Fatal("FC notice must not contain VFF identity")
	}
	if !strings.Contains(html, `<h1 class="h4 mb-3">Привязка кабинета</h1>`) {
		t.Fatal("H1 missing")
	}
	if !strings.Contains(html, `href="/account"`) {
		t.Fatal("missing /account link")
	}
}

func TestStandaloneLinkNoticePage_Escapes(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.Name = "Friends <Connect>"
	body, err := standaloneLinkNoticePage(cfg, "Привязка <кабинета>", `Ошибка <script>alert(1)</script>`)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	for _, needle := range []string{
		"Friends &lt;Connect&gt;",
		"Привязка &lt;кабинета&gt;",
		"Ошибка &lt;script&gt;alert(1)&lt;/script&gt;",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("missing escaped %q", needle)
		}
	}
	for _, forbid := range []string{
		"<Connect>",
		"<script>alert(1)</script>",
	} {
		if strings.Contains(html, forbid) {
			t.Fatalf("raw value must not appear: %q", forbid)
		}
	}
}

func TestStandaloneLinkNoticePage_FailClosed(t *testing.T) {
	if _, err := standaloneLinkNoticePage(nil, "Привязка кабинета", "x"); err == nil ||
		!strings.Contains(err.Error(), "account link brand name is required") {
		t.Fatalf("nil: %v", err)
	}
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""
	if _, err := standaloneLinkNoticePage(cfg, "Привязка кабинета", "x"); err == nil {
		t.Fatal("empty name must fail")
	}
	cfg.Brand.Name = " \t "
	if _, err := standaloneLinkNoticePage(cfg, "Привязка кабинета", "x"); err == nil {
		t.Fatal("whitespace name must fail")
	}
}

func TestServeAccountLink_NoticeErrorBrandMatrix(t *testing.T) {
	errCases := []struct {
		code string
		text string
	}{
		{"already_linked", "К этому аккаунту уже привязан другой email"},
		{"telegram_mismatch", "Данные сессии Telegram не совпали"},
		{"bad_user", "Не удалось завершить привязку. Попробуйте заново из Telegram-бота."},
		{"token_failed", "Не удалось выдать сессию"},
		{"shm_login2_not_persisted", "Не удалось завершить привязку аккаунта в биллинге"},
		{"link_failed", "Не удалось сохранить привязку"},
	}
	brands := []struct {
		name      string
		brandName string
		title     string
		forbid    string
	}{
		{"vff", "VPN for Friends", "<title>Привязка кабинета — VPN for Friends</title>", "— Friends Connect"},
		{"fc", "Friends Connect", "<title>Привязка кабинета — Friends Connect</title>", "— VPN for Friends"},
	}

	for _, brand := range brands {
		t.Run(brand.name, func(t *testing.T) {
			for _, ec := range errCases {
				t.Run(ec.code, func(t *testing.T) {
					cfg := orderStartTestCfg()
					if brand.name == "fc" {
						cfg = friendsConnectAccountTestCfg()
					}
					cfg.Brand.Name = brand.brandName

					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/account/link?err="+ec.code, nil)
					serveAccountLink(cfg, &stubAccountWeb{}).ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("code=%d", rec.Code)
					}
					if got := rec.Header().Get("Cache-Control"); got != "no-store" {
						t.Fatalf("Cache-Control=%q", got)
					}
					body := rec.Body.String()
					cl := rec.Header().Get("Content-Length")
					if cl != strconv.Itoa(len(body)) {
						t.Fatalf("Content-Length=%q len=%d", cl, len(body))
					}
					for _, needle := range []string{
						brand.title,
						`<h1 class="h4 mb-3">Привязка кабинета</h1>`,
						ec.text,
						`href="/account"`,
					} {
						if !strings.Contains(body, needle) {
							t.Fatalf("missing %q", needle)
						}
					}
					if strings.Contains(body, brand.forbid) {
						t.Fatalf("must not contain %q", brand.forbid)
					}
				})
			}
		})
	}
}

func TestServeAccountLink_NoticeRenderErrorReturns500(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.Brand.Name = ""
	rec := httptest.NewRecorder()
	serveAccountLink(cfg, &stubAccountWeb{}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/account/link?err=already_linked", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, forbid := range []string{
		"VPN for Friends",
		"Friends Connect",
		"Привязка кабинета",
		"уже привязан другой email",
		"<title>",
	} {
		if strings.Contains(body, forbid) {
			t.Fatalf("500 must not contain %q", forbid)
		}
	}
	if !strings.Contains(body, "Internal Server Error") {
		t.Fatalf("body=%q", body)
	}
}
