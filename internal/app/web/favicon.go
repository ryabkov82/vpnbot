package web

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

//go:embed static/favicon.ico
var vffFaviconICO []byte

//go:embed static/favicon-32x32.png
var vffFavicon32PNG []byte

//go:embed static/apple-touch-icon.png
var vffAppleTouchIconPNG []byte

// FC assets are generated from the official
// https://friends-connect.club/favicon.svg source.
//
//go:embed static/brands/fc/favicon.ico
var fcFaviconICO []byte

//go:embed static/brands/fc/favicon-32x32.png
var fcFavicon32PNG []byte

//go:embed static/brands/fc/apple-touch-icon.png
var fcAppleTouchIconPNG []byte

type faviconAssetSet struct {
	ICO        []byte
	PNG32      []byte
	AppleTouch []byte
}

func faviconAssetsForBrand(cfg *config.Config) (faviconAssetSet, error) {
	if cfg == nil {
		return faviconAssetSet{}, errors.New("favicon brand id is required")
	}
	id := strings.TrimSpace(cfg.BrandID())
	if id == "" {
		return faviconAssetSet{}, errors.New("favicon brand id is required")
	}
	switch id {
	case "vff":
		return faviconAssetSet{
			ICO:        vffFaviconICO,
			PNG32:      vffFavicon32PNG,
			AppleTouch: vffAppleTouchIconPNG,
		}, nil
	case "fc":
		return faviconAssetSet{
			ICO:        fcFaviconICO,
			PNG32:      fcFavicon32PNG,
			AppleTouch: fcAppleTouchIconPNG,
		}, nil
	default:
		return faviconAssetSet{}, fmt.Errorf("unsupported favicon brand id %q", id)
	}
}

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

func serveBrandFaviconAsset(cfg *config.Config, contentType string, pick func(faviconAssetSet) []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		set, err := faviconAssetsForBrand(cfg)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		body := pick(set)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=604800")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func serveBrandFaviconICO(cfg *config.Config) http.HandlerFunc {
	return serveBrandFaviconAsset(cfg, "image/x-icon", func(s faviconAssetSet) []byte { return s.ICO })
}

func serveBrandFavicon32PNG(cfg *config.Config) http.HandlerFunc {
	return serveBrandFaviconAsset(cfg, "image/png", func(s faviconAssetSet) []byte { return s.PNG32 })
}

func serveBrandAppleTouchIcon(cfg *config.Config) http.HandlerFunc {
	return serveBrandFaviconAsset(cfg, "image/png", func(s faviconAssetSet) []byte { return s.AppleTouch })
}
