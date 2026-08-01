package web

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

//go:embed static/account/account.css
var accountCSS []byte

var accountCSSETag = computeAccountCSSETag()

func computeAccountCSSETag() string {
	sum := sha256.Sum256(accountCSS)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func accountCSSIfNoneMatch(header string) bool {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		if strings.HasPrefix(tag, "W/") {
			tag = strings.TrimSpace(tag[2:])
		}
		if tag == accountCSSETag || tag == "*" {
			return true
		}
	}
	return false
}

// serveAccountCSS serves the shared embedded account stylesheet for all brands.
func serveAccountCSS(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("ETag", accountCSSETag)

	if accountCSSIfNoneMatch(r.Header.Get("If-None-Match")) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(accountCSS)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(accountCSS)
	}
}
