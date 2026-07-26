package web

import (
	"bytes"
	_ "embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/ryabkov82/vpnbot/internal/config"
)

//go:embed static/buy/index.html
var buyPageTemplateSrc string

var (
	buyPageTmplOnce sync.Once
	buyPageTmpl     *template.Template
	buyPageTmplErr  error
)

type buyPageData struct {
	PageTitle  string
	BrandName  string
	SupportURL string
}

func buyPageTemplate() (*template.Template, error) {
	buyPageTmplOnce.Do(func() {
		buyPageTmpl, buyPageTmplErr = template.New("buy").Parse(buyPageTemplateSrc)
	})
	return buyPageTmpl, buyPageTmplErr
}

func buyPageBrandName(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", errors.New("buy page brand name is required")
	}
	name := strings.TrimSpace(cfg.EffectiveBrand().Name)
	if name == "" {
		return "", errors.New("buy page brand name is required")
	}
	return name, nil
}

func renderedBuyPageHTML(cfg *config.Config) ([]byte, error) {
	brandName, err := buyPageBrandName(cfg)
	if err != nil {
		return nil, err
	}
	tmpl, err := buyPageTemplate()
	if err != nil {
		return nil, err
	}
	data := buyPageData{
		PageTitle:  brandName + " — купить VPN",
		BrandName:  brandName,
		SupportURL: WebCabinetResolvedSupportURL(cfg),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func serveBuy(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		body, err := renderedBuyPageHTML(cfg)
		if err != nil {
			log.Printf("buy page render: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
