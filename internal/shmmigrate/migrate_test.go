package shmmigrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func writeTempJSON(t *testing.T, dir, name string, v any) string {
	t.Helper()
	path := filepath.Join(dir, name)
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func samplePlan(userID int, chatID int64) []PlanEntry {
	return []PlanEntry{{
		Classification:    "fc_only",
		UserID:            userID,
		Login:             fmt.Sprintf("@%d", chatID),
		ProposedLogin:     fmt.Sprintf("@fc_%d", chatID),
		TelegramChatID:    chatID,
		TargetLoginExists: false,
		EvidenceHash:      "hash1",
	}}
}

type fakeSHM struct {
	users      map[int]LiveUser
	byLogin    map[string]int
	services   map[int][]UserService
	updateN    atomic.Int32
	lastBody   atomic.Value // []byte
	verifyFlip bool         // after update, mutate to break verification
}

func (f *fakeSHM) setUser(u LiveUser) {
	if f.users == nil {
		f.users = map[int]LiveUser{}
	}
	if f.byLogin == nil {
		f.byLogin = map[string]int{}
	}
	// remove old login mapping for this user
	for login, id := range f.byLogin {
		if id == u.UserID {
			delete(f.byLogin, login)
		}
	}
	f.users[u.UserID] = u
	f.byLogin[u.Login] = u.UserID
}

func (f *fakeSHM) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == pathAuth:
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess"})
		case r.Method == http.MethodGet && r.URL.Path == pathAdminUser:
			filter := r.URL.Query().Get("filter")
			var m map[string]any
			_ = json.Unmarshal([]byte(filter), &m)
			var out []LiveUser
			if idf, ok := m["user_id"]; ok {
				id := int(idf.(float64))
				if u, ok := f.users[id]; ok {
					out = append(out, u)
				}
			} else if login, ok := m["login"].(string); ok {
				if id, ok := f.byLogin[login]; ok {
					out = append(out, f.users[id])
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": out})
		case r.Method == http.MethodGet && r.URL.Path == pathAdminUserService:
			filter := r.URL.Query().Get("filter")
			var m map[string]any
			_ = json.Unmarshal([]byte(filter), &m)
			id := int(m["user_id"].(float64))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": f.services[id]})
		case r.Method == http.MethodPost && r.URL.Path == pathAdminUser:
			f.updateN.Add(1)
			body, _ := io.ReadAll(r.Body)
			f.lastBody.Store(append([]byte(nil), body...))
			var payload struct {
				UserID   int                        `json:"user_id"`
				Login    string                     `json:"login"`
				Settings map[string]json.RawMessage `json:"settings"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, "bad", 400)
				return
			}
			u, ok := f.users[payload.UserID]
			if !ok {
				http.Error(w, "missing", 404)
				return
			}
			settingsRaw, _ := json.Marshal(payload.Settings)
			u.Login = payload.Login
			u.Settings = settingsRaw
			if f.verifyFlip {
				u.Balance = u.Balance + 1 // break verification
			}
			f.setUser(u)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":200}`))
		case r.Method == http.MethodPut:
			t.Errorf("PUT must not be used")
			http.Error(w, "no put", 405)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, 404)
		}
	}))
}

func testOpts(t *testing.T, baseURL string, plan []PlanEntry, apply bool) Options {
	t.Helper()
	dir := t.TempDir()
	cfgPath := writeTempJSON(t, dir, "config.json", map[string]any{
		"api": map[string]any{
			"base_url": baseURL, "api_login": "l", "api_pass": "p", "timeout_seconds": 5,
		},
	})
	planPath := writeTempJSON(t, dir, "fc-only.json", plan)
	out := filepath.Join(dir, "out")
	return Options{ConfigPath: cfgPath, PlanPath: planPath, OutputDir: out, Apply: apply}
}

func readyUser(id int, chat int64, settings string) LiveUser {
	if settings == "" {
		settings = fmt.Sprintf(`{"telegram":{"chat_id":%d},"custom":{"x":1},"nested":[1,2,3]}`, chat)
	}
	return LiveUser{
		UserID: id, Login: fmt.Sprintf("@%d", chat),
		Balance: 10, Bonus: 1, Credit: 0,
		Login2:   "email@example.com",
		Settings: json.RawMessage(settings),
	}
}

func TestDryRun_NoUpdatePOST(t *testing.T) {
	f := &fakeSHM{services: map[int][]UserService{}}
	u := readyUser(311, 327490633, "")
	f.setUser(u)
	srv := f.server(t)
	defer srv.Close()

	opt := testOpts(t, srv.URL, samplePlan(311, 327490633), false)
	res, err := Run(context.Background(), opt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.updateN.Load() != 0 {
		t.Fatalf("update calls=%d", f.updateN.Load())
	}
	if res.Mode != "dry-run" || res.Ready != 1 || res.Writes != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestApply_UpdatesPayloadAndPreservesSettings(t *testing.T) {
	f := &fakeSHM{services: map[int][]UserService{
		311: {{UserID: 311, Category: categoryFC}},
	}}
	f.setUser(readyUser(311, 327490633, ""))
	srv := f.server(t)
	defer srv.Close()

	opt := testOpts(t, srv.URL, samplePlan(311, 327490633), true)
	res, err := Run(context.Background(), opt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.updateN.Load() != 1 {
		t.Fatalf("update calls=%d", f.updateN.Load())
	}
	body, _ := f.lastBody.Load().([]byte)
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if int(payload["user_id"].(float64)) != 311 {
		t.Fatalf("user_id=%v", payload["user_id"])
	}
	if payload["login"] != "@fc_327490633" {
		t.Fatalf("login=%v", payload["login"])
	}
	settings := payload["settings"].(map[string]any)
	if settings["brand_id"] != "fc" {
		t.Fatalf("brand_id=%v", settings["brand_id"])
	}
	if _, ok := settings["custom"]; !ok {
		t.Fatal("custom settings lost")
	}
	if _, ok := settings["nested"]; !ok {
		t.Fatal("nested settings lost")
	}
	if _, ok := settings["telegram"]; !ok {
		t.Fatal("telegram lost")
	}
	if _, ok := payload["password"]; ok {
		t.Fatal("password must not be sent")
	}
	if _, ok := payload["balance"]; ok {
		t.Fatal("balance must not be sent")
	}
	if res.Updated != 1 || res.Writes != 1 {
		t.Fatalf("%+v", res)
	}
	// backup exists with settings
	raw, err := os.ReadFile(filepath.Join(opt.OutputDir, "backup-before.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"custom"`) {
		t.Fatal("backup missing settings")
	}
}

func TestPreflight_TargetLoginOccupied(t *testing.T) {
	f := &fakeSHM{services: map[int][]UserService{}}
	f.setUser(readyUser(311, 327490633, ""))
	f.setUser(LiveUser{UserID: 999, Login: "@fc_327490633", Settings: json.RawMessage(`{"brand_id":"fc"}`)})
	srv := f.server(t)
	defer srv.Close()

	opt := testOpts(t, srv.URL, samplePlan(311, 327490633), true)
	_, err := Run(context.Background(), opt, nil)
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if f.updateN.Load() != 0 {
		t.Fatalf("updates=%d", f.updateN.Load())
	}
}

func TestPreflight_OneBadBlocksAll(t *testing.T) {
	f := &fakeSHM{services: map[int][]UserService{}}
	f.setUser(readyUser(311, 100, ""))
	f.setUser(readyUser(444, 200, `{"telegram":{"chat_id":200},"brand_id":"vff"}`))
	srv := f.server(t)
	defer srv.Close()

	plan := append(samplePlan(311, 100), samplePlan(444, 200)...)
	plan[1].EvidenceHash = "hash2"
	opt := testOpts(t, srv.URL, plan, true)
	_, err := Run(context.Background(), opt, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if f.updateN.Load() != 0 {
		t.Fatalf("updates=%d", f.updateN.Load())
	}
}

func TestPreflight_VFFServiceAppeared(t *testing.T) {
	f := &fakeSHM{services: map[int][]UserService{
		311: {{UserID: 311, Category: categoryVFF}},
	}}
	f.setUser(readyUser(311, 327490633, ""))
	srv := f.server(t)
	defer srv.Close()

	opt := testOpts(t, srv.URL, samplePlan(311, 327490633), true)
	_, err := Run(context.Background(), opt, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if f.updateN.Load() != 0 {
		t.Fatalf("updates=%d", f.updateN.Load())
	}
}

func TestAlreadyMigrated_NoUpdate(t *testing.T) {
	chat := int64(327490633)
	f := &fakeSHM{services: map[int][]UserService{}}
	settings := fmt.Sprintf(`{"brand_id":"fc","telegram":{"chat_id":%d}}`, chat)
	f.setUser(LiveUser{
		UserID: 311, Login: "@fc_327490633",
		Settings: json.RawMessage(settings),
	})
	srv := f.server(t)
	defer srv.Close()

	opt := testOpts(t, srv.URL, samplePlan(311, chat), true)
	res, err := Run(context.Background(), opt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.updateN.Load() != 0 {
		t.Fatalf("updates=%d", f.updateN.Load())
	}
	if res.AlreadyMigrated != 1 || res.Updated != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestApply_VerificationFailedStops(t *testing.T) {
	f := &fakeSHM{
		services: map[int][]UserService{
			311: {{UserID: 311, Category: categoryFC}},
			444: {{UserID: 444, Category: categoryFC}},
		},
		verifyFlip: true,
	}
	f.setUser(readyUser(311, 100, ""))
	f.setUser(readyUser(444, 200, ""))
	srv := f.server(t)
	defer srv.Close()

	plan := append(samplePlan(311, 100), samplePlan(444, 200)...)
	plan[1].EvidenceHash = "hash2"
	opt := testOpts(t, srv.URL, plan, true)
	res, err := Run(context.Background(), opt, nil)
	if err == nil {
		t.Fatal("expected verification failure")
	}
	if f.updateN.Load() != 1 {
		t.Fatalf("expected stop after first verify fail, updates=%d", f.updateN.Load())
	}
	if res.Failed != 1 {
		t.Fatalf("%+v", res)
	}
	var notStarted int
	for _, it := range res.Items {
		if it.ApplyState == applyNotStarted {
			notStarted++
		}
	}
	if notStarted != 1 {
		t.Fatalf("items=%+v", res.Items)
	}
}

func TestBuildUpdateSettings_PreservesUnknown(t *testing.T) {
	raw := json.RawMessage(`{"telegram":{"chat_id":123},"custom":{"x":1},"nested":[1,2,3]}`)
	out, err := BuildUpdateSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(out["brand_id"]) != `"fc"` {
		t.Fatalf("brand=%s", out["brand_id"])
	}
	if _, ok := out["custom"]; !ok {
		t.Fatal("custom lost")
	}
	if _, ok := out["nested"]; !ok {
		t.Fatal("nested lost")
	}
	marshaled, _ := json.Marshal(out)
	var check map[string]any
	_ = json.Unmarshal(marshaled, &check)
	if check["brand_id"] != "fc" {
		t.Fatal(check)
	}
}

func TestLoadPlan_RejectsNonFCOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSON(t, dir, "plan.json", []PlanEntry{{
		Classification: "shared", UserID: 1, Login: "@1", ProposedLogin: "@fc_1",
		TelegramChatID: 1, EvidenceHash: "h",
	}})
	_, err := LoadPlan(path)
	if err == nil {
		t.Fatal("expected error")
	}
}
