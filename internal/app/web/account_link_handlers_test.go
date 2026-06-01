package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestIsWebLinkedTelegramUser(t *testing.T) {
	t.Parallel()
	tab := []struct {
		name string
		u    *models.User
		want bool
	}{
		{"nil", nil, false},
		{"missing_login2", &models.User{Settings: models.UserSettings{Web: models.WebInfo{Email: "a@b.c"}}}, false},
		{"wrong_prefix", &models.User{Login2: "other", Settings: models.UserSettings{Web: models.WebInfo{Email: "a@b.c"}}}, false},
		{"missing_email", &models.User{Login2: "web_xx", Settings: models.UserSettings{}}, false},
		{"ok", &models.User{Login2: "web_9f1b113a", Settings: models.UserSettings{Web: models.WebInfo{Email: "a@b.c"}}}, true},
	}
	for _, tc := range tab {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isWebLinkedTelegramUser(tc.u); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestServeAccountLink_AlreadyLinked_RedirectSession(t *testing.T) {
	const chatID int64 = 44229901
	sec := strings.Repeat("z", 40)
	cfg := orderStartTestCfg()
	cfg.WebSales.OrderTokenSecret = sec

	linkTok, err := CreateAccountTelegramLinkToken(sec, 27, chatID, cfg)
	if err != nil {
		t.Fatal(err)
	}

	linkUser := models.User{
		ID:     27,
		Login:  "@telegram27",
		Login2: "web_9f1b113a91c4b2f6",
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{ChatID: chatID},
			Web:      models.WebInfo{Email: "linked.person@Example.COM"},
		},
	}
	st := &stubAccountWeb{getUserByIDRet: &linkUser}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account/link?token="+url.QueryEscape(linkTok), nil)
	serveAccountLink(cfg, st).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	to, err := url.Parse(loc)
	if err != nil || to.Path != "/account/session" {
		t.Fatalf("location=%q err=%v", loc, err)
	}
	raw := strings.TrimSpace(to.Query().Get("token"))
	if raw == "" {
		t.Fatal("missing session token in redirect")
	}
	claims, err := ParseAndVerifyAccountToken(sec, raw)
	if err != nil || claims.UserID != 27 || claims.Login != "@telegram27" || claims.Email != "linked.person@example.com" {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
	if st.getUserByIDCalls != 1 || st.getUserByIDArg != 27 {
		t.Fatalf("getUserByID calls=%d arg=%d", st.getUserByIDCalls, st.getUserByIDArg)
	}
	if st.linkWebEmailCalls != 0 || st.findOrCreateCalls != 0 {
		t.Fatalf("link=%d findOrCreate=%d", st.linkWebEmailCalls, st.findOrCreateCalls)
	}
}

func TestServeAccountLink_NotYetLinked_ShowsStartPage(t *testing.T) {
	const chatID int64 = 110022
	sec := strings.Repeat("y", 40)
	cfg := orderStartTestCfg()
	cfg.WebSales.OrderTokenSecret = sec
	linkTok, err := CreateAccountTelegramLinkToken(sec, 5, chatID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{
		ID:       5,
		Login:    "@tg5",
		Login2:   "",
		Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: chatID}},
	}
	rec := httptest.NewRecorder()
	serveAccountLink(cfg, &stubAccountWeb{getUserByIDRet: &u}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/account/link?token="+url.QueryEscape(linkTok), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/api/account/google/start") || !strings.Contains(body, "Привязка личного кабинета") {
		t.Fatalf("unexpected link_start body prefix: %s", truncateForTest(body))
	}
	if !strings.Contains(body, "Web-кабинет — это дополнительный способ управления VPN-услугами") ||
		!strings.Contains(body, "если Telegram недоступен или работает нестабильно") {
		t.Fatal("link_start must explain web cabinet as backup access")
	}
}

func TestServeAccountLink_Login2WithoutWebEmail_ShowsStartPage(t *testing.T) {
	const chatID int64 = 667788
	sec := strings.Repeat("x", 41)
	cfg := orderStartTestCfg()
	cfg.WebSales.OrderTokenSecret = sec
	linkTok, err := CreateAccountTelegramLinkToken(sec, 88, chatID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{
		ID:     88,
		Login:  "@tg88",
		Login2: "web_partial_only",
		Settings: models.UserSettings{
			Telegram: models.TelegramInfo{ChatID: chatID},
		},
	}
	rec := httptest.NewRecorder()
	st := &stubAccountWeb{getUserByIDRet: &u}
	serveAccountLink(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/account/link?token="+url.QueryEscape(linkTok), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	b := rec.Body.String()
	if !strings.Contains(b, "/api/account/google/start") {
		t.Fatalf("want link UI, body=%s", truncateForTest(b))
	}
	if st.linkWebEmailCalls != 0 {
		t.Fatalf("unexpected LinkWebEmail calls %d", st.linkWebEmailCalls)
	}
}

func truncateForTest(s string, maxLens ...int) string {
	ml := 200
	if len(maxLens) > 0 && maxLens[0] > 0 {
		ml = maxLens[0]
	}
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\r", "")
	if len(s) <= ml {
		return s
	}
	return s[:ml] + "…"
}
