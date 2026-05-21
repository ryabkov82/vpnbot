package web

import (
	_ "embed"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/email"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/payments"
)

type webOrderPayApp interface {
	FindOrCreateWebUser(email string) (*models.User, error)
	ServiceOrderByUserID(userID int, serviceID int) (*models.UserService, error)
	GetServiceByID(serviceID int) (*models.Service, error)
}

type buyPayPageData struct {
	ServiceName string
	Amount      string
	PaymentURL  string
	StatusURL   string
	OrderToken  string
}

//go:embed static/buy/pay.tmpl
var buyPayTmplRaw string

var buyPayTemplate = template.Must(template.New("pay").Parse(buyPayTmplRaw))

func serveBuyPay(cfg *config.Config, app webOrderPayApp, used *UsedStartTokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/buy/pay" && r.URL.Path != "/buy/pay/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		if !webSalesTokenFlowAvailable(cfg) {
			http.NotFound(w, r)
			return
		}

		rawTok := strings.TrimSpace(r.URL.Query().Get("token"))
		if rawTok == "" {
			writeBuyPayError(w, http.StatusBadRequest, "Отсутствует параметр token.")
			return
		}

		secret := strings.TrimSpace(cfg.WebSales.OrderTokenSecret)
		start, err := ParseAndVerifyOrderStartToken(secret, rawTok)
		if err != nil {
			writeBuyPayError(w, http.StatusBadRequest, "Ссылка недействительна или истекла.")
			return
		}

		now := time.Now().Unix()
		if used.IsUsed(rawTok, now) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`<!doctype html><html lang="ru"><head><meta charset="utf-8"><title>Заказ</title></head><body style="font-family:sans-serif;padding:2rem"><h1>Заказ по этой ссылке уже был создан</h1><p>Проверьте email — мы отправили ссылку для проверки оплаты. Также можно открыть <a href="/buy/status">/buy/status</a>, если браузер сохранил токен.</p><p><a href="/buy">Вернуться к тарифам</a></p></body></html>`))
			return
		}

		user, err := app.FindOrCreateWebUser(start.Email)
		if err != nil {
			slog.Error("buy/pay FindOrCreateWebUser", "err", err)
			writeBuyPayError(w, http.StatusInternalServerError, "Не удалось создать пользователя. Попробуйте позже.")
			return
		}

		order, err := app.ServiceOrderByUserID(user.ID, start.ServiceID)
		if err != nil || order == nil {
			slog.Error("buy/pay ServiceOrderByUserID", "err", err)
			writeBuyPayError(w, http.StatusInternalServerError, "Не удалось создать заказ. Попробуйте позже.")
			return
		}

		svc, err := app.GetServiceByID(start.ServiceID)
		if err != nil || svc == nil {
			slog.Error("buy/pay GetServiceByID", "err", err)
			writeBuyPayError(w, http.StatusInternalServerError, "Не удалось получить тариф.")
			return
		}
		preview := models.BuildServicePreview(svc)
		svcName := strings.TrimSpace(preview.Title)
		if svcName == "" {
			svcName = "Тариф"
		}
		amount := preview.Cost
		if amount <= 0 {
			amount = svc.Cost
		}

		baseURL := ""
		if cfg != nil {
			baseURL = cfg.API.BaseURL
		}
		paymentURL, err := payments.BuildYooKassaPaymentURL(baseURL, user.ID, amount, time.Now().Unix())
		if err != nil {
			slog.Error("buy/pay BuildYooKassaPaymentURL", "err", err)
			writeBuyPayError(w, http.StatusInternalServerError, "Не удалось сформировать ссылку на оплату.")
			return
		}

		ttl := webSalesOrderTokenTTL(cfg)
		orderTok, err := CreateOrderToken(secret, start.Email, start.ServiceID, user.ID, order.ServiceID, amount, ttl)
		if err != nil {
			slog.Error("buy/pay CreateOrderToken", "err", err)
			writeBuyPayError(w, http.StatusInternalServerError, "Внутренняя ошибка.")
			return
		}

		used.MarkUsed(rawTok, start.Exp)

		pubBase := publicOrderBaseURL(cfg, r)
		if pubBase == "" {
			writeBuyPayError(w, http.StatusInternalServerError, "Не настроен адрес сайта.")
			return
		}
		statusURL := pubBase + "/buy/status?token=" + url.QueryEscape(orderTok)

		amountStr := strconv.FormatFloat(amount, 'f', -1, 64)
		if err := email.SendOrderStatusEmail(cfg, start.Email, svcName, amountStr, statusURL); err != nil {
			slog.Warn("buy/pay status email", "err", err)
		}

		data := buyPayPageData{
			ServiceName: svcName,
			Amount:      strconv.FormatFloat(amount, 'f', -1, 64),
			PaymentURL:  paymentURL,
			StatusURL:   statusURL,
			OrderToken:  orderTok,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if err := buyPayTemplate.Execute(w, data); err != nil {
			slog.Error("buy/pay template", "err", err)
		}
	}
}

func writeBuyPayError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	esc := template.HTMLEscapeString(msg)
	_, _ = w.Write([]byte(`<!doctype html><html lang="ru"><head><meta charset="utf-8"><title>Ошибка</title></head><body style="font-family:sans-serif;padding:2rem"><p>` + esc + `</p><p><a href="/buy">На страницу тарифов</a></p></body></html>`))
}
