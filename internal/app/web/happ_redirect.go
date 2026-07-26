package web

import (
	_ "embed"
	"net/http"
	"strconv"
	"strings"
)

//go:embed static/premium-connect/redirect.html
var happRedirectHTML []byte

const happURLSchemePrefix = "happ://"

func isValidHappRedirectTarget(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), happURLSchemePrefix)
}

func serveHappRedirect(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/redirect.html" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	target := strings.TrimSpace(r.URL.Query().Get("url"))
	if !isValidHappRedirectTarget(target) {
		http.Error(w, "Invalid Happ URL", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(happRedirectHTML)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(happRedirectHTML)
}
