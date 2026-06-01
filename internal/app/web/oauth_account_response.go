package web

import (
	"net/http"
	"net/url"
	"strings"
)

// accountErrorEmailAlreadyLinked — значение query ?error= и поле JSON error для конфликта Telegram↔Web (email уже у другого SHM user).
const accountErrorEmailAlreadyLinked = "email_already_linked"

func accountPrefersJSONResponse(r *http.Request) bool {
	if r == nil {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	return strings.Contains(a, "application/json")
}

// respondAccountEmailAlreadyLinked — обычный web login: 409 JSON или 302 на /account?error=email_already_linked.
func respondAccountEmailAlreadyLinked(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if accountPrefersJSONResponse(r) {
		writeJSONError(w, http.StatusConflict, accountErrorEmailAlreadyLinked)
		return
	}
	q := url.Values{}
	q.Set("error", accountErrorEmailAlreadyLinked)
	http.Redirect(w, r, "/account?"+q.Encode(), http.StatusFound)
}

// respondLinkEmailAlreadyLinked — Telegram→Web link-flow: 302 на /account/link?token=…&err=email_already_linked.
func respondLinkEmailAlreadyLinked(w http.ResponseWriter, r *http.Request, linkToken string) {
	w.Header().Set("Cache-Control", "no-store")
	linkToken = strings.TrimSpace(linkToken)
	q := url.Values{}
	if linkToken != "" {
		q.Set("token", linkToken)
	}
	q.Set("err", accountErrorEmailAlreadyLinked)
	http.Redirect(w, r, "/account/link?"+q.Encode(), http.StatusFound)
}
