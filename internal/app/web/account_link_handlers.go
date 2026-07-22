package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/email"
	"github.com/ryabkov82/vpnbot/internal/models"
	appService "github.com/ryabkov82/vpnbot/internal/service"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// isWebLinkedTelegramUser — email уже привязан к Telegram-пользователю активного бренда:
// settings.web.email + точный login2 == canonical web login + brand membership.
func isWebLinkedTelegramUser(cfg *config.Config, user *models.User) bool {
	if cfg == nil || user == nil {
		return false
	}
	webEmail := strings.TrimSpace(user.Settings.Web.Email)
	if webEmail == "" {
		return false
	}
	normEmail, err := webuser.NormalizeEmail(webEmail)
	if err != nil {
		return false
	}
	prefix := cfg.WebUserLoginPrefix()
	canonical, err := webuser.WebLoginFromEmailWithPrefix(normEmail, prefix)
	if err != nil {
		return false
	}
	if strings.TrimSpace(user.Login2) != canonical {
		return false
	}
	brandID := strings.TrimSpace(cfg.EffectiveBrand().ID)
	stored := strings.TrimSpace(user.Settings.BrandID)
	if brandID == "" {
		return false
	}
	if brandID == "vff" {
		return stored == "vff" || stored == ""
	}
	return stored == brandID
}

func standaloneLinkNoticePage(title string, paragraphs ...string) []byte {
	var body strings.Builder
	body.WriteString(`<!doctype html>
<html lang="ru" data-bs-theme="dark">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>`)
	body.WriteString(html.EscapeString(title))
	body.WriteString(` — VPN for Friends</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css">
<style>[data-bs-theme='dark'] { --bs-body-bg: #282a36; }</style>
</head>
<body class="p-4">
<div class="container" style="max-width:560px;">
<h1 class="h4 mb-3">`)
	body.WriteString(html.EscapeString(title))
	body.WriteString(`</h1>
`)
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		body.WriteString(`<p class="text-secondary mb-3">`)
		body.WriteString(html.EscapeString(p))
		body.WriteString(`</p>`)
	}
	body.WriteString(`<p><a href="/account">Страница входа</a></p>
</div>
</body></html>`)
	return []byte(body.String())
}

func serveAccountLink(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/link" && r.URL.Path != "/account/link/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			http.NotFound(w, r)
			return
		}

		token := strings.TrimSpace(r.URL.Query().Get("token"))
		errQS := strings.TrimSpace(r.URL.Query().Get("err"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		var body []byte
		switch {
		case strings.TrimSpace(token) != "":
			secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
			claims, err := VerifyAccountTelegramLinkToken(secret, token)
			if err != nil {
				body = accountLinkInvalidHTML
				break
			}
			shu, errGU := app.GetUserByID(claims.ShmUserID)
			if errGU != nil {
				slog.Error("account link", "stage", "get_user_by_id", "user_id", claims.ShmUserID, "err", errGU)
				body = accountLinkInvalidHTML
				break
			}
			if shu == nil || shu.Settings.Telegram.ChatID != claims.TelegramChatID {
				body = accountLinkInvalidHTML
				break
			}
			if isWebLinkedTelegramUser(cfg, shu) {
				normEmail, nerr := webuser.NormalizeEmail(shu.Settings.Web.Email)
				if nerr != nil || strings.TrimSpace(shu.Login) == "" {
					qs := strconv.Quote(token)
					body = bytes.ReplaceAll(accountLinkStartHTML, []byte("__GO_JS_STRING__"), []byte(qs))
					break
				}
				rawTok, terr := CreateAccountToken(secret, normEmail, shu.ID, shu.Login, accountTokenTTL(cfg))
				if terr != nil {
					slog.Error("account link", "stage", "create_session_token", "user_id", shu.ID, "err", terr)
					http.Redirect(w, r, "/account/link?"+url.Values{"err": []string{"token_failed"}}.Encode(), http.StatusFound)
					return
				}
				slog.Info("account link: already linked, redirecting to session",
					"user_id", shu.ID, "login2_present", true, "web_email_present", true)
				http.Redirect(w, r, "/account/session?token="+url.QueryEscape(rawTok), http.StatusFound)
				return
			}
			qs := strconv.Quote(token)
			body = bytes.ReplaceAll(accountLinkStartHTML, []byte("__GO_JS_STRING__"), []byte(qs))

		default:
			switch errQS {
			case "google_email_conflict":
				body = accountLinkStandaloneConflictHTML
			case "email_used_by_other":
				body = accountLinkStandaloneConflictHTML
			case "already_linked":
				body = standaloneLinkNoticePage(
					"Привязка кабинета",
					"К этому аккаунту уже привязан другой email. Откройте личный кабинет заново из Telegram-бота или напишите в поддержку.")
			case "telegram_mismatch":
				body = standaloneLinkNoticePage(
					"Привязка кабинета",
					"Данные сессии Telegram не совпали. Откройте новую ссылку из Telegram-бота.")
			case "bad_user":
				body = standaloneLinkNoticePage("Привязка кабинета", "Не удалось завершить привязку. Попробуйте заново из Telegram-бота.")
			case "token_failed":
				body = standaloneLinkNoticePage("Привязка кабинета", "Не удалось выдать сессию. Попробуйте заново через бота или обычный вход на сайте.")
			case "shm_login2_not_persisted":
				body = standaloneLinkNoticePage(
					"Привязка кабинета",
					"Не удалось завершить привязку аккаунта в биллинге. Попробуйте еще раз позже или обратитесь в поддержку.")
			case "link_failed":
				body = standaloneLinkNoticePage("Привязка кабинета", "Не удалось сохранить привязку. Попробуйте позже или напишите в поддержку.")
			case "invalid_confirm_token", "expired_confirm":
				body = accountLinkInvalidHTML
			default:
				body = accountLinkInvalidHTML
			}
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func serveAccountLinkConfirm(cfg *config.Config, app accountWebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/link/confirm" && r.URL.Path != "/account/link/confirm/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			http.NotFound(w, r)
			return
		}

		raw := strings.TrimSpace(r.URL.Query().Get("token"))
		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		claims, err := VerifyAccountLinkEmailToken(secret, raw)
		if err != nil {
			http.Redirect(w, r, "/account/link"+linkQueryForErr(err), http.StatusFound)
			return
		}

		u, err := app.LinkWebEmailForTelegramUser(claims.ShmUserID, claims.TelegramChatID, claims.Email, "telegram_link")
		switch {
		case err == nil:
			break
		case errors.Is(err, appService.ErrWebEmailAlreadyLinked):
			http.Redirect(w, r, "/account/link"+linkErrQuery("already_linked"), http.StatusFound)
			return
		case errors.Is(err, appService.ErrWebEmailUsedByOtherAccount):
			if wlConflict, werr := webuser.WebLoginFromEmailWithPrefix(strings.TrimSpace(claims.Email), cfg.WebUserLoginPrefix()); werr != nil {
				slog.Warn("link confirm: email already linked to another user",
					"shm_user_id", claims.ShmUserID)
			} else {
				slog.Warn("link confirm: email already linked to another user",
					"shm_user_id", claims.ShmUserID, "web_login", wlConflict)
			}
			linkTok, tokErr := CreateAccountTelegramLinkToken(secret, claims.ShmUserID, claims.TelegramChatID, cfg)
			if tokErr != nil {
				slog.Error("link confirm", "stage", "recreate_link_token", "shm_user_id", claims.ShmUserID, "err", tokErr)
				http.Redirect(w, r, "/account/link"+linkErrQuery("link_failed"), http.StatusFound)
				return
			}
			respondLinkEmailAlreadyLinked(w, r, linkTok)
			return
		case errors.Is(err, appService.ErrTelegramChatMismatch):
			http.Redirect(w, r, "/account/link"+linkErrQuery("telegram_mismatch"), http.StatusFound)
			return
		case errors.Is(err, appService.ErrWebLogin2NotPersisted):
			slog.Error("link confirm", "stage", "login2_not_persisted", "shm_user_id", claims.ShmUserID, "err", err)
			http.Redirect(w, r, "/account/link"+linkErrQuery("shm_login2_not_persisted"), http.StatusFound)
			return
		default:
			slog.Error("link confirm", "stage", "link_web_email", "shm_user_id", claims.ShmUserID, "err", err)
			http.Redirect(w, r, "/account/link"+linkErrQuery("link_failed"), http.StatusFound)
			return
		}
		normEmail, nerr := webuser.NormalizeEmail(claims.Email)
		if nerr != nil || u == nil {
			http.Redirect(w, r, "/account/link"+linkErrQuery("bad_user"), http.StatusFound)
			return
		}

		acTok, err := CreateAccountToken(secret, normEmail, u.ID, u.Login, accountTokenTTL(cfg))
		if err != nil {
			slog.Error("link confirm", "stage", "create_session_token", "user_id", u.ID, "err", err)
			http.Redirect(w, r, "/account/link"+linkErrQuery("token_failed"), http.StatusFound)
			return
		}
		dest := "/account/session?token=" + url.QueryEscape(acTok)
		http.Redirect(w, r, dest, http.StatusFound)
	}
}

func linkErrQuery(code string) string {
	return "?err=" + url.QueryEscape(code)
}

func linkQueryForErr(tokErr error) string {
	switch {
	case errors.Is(tokErr, ErrAccountTokenExpired):
		return "?err=" + url.QueryEscape("expired_confirm")
	default:
		return "?err=" + url.QueryEscape("invalid_confirm_token")
	}
}

type accountLinkLoginStartReq struct {
	Email     string `json:"email"`
	LinkToken string `json:"link_token"`
}

func serveAccountLinkLoginStart(cfg *config.Config, app accountWebApp, rl *leadRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/link/login/start" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !webSalesTokenFlowAvailable(cfg) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req accountLinkLoginStartReq
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		linkClaims, err := VerifyAccountTelegramLinkToken(secret, strings.TrimSpace(req.LinkToken))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_link_token")
			return
		}

		normEmail, err := webuser.NormalizeEmail(req.Email)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_email")
			return
		}

		ipKey := ClientIPFromRequest(r)
		if ipKey == "" {
			ipKey = "unknown"
		}
		if !rl.allow(ipKey, strings.ToLower(normEmail)) {
			writeJSONError(w, http.StatusTooManyRequests, "rate_limited")
			return
		}

		if !email.IsConfigured(cfg) {
			writeJSONError(w, http.StatusServiceUnavailable, "email_unavailable")
			return
		}

		other, err := app.FindUserByWebEmail(normEmail)
		if err != nil {
			slog.Error("link login", "stage", "find_user_by_web_login", "user_id", linkClaims.ShmUserID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if other != nil && other.ID != linkClaims.ShmUserID {
			slog.Warn("link login: email already linked to another user",
				"link_user_id", linkClaims.ShmUserID, "other_user_id", other.ID)
			writeJSONError(w, http.StatusConflict, accountErrorEmailAlreadyLinked)
			return
		}

		emailTok, err := CreateAccountLinkEmailToken(secret, linkClaims.ShmUserID, linkClaims.TelegramChatID, normEmail, cfg)
		if err != nil {
			slog.Error("link login", "stage", "create_link_email_token", "user_id", linkClaims.ShmUserID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		base := strings.TrimRight(strings.TrimSpace(publicOrderBaseURL(cfg, r)), "/")
		if base == "" {
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		linkURL := base + "/account/link/confirm?token=" + url.QueryEscape(emailTok)
		if err := email.SendAccountLinkConfirmEmail(cfg, normEmail, linkURL); err != nil {
			if errors.Is(err, email.ErrNotConfigured) {
				writeJSONError(w, http.StatusServiceUnavailable, "email_unavailable")
				return
			}
			slog.Error("link login", "stage", "send_confirm_email", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "email_send_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "email_sent"})
	}
}
