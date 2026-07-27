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

	"github.com/ryabkov82/vpnbot/internal/attribution"
	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	appService "github.com/ryabkov82/vpnbot/internal/service"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

const googleOAuthStartMaxBodyBytes = 32 << 10

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

// resolveGoogleOAuthRedirectURL строит redirect_uri для Google OAuth по текущему request Host.
// GoogleRedirectURL используется как шаблон (scheme + path); host заменяется на разрешённый
// hostname из r.Host. X-Forwarded-Host не учитывается. Неизвестный Host — ошибка без fallback.
func resolveGoogleOAuthRedirectURL(cfg *config.Config, r *http.Request) (string, error) {
	if cfg == nil || r == nil {
		return "", errGoogleOAuthInvalidHost{}
	}
	template := strings.TrimSpace(cfg.WebAccount.GoogleRedirectURL)
	if template == "" {
		return "", errGoogleOAuthMisconfigured{}
	}
	host, ok := cfg.EffectiveBrand().MatchAllowedHost(r.Host)
	if !ok {
		return "", errGoogleOAuthInvalidHost{}
	}
	u, err := url.Parse(template)
	if err != nil {
		return "", errGoogleOAuthMisconfigured{}
	}
	if u.Scheme == "" || strings.TrimSpace(u.Host) == "" {
		return "", errGoogleOAuthMisconfigured{}
	}
	if strings.TrimSpace(u.Path) == "" || u.Path == "/" {
		return "", errGoogleOAuthMisconfigured{}
	}
	out := &url.URL{
		Scheme: u.Scheme,
		Host:   host,
		Path:   u.Path,
	}
	return out.String(), nil
}

func buildGoogleOAuthURL(cfg *config.Config, state, redirectURL string) (string, error) {
	if cfg == nil {
		return "", errGoogleOAuthMisconfigured{}
	}
	cid := strings.TrimSpace(cfg.WebAccount.GoogleClientID)
	redirect := strings.TrimSpace(redirectURL)
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

type errGoogleOAuthInvalidHost struct{}

func (errGoogleOAuthInvalidHost) Error() string {
	return "google oauth invalid host"
}

type googleOAuthTokenJSON struct {
	AccessToken string `json:"access_token"`
}

func exchangeGoogleOAuthCode(ctx context.Context, hc *http.Client, cfg *config.Config, code, redirectURL string) (accessToken string, err error) {
	if hc == nil {
		hc = googleOAuthHTTPClient()
	}
	if cfg == nil {
		return "", errGoogleOAuthMisconfigured{}
	}
	redirect := strings.TrimSpace(redirectURL)
	if redirect == "" {
		return "", errGoogleOAuthMisconfigured{}
	}
	form := url.Values{}
	form.Set("code", strings.TrimSpace(code))
	form.Set("client_id", strings.TrimSpace(cfg.WebAccount.GoogleClientID))
	form.Set("client_secret", strings.TrimSpace(cfg.WebAccount.GoogleClientSecret))
	form.Set("redirect_uri", redirect)
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
		switch r.Method {
		case http.MethodGet, http.MethodPost:
		default:
			w.Header().Set("Allow", "GET, POST")
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
		redirectURL, err := resolveGoogleOAuthRedirectURL(cfg, r)
		if err != nil {
			var inv errGoogleOAuthInvalidHost
			if errors.As(err, &inv) {
				writeJSONError(w, http.StatusBadRequest, "invalid_host")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		sec := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		brandID := cfgBrandID(cfg)
		capturedAt := time.Now()
		var state string

		switch r.Method {
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, googleOAuthStartMaxBodyBytes)
			if err := r.ParseForm(); err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					writeJSONError(w, http.StatusRequestEntityTooLarge, "bad_request")
					return
				}
				writeJSONError(w, http.StatusBadRequest, "bad_request")
				return
			}
			if strings.TrimSpace(r.Form.Get("link_token")) != "" {
				writeJSONError(w, http.StatusBadRequest, "bad_request")
				return
			}
			clearGoogleOAuthLinkTokenCookie(w, r)
			if lang := strings.TrimSpace(r.Form.Get("lang")); lang != "" {
				setAccountLangCookie(w, r, normalizeAccountLocale(lang))
			}
			marketing := attribution.MarketingInput{
				LandingPath: r.Form.Get("landing_path"),
				Referrer:    r.Form.Get("referrer"),
				UTMSource:   r.Form.Get("utm_source"),
				UTMMedium:   r.Form.Get("utm_medium"),
				UTMCampaign: r.Form.Get("utm_campaign"),
				UTMContent:  r.Form.Get("utm_content"),
				UTMTerm:     r.Form.Get("utm_term"),
			}
			state, _, err = createGoogleOAuthLoginStateForStart(cfg, marketing, capturedAt)
			if err != nil {
				slog.Error("google oauth start: login state", "err", err)
				writeJSONError(w, http.StatusInternalServerError, "internal_error")
				return
			}

		case http.MethodGet:
			linkQS := strings.TrimSpace(r.URL.Query().Get("link_token"))
			if linkQS != "" {
				if _, verr := VerifyAccountTelegramLinkToken(sec, brandID, linkQS); verr != nil {
					writeJSONError(w, http.StatusBadRequest, "invalid_link_token")
					return
				}
				setGoogleOAuthLinkTokenCookie(w, r, linkQS)
				state, err = createGoogleOAuthLinkState(sec, brandID, googleOAuthStateTTL())
				if err != nil {
					slog.Error("google oauth start: link state", "err", err)
					writeJSONError(w, http.StatusInternalServerError, "internal_error")
					return
				}
			} else {
				clearGoogleOAuthLinkTokenCookie(w, r)
				marketing := attribution.MarketingInput{
					LandingPath: "/account",
					Referrer:    strings.TrimSpace(r.Referer()),
				}
				state, _, err = createGoogleOAuthLoginStateForStart(cfg, marketing, capturedAt)
				if err != nil {
					slog.Error("google oauth start: login state", "err", err)
					writeJSONError(w, http.StatusInternalServerError, "internal_error")
					return
				}
			}
			if qLang := strings.TrimSpace(r.URL.Query().Get("lang")); qLang != "" {
				setAccountLangCookie(w, r, normalizeAccountLocale(qLang))
			}
		}

		loc, err := buildGoogleOAuthURL(cfg, state, redirectURL)
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
		redirectURL, err := resolveGoogleOAuthRedirectURL(cfg, r)
		if err != nil {
			var inv errGoogleOAuthInvalidHost
			if errors.As(err, &inv) {
				writeJSONError(w, http.StatusBadRequest, "invalid_host")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
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

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		brandID := cfgBrandID(cfg)
		var (
			signedClaims *googleOAuthStateClaims
			legacyState  bool
		)
		if strings.HasPrefix(stateQS, googleOAuthStatePrefix) {
			claims, serr := parseAndVerifyGoogleOAuthState(secret, brandID, stateQS)
			if serr != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_state")
				return
			}
			signedClaims = claims
		} else if isLegacyGoogleOAuthState(stateQS) {
			legacyState = true
		} else {
			writeJSONError(w, http.StatusBadRequest, "invalid_state")
			return
		}

		hasLinkCookie := strings.TrimSpace(linkCookie) != ""
		if hasLinkCookie {
			if signedClaims != nil && (signedClaims.Mode != googleOAuthModeLink || signedClaims.Attribution != nil) {
				writeJSONError(w, http.StatusBadRequest, "invalid_state")
				return
			}
		} else {
			if signedClaims != nil && (signedClaims.Mode != googleOAuthModeLogin || signedClaims.Attribution == nil) {
				writeJSONError(w, http.StatusBadRequest, "invalid_state")
				return
			}
		}

		if code == "" {
			writeJSONError(w, http.StatusBadRequest, "google_auth_failed")
			return
		}

		ctx := r.Context()
		hc := googleOAuthHTTPClient()
		acTok, err := exchangeGoogleOAuthCode(ctx, hc, cfg, code, redirectURL)
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

		if hasLinkCookie {
			linkClaims, lerr := VerifyAccountTelegramLinkToken(secret, brandID, linkCookie)
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
				if errors.Is(ferr, appService.ErrUserIdentityMismatch) {
					slog.Warn("google oauth link: identity mismatch", "user_id", linkClaims.ShmUserID)
					respondLinkEmailAlreadyLinked(w, r, linkCookie)
					return
				}
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
			case errors.Is(linkErr, appService.ErrUserIdentityMismatch):
				slog.Warn("google oauth link: link identity mismatch", "user_id", linkClaims.ShmUserID)
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"link_failed"}}.Encode(), http.StatusFound)
				return
			case errors.Is(linkErr, appService.ErrWebEmailAlreadyLinked):
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"already_linked"}}.Encode(), http.StatusFound)
				return
			case errors.Is(linkErr, appService.ErrWebEmailUsedByOtherAccount):
				if wlRecheck, werr := webuser.WebLoginFromEmailWithPrefix(normEmail, cfg.WebUserLoginPrefix()); werr != nil {
					slog.Warn("google oauth link: email already linked to another user",
						"link_user_id", linkClaims.ShmUserID)
				} else {
					slog.Warn("google oauth link: email already linked to another user",
						"link_user_id", linkClaims.ShmUserID, "web_login", wlRecheck)
				}
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
			rawSessionTok, err := CreateAccountToken(secret, brandID, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
			if err != nil {
				slog.Error("google oauth link", "stage", "create_session_token", "user_id", user.ID, "err", err)
				http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"token_failed"}}.Encode(), http.StatusFound)
				return
			}
			slog.Info("google oauth link: linked and redirecting",
				"user_id", user.ID, "mode", googleOAuthModeLink, "duration_ms", linkDoneMs)
			sessionURL := appendAccountLangQuery("/account/session?token="+url.QueryEscape(rawSessionTok), resolveAccountLocale(r))
			http.Redirect(w, r, sessionURL, http.StatusFound)
			return
		}

		var (
			user    *models.User
			created bool
			ferr    error
		)
		if signedClaims != nil {
			user, created, ferr = app.FindOrCreateWebUserWithAttribution(normEmail, *signedClaims.Attribution)
		} else {
			user, created, ferr = app.FindOrCreateWebUser(normEmail)
		}
		if ferr != nil || user == nil {
			if errors.Is(ferr, appService.ErrUserIdentityMismatch) {
				slog.Warn("google oauth callback: identity mismatch")
				writeJSONError(w, http.StatusForbidden, "google_auth_failed")
				return
			}
			slog.Error("google oauth callback", "stage", "find_or_create_web_user", "err", ferr)
			writeJSONError(w, http.StatusInternalServerError, "web_user_failed")
			return
		}

		rawSessionTok, err := CreateAccountToken(secret, brandID, normEmail, user.ID, user.Login, accountTokenTTL(cfg))
		if err != nil {
			slog.Error("google oauth callback", "stage", "create_session_token", "user_id", user.ID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		if created {
			sendAccountUserRegisteredTelegramNotification(cfg, normEmail, user.ID, user.Login, ClientIPFromRequest(r))
		}

		mode := googleOAuthModeLogin
		if signedClaims != nil {
			mode = signedClaims.Mode
		} else if legacyState {
			mode = "legacy"
		}
		slog.Info("google oauth callback: session ready",
			"user_id", user.ID, "created", created, "mode", mode)

		redirect := appendAccountLangQuery("/account/session?token="+url.QueryEscape(rawSessionTok), resolveAccountLocale(r))
		http.Redirect(w, r, redirect, http.StatusFound)
	}
}
