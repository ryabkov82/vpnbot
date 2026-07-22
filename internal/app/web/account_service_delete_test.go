package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryabkov82/vpnbot/internal/models"
)

func TestServeAccountServiceDelete_InvalidToken(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://api.example.com"
	h := serveAccountServiceDelete(cfg, &stubAccountWeb{})
	req := httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"x","user_service_id":3}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_token")
}

func TestServeAccountServiceDelete_InvalidUserServiceID(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@z.z", 1, "lg", time.Hour)
	h := serveAccountServiceDelete(cfg, &stubAccountWeb{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":0}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "invalid_service")
}

func TestServeAccountServiceDelete_ServiceNilForbidden(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@z.z", 10, "lg", time.Hour)
	st := &stubAccountWeb{}
	h := serveAccountServiceDelete(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":999}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "forbidden")
}

func TestServeAccountServiceDelete_UserMismatchForbidden(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@z.z", 10, "lg", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			55: {
				UserID:    777,
				ServiceID: 55,
				Status:    "NOT PAID",
				Name:      "X",
			},
		},
	}
	h := serveAccountServiceDelete(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":55}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestServeAccountServiceDelete_ServiceIDsMismatchForbidden(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@z.z", 10, "lg", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			55: {
				UserID:    10,
				ServiceID: 56,
				Status:    "NOT PAID",
			},
		},
	}
	h := serveAccountServiceDelete(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":55}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestServeAccountServiceDelete_ActiveForbidden(t *testing.T) {
	cfg := orderStartTestCfg()
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@z.z", 10, "lg", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			100: {
				UserID:    10,
				ServiceID: 100,
				Status:    "ACTIVE",
				Name:      "Подписка",
			},
		},
	}
	h := serveAccountServiceDelete(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":100}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "active_service_cannot_be_deleted")
}

func TestServeAccountServiceDelete_NotPaidDeletes(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://pay.test/"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@z.z", 88, "lg88", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			337: {
				UserID:    88,
				ServiceID: 337,
				Status:    "NOT PAID",
				Name:      "Услуга",
			},
		},
	}
	h := serveAccountServiceDelete(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":337}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if st.deleteCalls != 1 || st.deleteLastUID != 88 || st.deleteLastUserServiceID != "337" {
		t.Fatalf("delete call want 88 337 got %d %s calls=%d", st.deleteLastUID, st.deleteLastUserServiceID, st.deleteCalls)
	}
	var out accountServiceDeleteOKJSON
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "deleted" || out.UserServiceID != 337 || !strings.Contains(out.Message, "удалена") {
		t.Fatalf("%#v", out)
	}
}

func TestServeAccountServiceDelete_DeleteFails(t *testing.T) {
	cfg := orderStartTestCfg()
	cfg.API.BaseURL = "https://pay.test/"
	tok, _ := CreateAccountToken(cfg.WebSales.OrderTokenSecret, "vff", "a@z.z", 88, "lg88", time.Hour)
	st := &stubAccountWeb{
		single: map[int]*models.UserService{
			337: {UserID: 88, ServiceID: 337, Status: "BLOCK"},
		},
		deleteErr: errors.New("shm no"),
	}
	h := serveAccountServiceDelete(cfg, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/account/service/delete",
		strings.NewReader(`{"token":"`+tok+`","user_service_id":337}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
	assertJSONErrorField(t, rec.Body.String(), "delete_failed")
}
