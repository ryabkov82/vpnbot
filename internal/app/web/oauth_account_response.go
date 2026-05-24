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

// respondAccountEmailAlreadyLinked завершает запрос без дальнейшей логики OAuth/link: 409 JSON или 302 на /account.
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
