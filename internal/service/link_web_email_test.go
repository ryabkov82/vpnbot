package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/infrastructure/api"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

func decodeFilter(t *testing.T, qs string) map[string]interface{} {
	t.Helper()
	s, err := url.QueryUnescape(qs)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if json.Unmarshal([]byte(s), &m) != nil {
		t.Fatalf("bad filter %q", s)
	}
	return m
}

func TestLinkWebEmailConflictOtherUserUsesPrimaryWebLogin(t *testing.T) {
	em := `u@blocked.test`
	normEM, err := webuser.NormalizeEmail(em)
	if err != nil {
		t.Fatal(err)
	}
	wLogin := webuser.WebLoginFromEmail(normEM)

	var step atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s := step.Add(1)
		f := decodeFilter(t, r.URL.Query().Get("filter"))
		switch {
		case f["user_id"] != nil:
			if s != 1 {
				t.Fatalf("unexpected user_id GET at step %d", s)
			}
			b, _ := json.Marshal(map[string][]models.User{
				"data": {{
					ID:      42,
					Login:   "@9001",
					Login2:  "",
					Balance: 0,
					Settings: models.UserSettings{
						BrandID:  "vff",
						Telegram: models.TelegramInfo{ChatID: 9001},
					},
				}},
			})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)
		case f["login"] != nil:
			want := strings.TrimSpace(f["login"].(string))
			if want != wLogin {
				t.Fatalf("login filter %q", want)
			}
			// Same-brand other account occupies web login.
			_, _ = w.Write([]byte(`{"data":[{"user_id":999,"login":"` + wLogin + `","balance":0,"settings":{"brand_id":"vff"}}]}`))
		default:
			t.Fatalf("unexpected filter %#v step %d", f, s)
		}
	}))
	t.Cleanup(srv.Close)

	acl := &api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	svc := NewService(acl, testServiceBrand())
	_, ferr := svc.LinkWebEmailForTelegramUser(42, 9001, em, "telegram_link")
	if ferr != ErrWebEmailUsedByOtherAccount {
		t.Fatalf("want conflict got %v", ferr)
	}
	if step.Load() != 2 {
		t.Fatalf("steps %d", step.Load())
	}
}

func TestLinkWebEmailSuccess_PostsLogin2AndKeepsTelegramBlock(t *testing.T) {
	em := `linked@tg.test`
	normEM, err := webuser.NormalizeEmail(em)
	if err != nil {
		t.Fatal(err)
	}
	wLogin := webuser.WebLoginFromEmail(normEM)

	row := models.User{
		ID:      42,
		Login:   "@4242",
		Login2:  "",
		Balance: 0,
		Settings: models.UserSettings{
			BrandID:  "vff",
			Telegram: models.TelegramInfo{ChatID: 4242},
		},
	}
	rowJSON, err := json.Marshal(map[string][]models.User{"data": {row}})
	if err != nil {
		t.Fatal(err)
	}

	var postCount atomic.Int64
	var step atomic.Int64
	var login2Polls atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			postCount.Add(1)
			raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			var body map[string]interface{}
			if json.Unmarshal(raw, &body) != nil {
				t.Fatal("post body invalid")
			}
			if int(body["user_id"].(float64)) != 42 || body["login2"] != wLogin {
				t.Fatalf("unexpected post %+v", body)
			}
			stObj, ok := body["settings"].(map[string]interface{})
			if !ok {
				t.Fatalf("settings %+v", body)
			}
			tel, ok := stObj["telegram"].(map[string]interface{})
			if !ok || tel["chat_id"] == nil {
				t.Fatalf("telegram preserved? %#v", stObj)
			}
			webBlk, ok := stObj["web"].(map[string]interface{})
			if !ok || strings.TrimSpace(webBlk["email"].(string)) != normEM {
				t.Fatalf("web block %#v", webBlk)
			}
			if webBlk["source"] != "telegram_link" {
				t.Fatalf("web source %+v", webBlk)
			}
			if stObj["brand_id"] != "vff" {
				t.Fatalf("brand_id backfill %#v", stObj["brand_id"])
			}
			// Ответ POST может быть пустым — без лишнего GET должен сохраниться token-friendly user (login/login2/email).
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			return
		}
		f := decodeFilter(t, r.URL.Query().Get("filter"))
		st := step.Add(1)
		switch {
		case f["user_id"] != nil:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(rowJSON)
		case f["login"] != nil && f["login"] == wLogin:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
		case f["login2"] != nil && f["login2"] == wLogin:
			w.WriteHeader(http.StatusOK)
			rnd := login2Polls.Add(1)
			if rnd == 1 {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			rowVerify := models.User{
				ID:      42,
				Login:   "@4242",
				Login2:  wLogin,
				Balance: 0,
				Settings: models.UserSettings{
					BrandID:  "vff",
					Telegram: models.TelegramInfo{ChatID: 4242},
					Web:      models.WebInfo{Email: normEM, Source: "telegram_link"},
				},
			}
			bVerify, err := json.Marshal(map[string][]models.User{"data": {rowVerify}})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(bVerify)
		default:
			t.Fatalf("unexpected filter %#v step %d", f, st)
		}
	}))
	t.Cleanup(srv.Close)

	acl := &api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	svc := NewService(acl, testServiceBrand())
	got, ferr := svc.LinkWebEmailForTelegramUser(42, 4242, em, "telegram_link")
	if ferr != nil {
		t.Fatal(ferr)
	}
	if got == nil || got.Login2 != wLogin || got.Settings.Web.Email != normEM {
		t.Fatalf("%#v", got)
	}
	if postCount.Load() != 1 {
		t.Fatalf("POST count %d", postCount.Load())
	}
	if step.Load() != 5 {
		t.Fatalf("GET steps expected 5 got %d", step.Load())
	}
}

func TestLinkWebEmail_ErrLogin2NotPersisted(t *testing.T) {
	em := `noreply@shm.test`
	normEM, err := webuser.NormalizeEmail(em)
	if err != nil {
		t.Fatal(err)
	}
	wLogin := webuser.WebLoginFromEmail(normEM)

	row := models.User{
		ID:      30,
		Login:   "@7070",
		Login2:  "",
		Balance: 0,
		Settings: models.UserSettings{
			BrandID:  "vff",
			Telegram: models.TelegramInfo{ChatID: 7070},
		},
	}
	rowJSON, err := json.Marshal(map[string][]models.User{"data": {row}})
	if err != nil {
		t.Fatal(err)
	}

	var login2Polls atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			return
		}
		f := decodeFilter(t, r.URL.Query().Get("filter"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch {
		case f["user_id"] != nil:
			_, _ = w.Write(rowJSON)
		case f["login"] != nil && f["login"] == wLogin:
			_, _ = w.Write([]byte(`{"data":[]}`))
		case f["login2"] != nil && f["login2"] == wLogin:
			_ = login2Polls.Add(1)
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected filter %#v", f)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, testServiceBrand())
	_, ferr := svc.LinkWebEmailForTelegramUser(30, 7070, em, "telegram_link_google")
	if !errors.Is(ferr, ErrWebLogin2NotPersisted) {
		t.Fatalf("want ErrWebLogin2NotPersisted got %v", ferr)
	}
	if login2Polls.Load() < 2 {
		t.Fatalf("expected at least two login2 lookups for verify, got %d", login2Polls.Load())
	}
}

func TestLinkWebEmailIdempotentSameStoredEmail_NoPost(t *testing.T) {
	em := `already@tg.test`
	normEM, err := webuser.NormalizeEmail(em)
	if err != nil {
		t.Fatal(err)
	}
	wLogin := webuser.WebLoginFromEmail(normEM)

	row := models.User{
		ID:     51,
		Login:  `@9191`,
		Login2: wLogin,
		Settings: models.UserSettings{
			BrandID:  "vff",
			Telegram: models.TelegramInfo{ChatID: 9191},
			Web:      models.WebInfo{Email: normEM, Source: "telegram_link"},
		},
	}
	rowJSON, _ := json.Marshal(map[string][]models.User{"data": {row}})

	var posts atomic.Int64
	var gets atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			posts.Add(1)
			return
		}
		gets.Add(1)
		f := decodeFilter(t, r.URL.Query().Get("filter"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch {
		case f["user_id"] != nil:
			_, _ = w.Write(rowJSON)
		case f["login"] != nil:
			if f["login"] == wLogin {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			t.Fatal("unexpected login filter")
		case f["login2"] != nil && f["login2"] == wLogin:
			b, err := json.Marshal(map[string][]models.User{"data": {row}})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(b)
		default:
			t.Fatalf("unexpected filter %#v", f)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, testServiceBrand())
	u, ferr := svc.LinkWebEmailForTelegramUser(51, 9191, em, "telegram_link_google")
	if ferr != nil || u == nil || u.ID != 51 {
		t.Fatalf("%#v err=%v", u, ferr)
	}
	if posts.Load() != 0 {
		t.Fatal("unexpected POST")
	}
	if gets.Load() != 4 {
		t.Fatalf("GET count %d", gets.Load())
	}
}

func TestGoogleCallbackFind_OrViaLogin2_Pattern(t *testing.T) {
	// Общий вход web по email после Telegram→Web: FindOrCreateWebUser видит связку через login2, RegisterUser не вызывается.
	em := `g@xz.io`
	shm := &models.User{
		ID:       12,
		Login:    `@55`,
		Login2:   webuser.WebLoginFromEmail(em),
		Settings: models.UserSettings{BrandID: "vff"},
	}
	reg := &testWebUserRegistrar{
		firstGet:   nil,
		login2User: shm,
	}
	got, _, err := findOrCreateWebUser(reg, em, testWebLoginPrefix, testWebUserSource, "vff")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != shm.ID || reg.login2Calls != 1 || reg.getCalls != 1 {
		t.Fatalf("got %#v calls login=%d login2=%d", got, reg.getCalls, reg.login2Calls)
	}
}

func TestLinkWebEmail_WrongTelegramLogin_Mismatch(t *testing.T) {
	em := "x@y.zz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shm/v1/admin/user" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		b, _ := json.Marshal(map[string][]models.User{
			"data": {{
				ID:    7,
				Login: "@wrong",
				Settings: models.UserSettings{
					BrandID:  "vff",
					Telegram: models.TelegramInfo{ChatID: 100},
				},
			}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	_, err := svc.LinkWebEmailForTelegramUser(7, 100, em, "telegram_link")
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestLinkWebEmail_WrongBrandID_Mismatch(t *testing.T) {
	em := "x@y.zz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string][]models.User{
			"data": {{
				ID:    8,
				Login: "@fc_200",
				Settings: models.UserSettings{
					BrandID:  "vff",
					Telegram: models.TelegramInfo{ChatID: 200},
				},
			}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("fc"))
	_, err := svc.LinkWebEmailForTelegramUser(8, 200, em, "telegram_link")
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestLinkWebEmail_OtherBrandWebLogin_Mismatch(t *testing.T) {
	em := "taken@other.test"
	normEM, _ := webuser.NormalizeEmail(em)
	wLogin := webuser.WebLoginFromEmail(normEM)
	var step atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := decodeFilter(t, r.URL.Query().Get("filter"))
		step.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case f["user_id"] != nil:
			b, _ := json.Marshal(map[string][]models.User{"data": {{
				ID:    42,
				Login: "@9001",
				Settings: models.UserSettings{
					BrandID:  "vff",
					Telegram: models.TelegramInfo{ChatID: 9001},
				},
			}}})
			_, _ = w.Write(b)
		case f["login"] != nil:
			b, _ := json.Marshal(map[string][]models.User{"data": {{
				ID:       999,
				Login:    wLogin,
				Settings: models.UserSettings{BrandID: "fc"},
			}}})
			_, _ = w.Write(b)
		default:
			t.Fatalf("unexpected %#v", f)
		}
	}))
	t.Cleanup(srv.Close)
	svc := NewService(&api.APIClient{ServerURL: srv.URL, HTTPClient: srv.Client()}, brandCfg("vff"))
	_, err := svc.LinkWebEmailForTelegramUser(42, 9001, em, "telegram_link")
	if !errors.Is(err, ErrUserIdentityMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestMergeSettingsJSONToMap_PreservesUnknown(t *testing.T) {
	raw := json.RawMessage(`{"telegram":{"chat_id":1},"custom_keep":true,"web":{"email":"a@b.c"}}`)
	m, err := mergeSettingsJSONToMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m["custom_keep"] != true {
		t.Fatalf("%#v", m)
	}
}
