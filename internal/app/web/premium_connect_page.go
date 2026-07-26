package web

import (
	"bytes"
	_ "embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/ryabkov82/vpnbot/internal/config"
)

//go:embed static/premium-connect/index.html
var premiumConnectTemplateSrc string

var (
	premiumConnectTmplOnce sync.Once
	premiumConnectTmpl     *template.Template
	premiumConnectTmplErr  error
)

type premiumConnectPageData struct {
	SupportURL string
}

func premiumConnectTemplate() (*template.Template, error) {
	premiumConnectTmplOnce.Do(func() {
		premiumConnectTmpl, premiumConnectTmplErr = template.New("premium-connect").Parse(premiumConnectTemplateSrc)
	})
	return premiumConnectTmpl, premiumConnectTmplErr
}

func renderedPremiumConnectHTML(cfg *config.Config) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("premium connect config is required")
	}
	tmpl, err := premiumConnectTemplate()
	if err != nil {
		return nil, err
	}
	data := premiumConnectPageData{
		SupportURL: WebCabinetResolvedSupportURL(cfg),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func servePremiumConnect(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch path {
		case "/premium-connect", "/premium-connect/",
			"/premium-connect-test", "/premium-connect-test/":
		default:
			http.NotFound(w, r)
			return
		}

		log.Printf("premium-connect: %s %s", r.Method, r.URL.Path)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := renderedPremiumConnectHTML(cfg)
		if err != nil {
			log.Printf("premium-connect page render: %v", err)
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
