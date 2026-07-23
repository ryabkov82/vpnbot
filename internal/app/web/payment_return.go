package web

import (
	"bytes"
	_ "embed"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/ryabkov82/vpnbot/internal/config"
)

//go:embed static/payment/return.html
var paymentReturnPageTemplateSrc string

var (
	paymentReturnPageTmplOnce sync.Once
	paymentReturnPageTmpl     *template.Template
	paymentReturnPageTmplErr  error
)

type paymentReturnPageData struct {
	BrandName  string
	AccountURL string
	PageTitle  string
}

func paymentReturnPageTemplate() (*template.Template, error) {
	paymentReturnPageTmplOnce.Do(func() {
		paymentReturnPageTmpl, paymentReturnPageTmplErr = template.New("payment-return").Parse(paymentReturnPageTemplateSrc)
	})
	return paymentReturnPageTmpl, paymentReturnPageTmplErr
}

// servePaymentReturn — публичная страница возврата из YooKassa (без web-сессии).
// Не утверждает, что платёж подтверждён или зачислен.
func servePaymentReturn(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payment/return", "/payment/return/":
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		brandName := strings.TrimSpace(cfg.EffectiveBrand().Name)
		if brandName == "" {
			brandName = "Личный кабинет"
		}

		tmpl, err := paymentReturnPageTemplate()
		if err != nil {
			log.Printf("payment/return: template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		data := paymentReturnPageData{
			BrandName:  brandName,
			AccountURL: "/account",
			PageTitle:  brandName + " — возврат из оплаты",
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			log.Printf("payment/return: execute: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}
}
