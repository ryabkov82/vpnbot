package web

import (
	_ "embed"
	"log"
	"net/http"
	"strconv"
)

//go:embed static/buy/status.html
var buyStatusPageHTML []byte

func serveBuyStatus(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch path {
	case "/buy/status", "/buy/status/":
	default:
		http.NotFound(w, r)
		return
	}

	log.Printf("buy/status: %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(buyStatusPageHTML)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buyStatusPageHTML)
}
