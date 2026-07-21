package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	appService "github.com/ryabkov82/vpnbot/internal/service"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

type stubAccountWeb struct {
	userByLogin     *models.User
	userByLoginErr  error
	services        []models.UserService
	servicesErr     error
	single          map[int]*models.UserService
	requireCategory string // если задана — имитация фильтра категории бренда

	balance      *models.UserBalance
	balanceErr   error
	balanceCalls int

	pays    []models.UserPay
	paysErr error

	shmServices    []models.Service
	shmServicesErr error

	svcByID     map[int]*models.Service
	getSvcByErr error

	serviceOrderRet   *models.UserService
	serviceOrderErr   error
	serviceOrderUID   int
	serviceOrderSID   int
	serviceOrderCalls int

	deleteCalls             int
	deleteErr               error
	deleteLastUID           int
	deleteLastUserServiceID string

	findOrCreateRet     *models.User
	findOrCreateErr     error
	findOrCreateCalls   int
	findOrCreateCreated bool

	findUserByWebEmailRet   *models.User
	findUserByWebEmailErr   error
	findUserByWebEmailCalls int

	linkWebEmailCalls int
	linkWebEmailRet   *models.User
	linkWebEmailErr   error

	getUserByLoginCalls int

	getUserByIDCalls int
	getUserByIDArg   int
	getUserByIDRets  map[int]*models.User
	getUserByIDRet   *models.User
	getUserByIDErr   error
}

func (s *stubAccountWeb) GetUserByID(userID int) (*models.User, error) {
	s.getUserByIDCalls++
	s.getUserByIDArg = userID
	if s.getUserByIDErr != nil {
		return nil, s.getUserByIDErr
	}
	if len(s.getUserByIDRets) > 0 {
		return s.getUserByIDRets[userID], nil
	}
	return s.getUserByIDRet, nil
}

func (s *stubAccountWeb) FindUserByWebEmail(email string) (*models.User, error) {
	s.findUserByWebEmailCalls++
	if s.findUserByWebEmailErr != nil {
		return nil, s.findUserByWebEmailErr
	}
	return s.findUserByWebEmailRet, nil
}

func (s *stubAccountWeb) LinkWebEmailForTelegramUser(userID int, telegramChatID int64, email string, source string) (*models.User, error) {
	s.linkWebEmailCalls++
	if s.linkWebEmailErr != nil {
		return nil, s.linkWebEmailErr
	}
	return s.linkWebEmailRet, nil
}

func (s *stubAccountWeb) GetUserBalanceByUserID(userID int) (*models.UserBalance, error) {
	s.balanceCalls++
	if s.balanceErr != nil {
		return nil, s.balanceErr
	}
	return s.balance, nil
}

func (s *stubAccountWeb) GetUserPaysByUserID(userID int) ([]models.UserPay, error) {
	if s.paysErr != nil {
		return nil, s.paysErr
	}
	return s.pays, nil
}

func (s *stubAccountWeb) GetUserByLogin(login string) (*models.User, error) {
	s.getUserByLoginCalls++
	if s.userByLoginErr != nil {
		return nil, s.userByLoginErr
	}
	return s.userByLogin, nil
}

func (s *stubAccountWeb) FindOrCreateWebUser(email string) (*models.User, bool, error) {
	s.findOrCreateCalls++
	if s.findOrCreateErr != nil {
		return nil, false, s.findOrCreateErr
	}
	return s.findOrCreateRet, s.findOrCreateCreated, nil
}
func (s *stubAccountWeb) GetUserServicesByUserID(userID int) ([]models.UserService, error) {
	if s.servicesErr != nil {
		return nil, s.servicesErr
	}
	return s.services, nil
}

func (s *stubAccountWeb) GetOwnedUserServiceByUserID(userID int, userServiceID string) (*models.UserService, error) {
	if s.single == nil {
		return nil, appService.ErrUserServiceUnavailable
	}
	id, err := strconv.Atoi(userServiceID)
	if err != nil || id <= 0 {
		return nil, appService.ErrUserServiceUnavailable
	}
	us := s.single[id]
	if us == nil || us.UserID != userID || us.ServiceID != id {
		return nil, appService.ErrUserServiceUnavailable
	}
	if s.requireCategory != "" && strings.TrimSpace(us.Category) != strings.TrimSpace(s.requireCategory) {
		return nil, appService.ErrUserServiceUnavailable
	}
	return us, nil
}

func (s *stubAccountWeb) GetServices() ([]models.Service, error) {
	if s.shmServicesErr != nil {
		return nil, s.shmServicesErr
	}
	return s.shmServices, nil
}

func (s *stubAccountWeb) GetServiceByID(serviceID int) (*models.Service, error) {
	if s.getSvcByErr != nil {
		return nil, s.getSvcByErr
	}
	if s.svcByID == nil {
		return nil, nil
	}
	return s.svcByID[serviceID], nil
}

func (s *stubAccountWeb) ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error) {
	s.serviceOrderCalls++
	s.serviceOrderUID = userID
	s.serviceOrderSID = serviceID
	if s.serviceOrderErr != nil {
		return nil, s.serviceOrderErr
	}
	return s.serviceOrderRet, nil
}

func (s *stubAccountWeb) DeleteUserServiceByUserID(userID int, userServiceID string) error {
	s.deleteCalls++
	s.deleteLastUID = userID
	s.deleteLastUserServiceID = userServiceID
	return s.deleteErr
}

func extractMagicLinkTokenFromSMTPMsg(t *testing.T, msg []byte) string {
	t.Helper()
	s := string(msg)
	idx := strings.Index(s, "/account/session?token=")
	if idx < 0 {
		t.Fatal("magic link missing in SMTP payload")
	}
	rest := s[idx+len("/account/session?token="):]
	cut := len(rest)
	for _, sep := range []string{"\r\n", "\n"} {
		if p := strings.Index(rest, sep); p >= 0 && p < cut {
			cut = p
		}
	}
	enc := strings.TrimSpace(rest[:cut])
	dec, err := url.QueryUnescape(enc)
	if err != nil {
		return enc
	}
	return strings.TrimSpace(dec)
}

func TestServeAccountLoginStart_Honeypot(t *testing.T) {
	var smtpN int
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		smtpN++
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	h := serveAccountLoginStart(cfg, &stubAccountWeb{}, rl)
	body := `{"email":"a@b.c","website":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || smtpN != 0 {
		t.Fatalf("code=%d smtp=%d body=%s", rec.Code, smtpN, rec.Body.String())
	}
	var out accountLoginStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || out.Status != "email_sent" {
		t.Fatalf("%#v err=%v", out, err)
	}
}

func TestServeAccountLoginStart_EmptyWebLoginPrefix_InternalErrorNoSideEffects(t *testing.T) {
	var smtpN int
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		smtpN++
		return nil
	})
	cfg := orderStartTestCfg()
	cfg.Brand.WebUserLoginPrefix = ""
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	st := &stubAccountWeb{}
	h := serveAccountLoginStart(cfg, st, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"a@b.c","website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "internal_error")
	if smtpN != 0 {
		t.Fatalf("smtp must not run, got %d", smtpN)
	}
	if st.findUserByWebEmailCalls != 0 || st.getUserByLoginCalls != 0 || st.findOrCreateCalls != 0 {
		t.Fatalf("no lookup/create side effects: findEmail=%d getLogin=%d findOrCreate=%d",
			st.findUserByWebEmailCalls, st.getUserByLoginCalls, st.findOrCreateCalls)
	}
}

func TestServeAccountLoginStart_UnknownEmailSendsSignupTokenMail_NoWebUserYet(t *testing.T) {
	var gotMail []byte
	var smtpN int
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		smtpN++
		gotMail = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	st := &stubAccountWeb{}
	h := serveAccountLoginStart(cfg, st, rl)
	rawEmail := `nouser@test.com`
	normEmail, err := webuser.NormalizeEmail(rawEmail)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"`+rawEmail+`","website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || smtpN != 1 {
		t.Fatalf("code=%d smtp=%d body=%s", rec.Code, smtpN, rec.Body.String())
	}
	if st.findOrCreateCalls != 0 {
		t.Fatalf("FindOrCreateWebUser must not run at login/start, got %d calls", st.findOrCreateCalls)
	}
	var out accountLoginStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || out.Status != "email_sent" {
		t.Fatalf("%#v err=%v", out, err)
	}
	rawTok := extractMagicLinkTokenFromSMTPMsg(t, gotMail)
	sc, err := ParseAndVerifyAccountSignupToken(cfg.WebSales.OrderTokenSecret, rawTok)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(strings.TrimSpace(sc.Email)) != normEmail {
		t.Fatalf("email claim %q vs %q", sc.Email, normEmail)
	}
	wantLogin := webuser.WebLoginFromEmail(normEmail)
	if sc.Login != wantLogin {
		t.Fatalf("login claim %q vs %q", sc.Login, wantLogin)
	}
}

func TestServeAccountLoginStart_KnownEmailSendsMail(t *testing.T) {
	var gotMail []byte
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotMail = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	cfg.Brand.PublicBaseURL = "https://shop.example"
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	em := `known@test.com`
	wantNorm, nerr := webuser.NormalizeEmail(em)
	if nerr != nil {
		t.Fatal(nerr)
	}
	wantLogin := webuser.WebLoginFromEmail(wantNorm)
	u := &models.User{ID: 511, Login: wantLogin}
	st := &stubAccountWeb{userByLogin: u}
	h := serveAccountLoginStart(cfg, st, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"`+em+`","website":""}`))
	req.Host = "localhost:9090"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	raw := string(gotMail)
	if !strings.Contains(raw, "/account/session?token=") {
		t.Fatalf("missing magic link body: %s", raw[:min(600, len(raw))])
	}
	rawTok := extractMagicLinkTokenFromSMTPMsg(t, gotMail)
	ac, err := ParseAndVerifyAccountToken(cfg.WebSales.OrderTokenSecret, rawTok)
	if err != nil || ac.UserID != 511 || ac.Email != wantNorm || ac.Login != u.Login {
		t.Fatalf("account claims %+v err=%v", ac, err)
	}
}

func TestServeAccountLoginStart_PrefersSettingsWebEmailUser(t *testing.T) {
	var gotMail []byte
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotMail = append([]byte(nil), msg...)
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	em := `linked@test.com`
	normWant, err := webuser.NormalizeEmail(em)
	if err != nil {
		t.Fatal(err)
	}
	linked := models.User{ID: 918, Login: `@918`}
	st := &stubAccountWeb{
		findUserByWebEmailRet: &linked,
		userByLogin:           nil,
	}
	rec := httptest.NewRecorder()
	serveAccountLoginStart(cfg, st, rl).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"`+em+`","website":""}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rawTok := extractMagicLinkTokenFromSMTPMsg(t, gotMail)
	sec := cfg.WebSales.OrderTokenSecret
	ac, err := ParseAndVerifyAccountToken(sec, rawTok)
	if err != nil {
		t.Fatal(err)
	}
	if ac.UserID != 918 || ac.Email != normWant || ac.Login != linked.Login {
		t.Fatalf("claims %+v", ac)
	}
	if _, err := ParseAndVerifyAccountSignupToken(sec, rawTok); err == nil {
		t.Fatal("expected signup decode to fail (account magic link)")
	}
}

func TestServeAccountLoginStart_KnownAndUnknownSameJSONBody(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return nil
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)

	kRec := httptest.NewRecorder()
	knownEmail := "knowntest@test.com"
	kNorm, err := webuser.NormalizeEmail(knownEmail)
	if err != nil {
		t.Fatal(err)
	}
	kLogin := webuser.WebLoginFromEmail(kNorm)
	serveAccountLoginStart(cfg, &stubAccountWeb{userByLogin: &models.User{ID: 77, Login: kLogin}}, rl).ServeHTTP(kRec,
		httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"`+knownEmail+`","website":""}`)))
	if kRec.Code != http.StatusOK {
		t.Fatalf("known email: %s", kRec.Body.String())
	}
	uRec := httptest.NewRecorder()
	serveAccountLoginStart(cfg, &stubAccountWeb{}, rl).ServeHTTP(uRec,
		httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"unknowntest@test.com","website":""}`)))
	if uRec.Code != http.StatusOK {
		t.Fatalf("unknown email: %s", uRec.Body.String())
	}
	if strings.TrimSpace(kRec.Body.String()) != strings.TrimSpace(uRec.Body.String()) {
		t.Fatalf("responses differ (enumeration leak?)\nk=%s\nu=%s", kRec.Body.String(), uRec.Body.String())
	}
}

func TestServeAccountLoginStart_SMTPError(t *testing.T) {
	patchSMTP(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return errors.New("smtp down")
	})
	cfg := orderStartTestCfg()
	rl := newLeadRateLimiter(50, time.Hour, 50, time.Hour)
	un, err := webuser.NormalizeEmail("u@test.com")
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{userByLogin: &models.User{ID: 1, Login: webuser.WebLoginFromEmail(un)}}
	h := serveAccountLoginStart(cfg, st, rl)
	req := httptest.NewRequest(http.MethodPost, "/api/account/login/start", strings.NewReader(`{"email":"u@test.com","website":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "email_send_failed")
}

func TestServeAccountSessionStart_InvalidToken(t *testing.T) {
	cfg := orderStartTestCfg()
	h := serveAccountSessionStart(cfg, &stubAccountWeb{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(`{"token":"not-a-valid-token"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func accountSessionStartPostBody(t *testing.T, tok string) string {
	t.Helper()
	b, err := json.Marshal(accountSessionStartReqJSON{Token: tok})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestServeAccountSessionStart_AccountTokenReturnsSame(t *testing.T) {
	var telegramNotifyCalls int
	patchAccountWebUserRegisteredTelegramNotifier(t, func(*config.Config, string, int, string, string) {
		telegramNotifyCalls++
	})
	cfg := orderStartTestCfg()
	sec := cfg.WebSales.OrderTokenSecret
	rawTok, err := CreateAccountToken(sec, "acc@test.com", 44, "web_acc44", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{}
	h := serveAccountSessionStart(cfg, st)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(accountSessionStartPostBody(t, rawTok)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountSessionStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || out.AccountToken != rawTok || out.IsNewUser {
		t.Fatalf("%+v", out)
	}
	if st.findOrCreateCalls != 0 {
		t.Fatalf("unexpected FindOrCreateWebUser calls: %d", st.findOrCreateCalls)
	}
	if telegramNotifyCalls != 0 {
		t.Fatalf("telegram notifier must not run for plain account token, got %d", telegramNotifyCalls)
	}
}

func TestServeAccountSessionStart_SignupTokenCreatesWebUser(t *testing.T) {
	var telegramNotifyCalls int
	patchAccountWebUserRegisteredTelegramNotifier(t, func(*config.Config, string, int, string, string) {
		telegramNotifyCalls++
	})
	cfg := orderStartTestCfg()
	sec := cfg.WebSales.OrderTokenSecret
	em := "signup-new@test.com"
	norm, err := webuser.NormalizeEmail(em)
	if err != nil {
		t.Fatal(err)
	}
	login := webuser.WebLoginFromEmail(norm)
	signupTok, err := CreateAccountSignupToken(sec, norm, login, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := &models.User{ID: 6600, Login: login}
	st := &stubAccountWeb{findOrCreateRet: want, findOrCreateCreated: true}
	h := serveAccountSessionStart(cfg, st)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(accountSessionStartPostBody(t, signupTok)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountSessionStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.IsNewUser || out.Status != "ok" || out.AccountToken == "" || out.AccountToken == signupTok {
		t.Fatalf("%+v", out)
	}
	if st.findOrCreateCalls != 1 {
		t.Fatalf("want 1 FindOrCreateWebUser, got %d", st.findOrCreateCalls)
	}
	ac, err := ParseAndVerifyAccountToken(sec, out.AccountToken)
	if err != nil || ac.UserID != 6600 || ac.Login != login || ac.Email != norm {
		t.Fatalf("%+v err=%v", ac, err)
	}
	if telegramNotifyCalls != 1 {
		t.Fatalf("want 1 telegram notify for new registration, got %d", telegramNotifyCalls)
	}
}

func TestServeAccountSessionStart_SignupTokenExistingUser_NoDuplicate(t *testing.T) {
	var telegramNotifyCalls int
	patchAccountWebUserRegisteredTelegramNotifier(t, func(*config.Config, string, int, string, string) {
		telegramNotifyCalls++
	})
	cfg := orderStartTestCfg()
	sec := cfg.WebSales.OrderTokenSecret
	em := "signup-old@test.com"
	norm, _ := webuser.NormalizeEmail(em)
	login := webuser.WebLoginFromEmail(norm)
	signupTok, err := CreateAccountSignupToken(sec, norm, login, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	existing := &models.User{ID: 6611, Login: login}
	st := &stubAccountWeb{
		findOrCreateRet:     existing,
		findOrCreateCreated: false,
	}
	h := serveAccountSessionStart(cfg, st)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(accountSessionStartPostBody(t, signupTok)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountSessionStartOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.IsNewUser {
		t.Fatalf("expected existing user %+v", out)
	}
	if st.findOrCreateCalls != 1 {
		t.Fatalf("want 1 FindOrCreateWebUser, got %d", st.findOrCreateCalls)
	}
	ac, err := ParseAndVerifyAccountToken(sec, out.AccountToken)
	if err != nil || ac.UserID != 6611 {
		t.Fatalf("%+v err=%v", ac, err)
	}
	if telegramNotifyCalls != 0 {
		t.Fatalf("telegram notify must not run for existing SHM user, got %d", telegramNotifyCalls)
	}
}

func TestServeAccountSessionStart_SignupNewUserTelegramAPIErrorStill200(t *testing.T) {
	old := leadTelegramHTTPPost
	leadTelegramHTTPPost = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader(`{"ok":false}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	t.Cleanup(func() { leadTelegramHTTPPost = old })

	prevHook := accountWebUserRegisteredTelegramNotifier
	accountWebUserRegisteredTelegramNotifier = sendAccountWebUserRegisteredTelegramImpl
	t.Cleanup(func() { accountWebUserRegisteredTelegramNotifier = prevHook })

	cfg := orderStartTestCfg()
	cfg.Telegram.Token = "notify-test-token"
	cfg.Telegram.LeadsChatID = 7001

	sec := cfg.WebSales.OrderTokenSecret
	em := "tg-error@test.com"
	norm, _ := webuser.NormalizeEmail(em)
	login := webuser.WebLoginFromEmail(norm)
	signupTok, err := CreateAccountSignupToken(sec, norm, login, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := &models.User{ID: 7700, Login: login}
	st := &stubAccountWeb{findOrCreateRet: want, findOrCreateCreated: true}

	rec := httptest.NewRecorder()
	serveAccountSessionStart(cfg, st).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(accountSessionStartPostBody(t, signupTok))))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestServeAccountSessionStart_SignupNewUserNotifierGetsFirstXFFIP(t *testing.T) {
	var gotIP string
	patchAccountWebUserRegisteredTelegramNotifier(t, func(_ *config.Config, _ string, _ int, _ string, ip string) {
		gotIP = ip
	})

	cfg := orderStartTestCfg()
	sec := cfg.WebSales.OrderTokenSecret
	em := "xff@test.com"
	norm, _ := webuser.NormalizeEmail(em)
	login := webuser.WebLoginFromEmail(norm)
	signupTok, _ := CreateAccountSignupToken(sec, norm, login, time.Hour)
	want := &models.User{ID: 8801, Login: login}
	st := &stubAccountWeb{findOrCreateRet: want, findOrCreateCreated: true}

	req := httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(accountSessionStartPostBody(t, signupTok)))
	req.Header.Set("X-Forwarded-For", "203.0.113.55, 10.0.0.22")
	rec := httptest.NewRecorder()
	serveAccountSessionStart(cfg, st).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if gotIP != "203.0.113.55" {
		t.Fatalf("IP %q", gotIP)
	}
}

func TestServeAccountSessionStart_SignupTokenWebUserFails(t *testing.T) {
	cfg := orderStartTestCfg()
	sec := cfg.WebSales.OrderTokenSecret
	em := "fail-create@test.com"
	norm, _ := webuser.NormalizeEmail(em)
	login := webuser.WebLoginFromEmail(norm)
	signupTok, _ := CreateAccountSignupToken(sec, norm, login, time.Hour)
	st := &stubAccountWeb{findOrCreateErr: errors.New("register failed")}
	h := serveAccountSessionStart(cfg, st)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(accountSessionStartPostBody(t, signupTok)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "web_user_failed")
}

func TestServeAccountSessionStart_SignupTokenLoginMismatch(t *testing.T) {
	cfg := orderStartTestCfg()
	sec := cfg.WebSales.OrderTokenSecret
	em := "mis@test.com"
	norm, _ := webuser.NormalizeEmail(em)
	tok, err := CreateAccountSignupToken(sec, norm, "not_the_derived_login", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountSessionStart(cfg, &stubAccountWeb{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/account/session/start", strings.NewReader(accountSessionStartPostBody(t, tok)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountServices_InvalidToken(t *testing.T) {
	cfg := orderStartTestCfg()
	h := serveAccountServices(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodGet, "/api/account/services?token=bad.token.here", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountPayments_EmptyToken401(t *testing.T) {
	cfg := orderStartTestCfg()
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, &stubAccountWeb{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountPayments_InvalidToken401(t *testing.T) {
	cfg := orderStartTestCfg()
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, &stubAccountWeb{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments?token=not-a-jwt", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d: %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountPayments_APIDown500(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "p@test.com", 5, "l", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{paysErr: errors.New("shm")}
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments?token="+tok, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "payments_failed")
}

func TestServeAccountPayments_FiltersCanceledNoLeak(t *testing.T) {
	ev, err := json.Marshal(map[string]string{"event": "payment.canceled"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "pay@test.com", 9, "l9", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{pays: []models.UserPay{
		{Date: "2024-01-01", Money: 0, PaySystemID: "yookassa-canceled", Comment: json.RawMessage(ev), UniqKey: "secret-uk"},
		{Date: "2024-02-02", Money: 150.5, PaySystemID: "yookassa"},
		{Date: "2024-03-03", Money: -1318.74, PaySystemID: "corr"},
	}}
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	raw := rec.Body.String()
	for _, forbid := range []string{"uniq_key", `"comment"`, "secret-uk"} {
		if strings.Contains(raw, forbid) {
			t.Fatalf("leak %q in %s", forbid, raw)
		}
	}
	var env accountPaymentsOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Payments) != 2 {
		t.Fatalf("got %+v", env.Payments)
	}
	if env.Payments[0].Amount != 150.5 || env.Payments[0].AmountText != "150.50 ₽" || env.Payments[0].PaySystemID != "yookassa" {
		t.Fatalf("row0 %+v", env.Payments[0])
	}
	if env.Payments[1].Amount != -1318.74 || env.Payments[1].AmountText != "-1318.74 ₽" {
		t.Fatalf("row1 %+v", env.Payments[1])
	}
}

func TestServeAccountPayments_OnlyCanceledEmptySlice(t *testing.T) {
	ev, err := json.Marshal(map[string]string{"event": "payment.canceled"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "empty@test.com", 3, "l3", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{pays: []models.UserPay{
		{Date: "d", Money: 0, PaySystemID: "yookassa-canceled", Comment: json.RawMessage(ev)},
	}}
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var env accountPaymentsOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Payments) != 0 {
		t.Fatalf("%+v", env.Payments)
	}
	if !strings.Contains(rec.Body.String(), `"payments":[]`) {
		t.Fatalf("%s", rec.Body.String())
	}
}

func TestServeAccountPayments_LimitTwenty(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "lim@test.com", 2, "l2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var pays []models.UserPay
	for i := 1; i <= 21; i++ {
		pays = append(pays, models.UserPay{Date: "d", Money: float64(i), PaySystemID: "x"})
	}
	st := &stubAccountWeb{pays: pays}
	rec := httptest.NewRecorder()
	serveAccountPayments(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/payments?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var env accountPaymentsOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Payments) != 20 {
		t.Fatalf("len=%d", len(env.Payments))
	}
	if env.Payments[0].Amount != 2 {
		t.Fatalf("first should be first of truncated window, got %+v", env.Payments[0])
	}
	if env.Payments[19].Amount != 21 {
		t.Fatalf("last %+v", env.Payments[19])
	}
}

func TestServeAccountServices_SuccessNoSensitiveLeak(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateAccountToken(secret, "ok@test.com", 99, "web_l99", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		balance: &models.UserBalance{Balance: 0.93, Forecast: 280},
		services: []models.UserService{{
			Name:          "1 мес",
			ServiceID:     336,
			BaseServiceID: 3,
			Status:        "ACTIVE",
			Expire:        "2099",
			Period:        "1",
			Category:      "vpn-mz-test",
			KeyMarzban: models.UserKeyMarzban{
				SubscriptionURL: "https://sub-secret.example/",
				Links:           []string{"http://never"},
			},
		}},
	}
	h := serveAccountServices(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(strings.ToLower(raw), "subscription_url") || strings.Contains(raw, `"links"`) {
		t.Fatal("response leaks subscription_url or links")
	}
	var env struct {
		User struct {
			Email          string `json:"email"`
			ID             int    `json:"user_id"`
			Balance        float64
			Forecast       float64
			TelegramLinked bool `json:"telegram_linked"`
		} `json:"user"`
		Services []struct {
			UserServiceID int    `json:"user_service_id"`
			ServiceID     int    `json:"service_id"`
			CanConnect    bool   `json:"can_connect"`
			Tier          string `json:"tier"`
			ConnectApp    string `json:"connect_app"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	if env.User.Email != "ok@test.com" || env.User.ID != 99 || env.User.Balance != 0.93 || env.User.Forecast != 280 {
		t.Fatalf("user %+v", env.User)
	}
	if env.User.TelegramLinked {
		t.Fatalf("web-only stub user must not be telegram_linked: %+v", env.User)
	}
	if len(env.Services) != 1 || !env.Services[0].CanConnect || env.Services[0].UserServiceID != 336 || env.Services[0].ServiceID != 3 {
		t.Fatalf("services %+v", env.Services)
	}
	if env.Services[0].Tier != publicTierStandard || env.Services[0].ConnectApp != publicConnectSubscription {
		t.Fatalf("want standard tier, got %+v", env.Services[0])
	}
}

func TestServeAccountServices_NotPaidIncludesCostFromUser(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateAccountToken(secret, "paid@test.com", 77, "web_lp", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		balance: &models.UserBalance{Balance: 0, Forecast: 0},
		services: []models.UserService{{
			Name:          "Подписка",
			ServiceID:     401,
			BaseServiceID: 9,
			Status:        "NOT PAID",
			Cost:          "150,50",
			Expire:        "—",
			Period:        "1",
			Category:      "vpn-mz-test",
		}},
		svcByID: map[int]*models.Service{},
	}
	h := serveAccountServices(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	raw := strings.TrimSpace(rec.Body.String())
	var env accountServicesOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Services) != 1 || env.Services[0].Cost != 150.5 || env.Services[0].Status != "NOT PAID" {
		t.Fatalf("services row: %+v", env.Services)
	}
	for _, forbid := range []string{"subscription_url", "internal_squad", "Remnawave", `"config"`} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(forbid)) {
			t.Fatalf("unexpected substring %q in %s", forbid, raw)
		}
	}
}

func TestServeAccountServices_NotPaidUsesCatalogFallbackWhenUserCostBlank(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "cat@test.com", 88, "web_lc", time.Hour)
	st := &stubAccountWeb{
		balance: &models.UserBalance{Balance: 0, Forecast: 0},
		services: []models.UserService{{
			Name:          "Подписка",
			ServiceID:     402,
			BaseServiceID: 5,
			Status:        "NOT PAID",
			Cost:          "",
			Expire:        "—",
			Period:        "1",
			Category:      "vpn-mz-test",
		}},
		svcByID: map[int]*models.Service{
			5: {
				ServiceID:    5,
				Name:         "Цифровой тариф",
				Cost:         320,
				AllowToOrder: 1,
			},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServices(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var env accountServicesOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Services) != 1 || env.Services[0].Cost != 320 || env.Services[0].ServiceID != 5 {
		t.Fatalf("want catalog cost fallback, got %+v", env.Services[0])
	}
}

func TestServeAccountConnect_ACTIVE_OK(t *testing.T) {
	cfg := orderStartTestCfg()
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateAccountToken(secret, "me@test.com", 10, "web_aa", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:        10,
				ServiceID:     336,
				BaseServiceID: 3,
				Status:        "ACTIVE",
				Category:      "vpn-mz-x",
				KeyMarzban:    models.UserKeyMarzban{SubscriptionURL: "https://sub.example/connect"},
			},
		},
	}
	h := serveAccountServiceConnect(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var out accountConnectOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || out.ConnectURL != "https://sub.example/connect" || out.ConnectTitle != accountConnectTitleStandard || out.ConnectApp != publicConnectSubscription {
		t.Fatalf("%#v", out)
	}
}

func TestServeAccountConnect_UserMismatchForbidden(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:    999,
				ServiceID: 336,
				Status:    "ACTIVE",
				Category:  "vpn-mz-x",
			},
		},
	}
	h := serveAccountServiceConnect(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestServeAccountConnect_NotPaid_NotReady(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			336: {
				UserID:    10,
				ServiceID: 336,
				Status:    "NOT PAID",
				Category:  "vpn-mz-x",
			},
		},
	}
	h := serveAccountServiceConnect(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=336", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out accountConnectOKJSON
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if rec.Code != http.StatusOK || out.Status != "not_ready" || out.ConnectURL != "" {
		t.Fatalf("%#v code=%d", out, rec.Code)
	}
}

func catalogPremiumConfigRaw(name string) string {
	return `{"remnawave":{"internal_squad_name":"` + name + `"}}`
}

func TestServeAccountServices_PremiumActive(t *testing.T) {
	cfg := orderStartTestCfg()
	squad := "anti-premium-squad-x"
	cfg.PremiumSquadName = squad
	secret := cfg.WebSales.OrderTokenSecret
	tok, err := CreateAccountToken(secret, "p@test.com", 55, "web_p55", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		services: []models.UserService{{
			Name:          "Premium line",
			ServiceID:     990,
			BaseServiceID: 12,
			UserID:        55,
			Status:        "ACTIVE",
			Category:      "premium-antiblock",
			ConfigRaw:     catalogPremiumConfigRaw(squad),
		}},
	}
	h := serveAccountServices(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var wrap struct {
		Services []struct {
			UserServiceID int      `json:"user_service_id"`
			CanConnect    bool     `json:"can_connect"`
			Tier          string   `json:"tier"`
			ConnectApp    string   `json:"connect_app"`
			Badges        []string `json:"badges"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	if len(wrap.Services) != 1 || !wrap.Services[0].CanConnect {
		t.Fatalf("%+v", wrap.Services)
	}
	s0 := wrap.Services[0]
	if s0.Tier != publicTierPremium || s0.ConnectApp != publicConnectHapp || len(s0.Badges) != 3 {
		t.Fatalf("%+v", s0)
	}
}

func TestServeAccountServices_PremiumNotPaid_NoConnect(t *testing.T) {
	cfg := orderStartTestCfg()
	squad := "anti-premium-squad-x"
	cfg.PremiumSquadName = squad
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "p@test.com", 55, "web_p55", time.Hour)
	st := &stubAccountWeb{
		services: []models.UserService{{
			Name:          "Premium line",
			ServiceID:     991,
			BaseServiceID: 12,
			UserID:        55,
			Status:        "NOT PAID",
			ConfigRaw:     catalogPremiumConfigRaw(squad),
		}},
	}
	rec := httptest.NewRecorder()
	serveAccountServices(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil))
	var wrap struct {
		Services []struct {
			CanConnect bool   `json:"can_connect"`
			Tier       string `json:"tier"`
		} `json:"services"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wrap)
	if len(wrap.Services) != 1 || wrap.Services[0].CanConnect || wrap.Services[0].Tier != publicTierPremium {
		t.Fatalf("%+v", wrap.Services)
	}
}

func TestServeAccountConnect_Premium_OK_WithSignedLink(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.PremiumSquadName = "ps-web"
	cfg.PremiumConnectBaseURL = "https://shop.example/premium-connect"
	cfg.PremiumLinkSigningSecret = "signing-signing-xx"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	us := &models.UserService{
		UserID:        10,
		ServiceID:     442,
		BaseServiceID: 9,
		Status:        "ACTIVE",
		Category:      "premium-antiblock",
		ConfigRaw:     catalogPremiumConfigRaw("ps-web"),
		KeyMarzban:    models.UserKeyMarzban{SubscriptionURL: "https://raw-subscription.example/secret-sub"},
	}
	st := &stubAccountWeb{single: map[int]*models.UserService{442: us}}
	rec := httptest.NewRecorder()
	serveAccountServiceConnect(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=442", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var out accountConnectOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || out.ConnectApp != publicConnectHapp {
		t.Fatalf("%#v", out)
	}
	if !strings.Contains(out.ConnectURL, "premium-connect") || !strings.Contains(out.ConnectURL, "access_token") {
		t.Fatalf("url: %q", out.ConnectURL)
	}
	if strings.Contains(rec.Body.String(), "raw-subscription.example") {
		t.Fatal("must not expose raw subscription in JSON")
	}
}

func TestServeAccountConnect_Premium_NoSecret_NotRawSubscription(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.PremiumSquadName = "ps-web"
	cfg.PremiumConnectBaseURL = "https://shop.example/pc"
	cfg.PremiumLinkSigningSecret = ""
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "me@test.com", 10, "web_aa", time.Hour)
	us := &models.UserService{
		UserID:     10,
		ServiceID:  442,
		Status:     "ACTIVE",
		Category:   "premium-antiblock",
		ConfigRaw:  catalogPremiumConfigRaw("ps-web"),
		KeyMarzban: models.UserKeyMarzban{SubscriptionURL: "https://leak-this.example/forbidden"},
	}
	st := &stubAccountWeb{single: map[int]*models.UserService{442: us}}
	rec := httptest.NewRecorder()
	serveAccountServiceConnect(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/service/connect?token="+tok+"&user_service_id=442", nil))
	raw := rec.Body.String()
	var out accountConnectOKJSON
	_ = json.Unmarshal([]byte(raw), &out)
	if out.ConnectURL != "" || strings.Contains(raw, "leak-this.example") || out.Status != "not_ready" {
		t.Fatalf("%s | %#v", raw, out)
	}
}

func TestServeAccountServices_TelegramUsername(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "tg@example.com", 12, "web_tg12", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		balance: &models.UserBalance{Balance: 1, Forecast: 0},
		getUserByIDRet: &models.User{
			ID: 12,
			Settings: models.UserSettings{
				Telegram: models.TelegramInfo{Username: "vpn_friend", ChatID: 555},
			},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServices(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if st.getUserByIDCalls != 1 || st.getUserByIDArg != 12 {
		t.Fatalf("GetUserByID calls=%d arg=%d", st.getUserByIDCalls, st.getUserByIDArg)
	}
	var env accountServicesOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.User.TelegramLinked || env.User.TelegramUsername != "@vpn_friend" || env.User.TelegramChatID != 0 {
		t.Fatalf("user %+v", env.User)
	}
	if strings.Contains(rec.Body.String(), `"settings"`) {
		t.Fatal("must not expose raw settings")
	}
}

func TestServeAccountServices_TelegramChatIDOnly(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "id@example.com", 13, "web_tg13", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		balance: &models.UserBalance{},
		getUserByIDRet: &models.User{
			Settings: models.UserSettings{Telegram: models.TelegramInfo{ChatID: 919191}},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServices(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var env accountServicesOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.User.TelegramLinked || env.User.TelegramUsername != "" || env.User.TelegramChatID != 919191 {
		t.Fatalf("user %+v", env.User)
	}
}

func TestServeAccountServices_TelegramNotLinkedWebOnly(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "web@example.com", 14, "web_only14", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{
		balance: &models.UserBalance{},
		getUserByIDRet: &models.User{
			Settings: models.UserSettings{Web: models.WebInfo{Email: "web@example.com"}},
		},
	}
	rec := httptest.NewRecorder()
	serveAccountServices(cfg, st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var env accountServicesOKJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.User.TelegramLinked || env.User.TelegramUsername != "" || env.User.TelegramChatID != 0 {
		t.Fatalf("user %+v", env.User)
	}
}

func TestServeAccountServices_BalanceFailed(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 50, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := &stubAccountWeb{balanceErr: errors.New("shm down")}
	h := serveAccountServices(cfg, st)
	req := httptest.NewRequest(http.MethodGet, "/api/account/services?token="+tok, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "balance_failed")
}

func TestServeAccountBalanceTopup_InvalidToken(t *testing.T) {
	cfg := orderStartTestCfg()
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(`{"token":"","amount":150}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountBalanceTopup_InvalidAmount(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example.com"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 51, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	for _, body := range []string{
		`{"token":"` + tok + `","amount":49}`,
		`{"token":"` + tok + `","amount":10000.01}`,
		`{"token":"` + tok + `","amount":150.001}`,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(body))
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400 got %d %s for %s", rec.Code, rec.Body.String(), body)
		}
		assertJSONErrorField(t, rec.Body.String(), "invalid_amount")
	}
}

func TestServeAccountBalanceTopup_SuccessPaymentURL(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.fix.test"
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "a@b.c", 701, "web_xx", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	body := `{"token":"` + tok + `","amount":150}`
	req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out accountBalanceTopupOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "payment_required" || out.Amount != 150 || out.PaymentURL == "" {
		t.Fatalf("%#v", out)
	}
	if !strings.Contains(out.PaymentURL, "yookassa.cgi") || !strings.Contains(out.PaymentURL, "701") || !strings.Contains(out.PaymentURL, "amount=150") {
		t.Fatal(out.PaymentURL)
	}
}

func TestServeAccountBalanceTopup_PaymentURLFailed_EmptyAPIBase(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = ""
	tok, err := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "z@z.z", 2, "web_yy", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := serveAccountBalanceTopup(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodPost, "/api/account/balance/topup", strings.NewReader(`{"token":"`+tok+`","amount":100}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d %s", rec.Code, rec.Body.String())
	}
	assertJSONErrorField(t, rec.Body.String(), "payment_url_failed")
}
