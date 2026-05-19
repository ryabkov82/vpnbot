package web

import (
	_ "embed"
	"log"
	"net/http"
	"strconv"
)

//go:embed static/buy/index.html
var buyPageHTML []byte

func serveBuy(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch path {
	case "/buy", "/buy/":
	default:
		http.NotFound(w, r)
		return
	}

	log.Printf("buy: %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(buyPageHTML)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buyPageHTML)
}
