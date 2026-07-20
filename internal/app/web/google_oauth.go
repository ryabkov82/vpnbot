package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	appService "github.com/ryabkov82/vpnbot/internal/service"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

const (
	googleOAuthAuthURL    = "https://accounts.google.com/o/oauth2/v2/auth"
	googleOAuthCookieName = "vff_google_oauth_state"

	googleOAuthCookieLinkToken = "vff_google_oauth_link_token"

	googleOAuthDefaultTokenURL    = "https://oauth2.googleapis.com/token"
	googleOAuthDefaultUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

	googleOAuthCookieMaxAgeSecs = 600 // 10 minutes
)

// googleOAuthTokenURLOverride и googleOAuthUserinfoURLOverride подменяют endpoint'ы только в тестах.
var (
	googleOAuthTokenURLOverride    string
	googleOAuthUserinfoURLOverride string
)

func resolvedGoogleOAuthTokenURL() string {
	if strings.TrimSpace(googleOAuthTokenURLOverride) != "" {
		return googleOAuthTokenURLOverride
	}
	return googleOAuthDefaultTokenURL
}

func resolvedGoogleOAuthUserinfoURL() string {
	if strings.TrimSpace(googleOAuthUserinfoURLOverride) != "" {
		return googleOAuthUserinfoURLOverride
	}
	return googleOAuthDefaultUserinfoURL
}

func googleOAuthHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// googleOAuthAvailable возвращает true, когда Google OAuth настроен и может использоваться безопасно.
func googleOAuthAvailable(cfg *config.Config) bool {
	if cfg == nil || !cfg.WebAccount.GoogleEnabled {
		return false
	}
	a := cfg.WebAccount
	if strings.TrimSpace(a.GoogleClientID) == "" {
		return false
	}
	if strings.TrimSpace(a.GoogleClientSecret) == "" {
		return false
	}
	if strings.TrimSpace(a.GoogleRedirectURL) == "" {
		return false
	}
	return true
}

func newGoogleOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func requestLikelyHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func setGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	c := &http.Cookie{
		Name:     googleOAuthCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   googleOAuthCookieMaxAgeSecs,
		HttpOnly: true,
		Secure:   requestLikelyHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
}

func readGoogleOAuthStateCookie(r *http.Request) string {
	c, err := r.Cookie(googleOAuthCookieName)
	if err != nil || c == nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func clearGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestLikelyHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func setGoogleOAuthLinkTokenCookie(w http.ResponseWriter, r *http.Request, linkToken string) {
	c := &http.Cookie{
		Name:     googleOAuthCookieLinkToken,
		Value:    strings.TrimSpace(linkToken),
		Path:     "/",
		MaxAge:   googleOAuthCookieMaxAgeSecs,
		HttpOnly: true,
		Secure:   requestLikelyHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
}

func readGoogleOAuthLinkTokenCookie(r *http.Request) string {
	c, err := r.Cookie(googleOAuthCookieLinkToken)
	if err != nil || c == nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func clearGoogleOAuthLinkTokenCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthCookieLinkToken,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestLikelyHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func buildGoogleOAuthURL(cfg *config.Config, state string) (string, error) {
	if cfg == nil {
		return "", errGoogleOAuthMisconfigured{}
	}
	cid := strings.TrimSpace(cfg.WebAccount.GoogleClientID)
	redirect := strings.TrimSpace(cfg.WebAccount.GoogleRedirectURL)
	if cid == "" || redirect == "" || strings.TrimSpace(state) == "" {
		return "", errGoogleOAuthMisconfigured{}
	}
	u, err := url.Parse(googleOAuthAuthURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", cid)
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("access_type", "online")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type errGoogleOAuthMisconfigured struct{}

func (errGoogleOAuthMisconfigured) Error() string {
	return "google oauth misconfigured"
}

type googleOAuthTokenJSON struct {
	AccessToken string `json:"access_token"`
}

func exchangeGoogleOAuthCode(ctx context.Context, hc *http.Client, cfg *config.Config, code string) (accessToken string, err error) {
	if hc == nil {
		hc = googleOAuthHTTPClient()
	}
	if cfg == nil {
		return "", errGoogleOAuthMisconfigured{}
	}
	form := url.Values{}
	form.Set("code", strings.TrimSpace(code))
	form.Set("client_id", strings.TrimSpace(cfg.WebAccount.GoogleClientID))
	form.Set("client_secret", strings.TrimSpace(cfg.WebAccount.GoogleClientSecret))
	form.Set("redirect_uri", strings.TrimSpace(cfg.WebAccount.GoogleRedirectURL))
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolvedGoogleOAuthTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("google oauth token exchange rejected", "status", resp.StatusCode)
		return "", errGoogleTokenExchangeRejected{}
	}
	var tj googleOAuthTokenJSON
	if json.Unmarshal(body, &tj) != nil || strings.TrimSpace(tj.AccessToken) == "" {
		slog.Warn("google oauth token response invalid")
		return "", errGoogleTokenExchangeRejected{}
	}
	return strings.TrimSpace(tj.AccessToken), nil
}

type errGoogleTokenExchangeRejected struct{}

func (errGoogleTokenExchangeRejected) Error() string {
	return "google token rejected"
}

func fetchGoogleOAuthUserInfo(ctx context.Context, hc *http.Client, accessToken string) (email string, verified bool, err error) {
	if hc == nil {
		hc = googleOAuthHTTPClient()
	}
	tok := strings.TrimSpace(accessToken)
	if tok == "" {
		return "", false, errGoogleUserinfoRejected{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolvedGoogleOAuthUserinfoURL(), nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := hc.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", false, err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("google oauth userinfo rejected", "status", resp.StatusCode)
		return "", false, errGoogleUserinfoRejected{}
	}
	var uj map[string]any
	if err := json.Unmarshal(body, &uj); err != nil {
		slog.Warn("google oauth userinfo json invalid")
		return "", false, errGoogleUserinfoRejected{}
	}
	emRaw, _ := uj["email"].(string)
	em := strings.TrimSpace(emRaw)
	if em == "" {
		return "", false, errGoogleUserinfoRejected{}
	}
	emailVerified := false
	switch v := uj["email_verified"].(type) {
	case bool:
		emailVerified = v
	case string:
		emailVerified = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return em, emailVerified, nil
}

type errGoogleUserinfoRejected struct{}

func (errGoogleUserinfoRejected) Error() string {
	return "google userinfo rejected"
}

func serveGoogleOAuthStart(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/google/start", "/api/account/google/start/":
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		if !googleOAuthAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "google_auth_unavailable")
			return
		}
		state, err := newGoogleOAuthState()
		if err != nil {
			slog.Error("google oauth start: random state", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		sec := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		linkQS := strings.TrimSpace(r.URL.Query().Get("link_token"))
		if linkQS != "" {
			if _, err := VerifyAccountTelegramLinkToken(sec, linkQS); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_link_token")
				return
			}
			setGoogleOAuthLinkTokenCookie(w, r, linkQS)
		} else {
			clearGoogleOAuthLinkTokenCookie(w, r)
		}
		if qLang := strings.TrimSpace(r.URL.Query().Get("lang")); qLang != "" {
			setAccountLangCookie(w, r, normalizeAccountLocale(qLang))
		}
		loc, err := buildGoogleOAuthURL(cfg, state)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		setGoogleOAuthStateCookie(w, r, state)
		http.Redirect(w, r, loc, http.StatusFound)
	}
}

func serveGoogleOAuthCallback(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/google/callback", "/api/account/google/callback/":
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) || !googleOAuthAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "google_auth_unavailable")
			return
		}

		q := r.URL.Query()
		if strings.TrimSpace(q.Get("error")) != "" {
			clearGoogleOAuthStateCookie(w, r)
			clearGoogleOAuthLinkTokenCookie(w, r)
			writeJSONError(w, http.StatusBadRequest, "google_auth_failed")
			return
		}

		code := strings.TrimSpace(q.Get("code"))
		stateQS := strings.TrimSpace(q.Get("state"))
		cookieState := readGoogleOAuthStateCookie(r)
		linkCookie := readGoogleOAuthLinkTokenCookie(r)
		clearGoogleOAuthStateCookie(w, r)
		clearGoogleOAuthLinkTokenCookie(w, r)
		if cookieState == "" || stateQS == "" || cookieState != stateQS {
			writeJSONError(w, http.StatusBadRequest, "invalid_state")
			return
		}
		if code == "" {
			writeJSONError(w, http.StatusBadRequest, "google_auth_failed")
			return
		}

		ctx := r.Context()
		hc := googleOAuthHTTPClient()
		acTok, err := exchangeGoogleOAuthCode(ctx, hc, cfg, code)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "google_auth_failed")
			return
		}

		emailGoogle, verified, err := fetchGoogleOAuthUserInfo(ctx, hc, acTok)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "google_auth_failed")
			return
		}
		if !verified {
			writeJSONError(w, http.StatusForbidden, "google_email_not_verified")
			return
		}

		normEmail, err := webuser.NormalizeEmail(emailGoogle)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "google_auth_failed")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		if strings.TrimSpace(linkCookie) != "" {
			linkClaims, lerr := VerifyAccountTelegramLinkToken(secret, linkCookie)
			if lerr != nil {
				errCode := "invalid_confirm_token"
				if errors.Is(lerr, ErrAccountTokenExpired) {
					errCode = "expired_confirm"
				}
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{errCode}}.Encode(), http.StatusFound)
				return
			}
			other, ferr := app.FindUserByWebEmail(normEmail)
			if ferr != nil {
				slog.Error("google oauth link", "stage", "find_user_by_web_login", "user_id", linkClaims.ShmUserID, "err", ferr)
				writeJSONError(w, http.StatusInternalServerError, "web_user_failed")
				return
			}
			if other != nil && other.ID != linkClaims.ShmUserID {
				slog.Warn("google oauth link: email already linked to another user",
					"link_user_id", linkClaims.ShmUserID, "other_user_id", other.ID)
				respondLinkEmailAlreadyLinked(w, r, linkCookie)
				return
			}
			linkStarted := time.Now()
			user, linkErr := app.LinkWebEmailForTelegramUser(linkClaims.ShmUserID, linkClaims.TelegramChatID, normEmail, "telegram_link_google")
			switch {
			case linkErr == nil:
				break
			case errors.Is(linkErr, appService.ErrWebEmailAlreadyLinked):
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"already_linked"}}.Encode(), http.StatusFound)
				return
			case errors.Is(linkErr, appService.ErrWebEmailUsedByOtherAccount):
				wlRecheck := webuser.WebLoginFromEmailWithPrefix(normEmail, cfg.WebUserLoginPrefix())
				slog.Warn("google oauth link: email already linked to another user",
					"link_user_id", linkClaims.ShmUserID, "web_login", wlRecheck)
				respondLinkEmailAlreadyLinked(w, r, linkCookie)
				return
			case errors.Is(linkErr, appService.ErrTelegramChatMismatch):
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"telegram_mismatch"}}.Encode(), http.StatusFound)
				return
			case errors.Is(linkErr, appService.ErrWebLogin2NotPersisted):
				slog.Error("google oauth link", "stage", "login2_not_persisted", "user_id", linkClaims.ShmUserID, "err", linkErr)
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"shm_login2_not_persisted"}}.Encode(), http.StatusFound)
				return
			default:
				slog.Error("google oauth link", "stage", "link_web_email", "user_id", linkClaims.ShmUserID, "err", linkErr)
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"link_failed"}}.Encode(), http.StatusFound)
				return
			}
			if user == nil {
				writeJSONError(w, http.StatusInternalServerError, "web_user_failed")
				return
			}
			linkDoneMs := time.Since(linkStarted).Milliseconds()
			rawSessionTok, err := CreateAccountToken(secret, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
			if err != nil {
				slog.Error("google oauth link", "stage", "create_session_token", "user_id", user.ID, "err", err)
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"token_failed"}}.Encode(), http.StatusFound)
				return
			}
			slog.Info("google oauth link: linked and redirecting", "user_id", user.ID, "duration_ms", linkDoneMs)
			sessionURL := appendAccountLangQuery("/account/session?token="+url.QueryEscape(rawSessionTok), resolveAccountLocale(r))
			http.Redirect(w, r, sessionURL, http.StatusFound)
			return
		}

		user, created, ferr := app.FindOrCreateWebUser(normEmail)
		if ferr != nil || user == nil {
			slog.Error("google oauth callback", "stage", "find_or_create_web_user", "err", ferr)
			writeJSONError(w, http.StatusInternalServerError, "web_user_failed")
			return
		}

		rawSessionTok, err := CreateAccountToken(secret, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
		if err != nil {
			slog.Error("google oauth callback", "stage", "create_session_token", "user_id", user.ID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		if created {
			sendAccountUserRegisteredTelegramNotification(cfg, normEmail, user.ID, user.Login, ClientIPFromRequest(r))
		}

		redirect := appendAccountLangQuery("/account/session?token="+url.QueryEscape(rawSessionTok), resolveAccountLocale(r))
		http.Redirect(w, r, redirect, http.StatusFound)
	}
}
