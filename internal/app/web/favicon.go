package web

import (
	_ "embed"
	"net/http"
	"strconv"
)

//go:embed static/favicon.ico
var faviconICO []byte

//go:embed static/favicon-32x32.png
var favicon32PNG []byte

//go:embed static/apple-touch-icon.png
var appleTouchIconPNG []byte

func serveEmbeddedAsset(contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=604800")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
