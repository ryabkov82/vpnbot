package shmmigrate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	brandFC         = "fc"
	categoryFC      = "vpn-mz-fc"
	categoryVFF     = "vpn-mz-test"
	stateReady      = "ready"
	stateMigrated   = "already_migrated"
	applyUpdated    = "updated"
	applyFailed     = "failed"
	applySkipped    = "skipped"
	applyNotStarted = "not_started"
)

// Options — CLI-параметры миграции.
type Options struct {
	ConfigPath string
	PlanPath   string
	OutputDir  string
	Apply      bool
}

// PlanEntry — запись из fc-only.json (allowlist).
type PlanEntry struct {
	Classification    string `json:"classification"`
	UserID            int    `json:"user_id"`
	Login             string `json:"login"`
	ProposedLogin     string `json:"proposed_login"`
	TelegramChatID    int64  `json:"telegram_chat_id"`
	TargetLoginExists bool   `json:"target_login_exists"`
	EvidenceHash      string `json:"evidence_hash"`
}

// PreflightItem — результат preflight по одной записи.
type PreflightItem struct {
	UserID      int    `json:"user_id"`
	OldLogin    string `json:"old_login"`
	NewLogin    string `json:"new_login"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	LiveLogin   string `json:"live_login,omitempty"`
	LiveBrandID string `json:"live_brand_id,omitempty"`
}

// ResultItem — строка result.json (apply).
type ResultItem struct {
	UserID         int    `json:"user_id"`
	OldLogin       string `json:"old_login"`
	NewLogin       string `json:"new_login"`
	PreflightState string `json:"preflight_state"`
	ApplyState     string `json:"apply_state"`
	Error          string `json:"error,omitempty"`
}

// Result — итоговый отчёт.
type Result struct {
	Mode            string       `json:"mode"`
	Total           int          `json:"total"`
	Ready           int          `json:"ready"`
	AlreadyMigrated int          `json:"already_migrated"`
	Updated         int          `json:"updated"`
	Failed          int          `json:"failed"`
	Writes          int          `json:"writes"`
	Items           []ResultItem `json:"items,omitempty"`
}

// BackupUser — запись backup-before.json.
type BackupUser struct {
	UserID   int             `json:"user_id"`
	Login    string          `json:"login"`
	Login2   string          `json:"login2"`
	Balance  float64         `json:"balance"`
	Bonus    float64         `json:"bonus"`
	Credit   float64         `json:"credit"`
	Settings json.RawMessage `json:"settings"`
}

// Logger — stdout/stderr без секретов и settings.
type Logger func(format string, args ...any)

// LoadConfig читает минимальный API-конфиг.
func LoadConfig(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("config path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.API.BaseURL = strings.TrimSpace(cfg.API.BaseURL)
	cfg.API.Login = strings.TrimSpace(cfg.API.Login)
	if cfg.API.BaseURL == "" || cfg.API.Login == "" || cfg.API.Pass == "" {
		return nil, fmt.Errorf("api.base_url, api.api_login and api.api_pass are required")
	}
	if cfg.API.Timeout <= 0 {
		cfg.API.Timeout = 30
	}
	return &cfg, nil
}

// ValidateOptions проверяет обязательные флаги.
func ValidateOptions(opt Options) error {
	if strings.TrimSpace(opt.ConfigPath) == "" {
		return fmt.Errorf("--config is required")
	}
	if strings.TrimSpace(opt.PlanPath) == "" {
		return fmt.Errorf("--plan is required")
	}
	if strings.TrimSpace(opt.OutputDir) == "" {
		return fmt.Errorf("--output is required")
	}
	return nil
}

// LoadPlan читает и валидирует fc-only.json allowlist.
func LoadPlan(path string) ([]PlanEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	var plan []PlanEntry
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, fmt.Errorf("parse plan: expected JSON array: %w", err)
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("plan is empty")
	}
	seenUID := map[int]struct{}{}
	seenLogin := map[string]struct{}{}
	seenProposed := map[string]struct{}{}
	for i, e := range plan {
		if e.UserID <= 0 {
			return nil, fmt.Errorf("plan[%d]: invalid user_id", i)
		}
		if _, ok := seenUID[e.UserID]; ok {
			return nil, fmt.Errorf("plan: duplicate user_id %d", e.UserID)
		}
		seenUID[e.UserID] = struct{}{}
		if e.Classification != "fc_only" {
			return nil, fmt.Errorf("plan user_id=%d: classification must be fc_only", e.UserID)
		}
		if e.TelegramChatID <= 0 {
			return nil, fmt.Errorf("plan user_id=%d: telegram_chat_id must be > 0", e.UserID)
		}
		wantLogin := "@" + strconv.FormatInt(e.TelegramChatID, 10)
		if e.Login != wantLogin {
			return nil, fmt.Errorf("plan user_id=%d: login must be %s", e.UserID, wantLogin)
		}
		wantProposed := "@fc_" + strconv.FormatInt(e.TelegramChatID, 10)
		if e.ProposedLogin != wantProposed {
			return nil, fmt.Errorf("plan user_id=%d: proposed_login must be %s", e.UserID, wantProposed)
		}
		if e.TargetLoginExists {
			return nil, fmt.Errorf("plan user_id=%d: target_login_exists must be false", e.UserID)
		}
		if strings.TrimSpace(e.EvidenceHash) == "" {
			return nil, fmt.Errorf("plan user_id=%d: evidence_hash is required", e.UserID)
		}
		if _, ok := seenLogin[e.Login]; ok {
			return nil, fmt.Errorf("plan: duplicate login %s", e.Login)
		}
		seenLogin[e.Login] = struct{}{}
		if _, ok := seenProposed[e.ProposedLogin]; ok {
			return nil, fmt.Errorf("plan: duplicate proposed_login %s", e.ProposedLogin)
		}
		seenProposed[e.ProposedLogin] = struct{}{}
	}
	sort.SliceStable(plan, func(i, j int) bool { return plan[i].UserID < plan[j].UserID })
	return plan, nil
}

func parseSettings(raw json.RawMessage) (map[string]json.RawMessage, string, int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]json.RawMessage{}, "", 0, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, "", 0, fmt.Errorf("settings is not a JSON object")
	}
	brand := ""
	if b, ok := obj["brand_id"]; ok && string(b) != "null" {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, "", 0, fmt.Errorf("settings.brand_id is not a string")
		}
		brand = strings.TrimSpace(s)
	}
	var chatID int64
	if tgRaw, ok := obj["telegram"]; ok && len(tgRaw) > 0 && string(tgRaw) != "null" {
		var tg map[string]json.RawMessage
		if err := json.Unmarshal(tgRaw, &tg); err != nil {
			return nil, "", 0, fmt.Errorf("settings.telegram is not an object")
		}
		if cRaw, ok := tg["chat_id"]; ok && len(cRaw) > 0 && string(cRaw) != "null" {
			id, err := parseChatID(cRaw)
			if err != nil {
				return nil, "", 0, err
			}
			chatID = id
		}
	}
	return obj, brand, chatID, nil
}

func parseChatID(raw json.RawMessage) (int64, error) {
	var n json.Number
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return 0, fmt.Errorf("settings.telegram.chat_id: %w", err)
	}
	switch t := v.(type) {
	case json.Number:
		n = t
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("settings.telegram.chat_id: invalid number")
		}
		return i, nil
	case string:
		s := strings.TrimSpace(t)
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("settings.telegram.chat_id: invalid string")
		}
		return i, nil
	case float64:
		i := int64(t)
		if float64(i) != t {
			return 0, fmt.Errorf("settings.telegram.chat_id: non-integer")
		}
		return i, nil
	default:
		return 0, fmt.Errorf("settings.telegram.chat_id: unsupported type")
	}
}

// BuildUpdateSettings копирует raw settings и ставит brand_id=fc.
func BuildUpdateSettings(raw json.RawMessage) (map[string]json.RawMessage, error) {
	obj, _, _, err := parseSettings(raw)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	obj["brand_id"] = json.RawMessage(`"fc"`)
	return obj, nil
}

func checkServiceCategories(services []UserService) error {
	for _, s := range services {
		cat := strings.TrimSpace(s.Category)
		if cat == "" {
			return fmt.Errorf("empty service category")
		}
		if cat == categoryVFF {
			return fmt.Errorf("current VFF service category %s", cat)
		}
		if cat != categoryFC {
			return fmt.Errorf("unknown service category %s", cat)
		}
	}
	return nil
}

func preflightOne(ctx context.Context, c *Client, e PlanEntry) PreflightItem {
	item := PreflightItem{
		UserID:   e.UserID,
		OldLogin: e.Login,
		NewLogin: e.ProposedLogin,
	}
	live, err := c.GetUserByID(ctx, e.UserID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if live == nil {
		item.Error = "user not found"
		return item
	}
	item.LiveLogin = live.Login
	settings, brand, chatID, err := parseSettings(live.Settings)
	_ = settings
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.LiveBrandID = brand

	// already_migrated
	if live.Login == e.ProposedLogin && brand == brandFC && chatID == e.TelegramChatID {
		byLogin, err := c.GetUserByLogin(ctx, e.ProposedLogin)
		if err != nil {
			item.Error = err.Error()
			return item
		}
		if byLogin == nil || byLogin.UserID != e.UserID {
			item.Error = "proposed_login does not belong to this user_id"
			return item
		}
		item.State = stateMigrated
		return item
	}

	// ready checks
	if live.UserID != e.UserID {
		item.Error = "user_id mismatch"
		return item
	}
	if live.Login != e.Login {
		item.Error = fmt.Sprintf("login changed: live=%s plan=%s", live.Login, e.Login)
		return item
	}
	if brand != "" {
		item.Error = fmt.Sprintf("brand_id is not empty: %q", brand)
		return item
	}
	if chatID != e.TelegramChatID {
		item.Error = fmt.Sprintf("telegram.chat_id mismatch: live=%d plan=%d", chatID, e.TelegramChatID)
		return item
	}
	occupant, err := c.GetUserByLogin(ctx, e.ProposedLogin)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if occupant != nil && occupant.UserID != e.UserID {
		item.Error = fmt.Sprintf("target login occupied by user_id=%d", occupant.UserID)
		return item
	}
	if occupant != nil && occupant.UserID == e.UserID {
		// same user already has proposed login but didn't match already_migrated → inconsistent
		item.Error = "inconsistent state: proposed_login owned by user but not already_migrated"
		return item
	}
	services, err := c.GetUserServices(ctx, e.UserID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if err := checkServiceCategories(services); err != nil {
		item.Error = err.Error()
		return item
	}
	item.State = stateReady
	return item
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return os.Chmod(path, 0o600)
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeFileAtomic(path, raw)
}

func prepareOutputDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"backup-before.json", "preflight.json", "result.json"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("output file already exists: %s", p)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func verifyAfterUpdate(ctx context.Context, c *Client, e PlanEntry, before *LiveUser) error {
	live, err := c.GetUserByID(ctx, e.UserID)
	if err != nil {
		return err
	}
	if live == nil {
		return fmt.Errorf("user missing after update")
	}
	if live.UserID != e.UserID {
		return fmt.Errorf("user_id changed")
	}
	if live.Login != e.ProposedLogin {
		return fmt.Errorf("login not updated: %s", live.Login)
	}
	_, brand, chatID, err := parseSettings(live.Settings)
	if err != nil {
		return err
	}
	if brand != brandFC {
		return fmt.Errorf("brand_id=%q", brand)
	}
	if chatID != e.TelegramChatID {
		return fmt.Errorf("telegram.chat_id changed")
	}
	if live.Balance != before.Balance || live.Bonus != before.Bonus || live.Credit != before.Credit {
		return fmt.Errorf("balance/bonus/credit changed")
	}
	if live.Login2 != before.Login2 {
		return fmt.Errorf("login2 changed")
	}
	byNew, err := c.GetUserByLogin(ctx, e.ProposedLogin)
	if err != nil {
		return err
	}
	if byNew == nil || byNew.UserID != e.UserID {
		return fmt.Errorf("GET by proposed_login mismatch")
	}
	byOld, err := c.GetUserByLogin(ctx, e.Login)
	if err != nil {
		return err
	}
	if byOld != nil && byOld.UserID == e.UserID {
		return fmt.Errorf("old login still resolves to user")
	}
	return nil
}

// Run выполняет dry-run или apply миграции.
func Run(ctx context.Context, opt Options, log Logger) (*Result, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	if err := ValidateOptions(opt); err != nil {
		return nil, err
	}
	plan, err := LoadPlan(opt.PlanPath)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(plan))
	for i, e := range plan {
		ids[i] = strconv.Itoa(e.UserID)
	}
	log("plan user_ids=%s", strings.Join(ids, ","))

	cfg, err := LoadConfig(opt.ConfigPath)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	log("authenticating with SHM (credentials not logged)")
	if err := client.Authenticate(ctx); err != nil {
		return nil, err
	}

	if err := prepareOutputDir(opt.OutputDir); err != nil {
		return nil, err
	}

	preflight := make([]PreflightItem, 0, len(plan))
	liveByID := map[int]*LiveUser{}
	var ready, migrated, errors int
	for _, e := range plan {
		item := preflightOne(ctx, client, e)
		preflight = append(preflight, item)
		switch item.State {
		case stateReady:
			ready++
			u, err := client.GetUserByID(ctx, e.UserID)
			if err != nil {
				return nil, err
			}
			liveByID[e.UserID] = u
		case stateMigrated:
			migrated++
		default:
			errors++
		}
	}

	mode := "dry-run"
	if opt.Apply {
		mode = "apply"
	}
	result := &Result{
		Mode:            mode,
		Total:           len(plan),
		Ready:           ready,
		AlreadyMigrated: migrated,
	}

	// Print table
	log("%-8s %-16s %-20s %s", "user_id", "old_login", "new_login", "state")
	for _, item := range preflight {
		st := item.State
		if st == "" {
			st = "error"
		}
		log("%-8d %-16s %-20s %s", item.UserID, item.OldLogin, item.NewLogin, st)
		if item.Error != "" {
			log("  error user_id=%d: %s", item.UserID, item.Error)
		}
	}

	if errors > 0 {
		result.Failed = errors
		result.Writes = 0
		_ = writeJSON(filepath.Join(opt.OutputDir, "preflight.json"), preflight)
		_ = writeJSON(filepath.Join(opt.OutputDir, "result.json"), result)
		return result, fmt.Errorf("preflight failed for %d user(s)", errors)
	}

	if err := writeJSON(filepath.Join(opt.OutputDir, "preflight.json"), preflight); err != nil {
		return nil, fmt.Errorf("write preflight: %w", err)
	}

	if !opt.Apply {
		result.Writes = 0
		log("mode=dry-run total=%d ready=%d already_migrated=%d errors=0 writes=0",
			result.Total, result.Ready, result.AlreadyMigrated)
		if err := writeJSON(filepath.Join(opt.OutputDir, "result.json"), result); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Apply: backup then sequential updates
	backup := make([]BackupUser, 0, len(plan))
	items := make([]ResultItem, 0, len(plan))
	for _, e := range plan {
		pf := findPreflight(preflight, e.UserID)
		ri := ResultItem{
			UserID:         e.UserID,
			OldLogin:       e.Login,
			NewLogin:       e.ProposedLogin,
			PreflightState: pf.State,
			ApplyState:     applyNotStarted,
		}
		if pf.State == stateMigrated {
			ri.ApplyState = applySkipped
		}
		items = append(items, ri)

		live := liveByID[e.UserID]
		if live == nil && pf.State == stateReady {
			u, err := client.GetUserByID(ctx, e.UserID)
			if err != nil {
				return nil, err
			}
			live = u
		}
		if live != nil {
			backup = append(backup, BackupUser{
				UserID: live.UserID, Login: live.Login, Login2: live.Login2,
				Balance: live.Balance, Bonus: live.Bonus, Credit: live.Credit,
				Settings: live.Settings,
			})
		} else if pf.State == stateMigrated {
			// still backup current migrated state
			u, err := client.GetUserByID(ctx, e.UserID)
			if err != nil {
				return nil, err
			}
			if u != nil {
				backup = append(backup, BackupUser{
					UserID: u.UserID, Login: u.Login, Login2: u.Login2,
					Balance: u.Balance, Bonus: u.Bonus, Credit: u.Credit,
					Settings: u.Settings,
				})
			}
		}
	}
	if err := writeJSON(filepath.Join(opt.OutputDir, "backup-before.json"), backup); err != nil {
		return nil, fmt.Errorf("write backup: %w", err)
	}

	updated := 0
	failed := 0
	writes := 0
	stopped := false
	for i, e := range plan {
		if stopped {
			continue
		}
		if items[i].PreflightState == stateMigrated {
			continue
		}
		before := liveByID[e.UserID]
		if before == nil {
			items[i].ApplyState = applyFailed
			items[i].Error = "missing live user snapshot"
			failed++
			stopped = true
			continue
		}
		settings, err := BuildUpdateSettings(before.Settings)
		if err != nil {
			items[i].ApplyState = applyFailed
			items[i].Error = err.Error()
			failed++
			stopped = true
			continue
		}
		payload := map[string]any{
			"user_id":  e.UserID,
			"login":    e.ProposedLogin,
			"settings": settings,
		}
		if err := client.UpdateUser(ctx, payload); err != nil {
			items[i].ApplyState = applyFailed
			items[i].Error = err.Error()
			failed++
			stopped = true
			continue
		}
		writes++
		if err := verifyAfterUpdate(ctx, client, e, before); err != nil {
			items[i].ApplyState = applyFailed
			items[i].Error = "post-update verification: " + err.Error()
			failed++
			stopped = true
			continue
		}
		items[i].ApplyState = applyUpdated
		updated++
	}

	result.Updated = updated
	result.Failed = failed
	result.Writes = writes
	result.Items = items
	log("mode=apply total=%d updated=%d already_migrated=%d failed=%d writes=%d",
		result.Total, result.Updated, result.AlreadyMigrated, result.Failed, result.Writes)
	if err := writeJSON(filepath.Join(opt.OutputDir, "result.json"), result); err != nil {
		return result, err
	}
	if failed > 0 {
		return result, fmt.Errorf("migration failed: updated=%d failed=%d", updated, failed)
	}
	return result, nil
}

func findPreflight(items []PreflightItem, userID int) PreflightItem {
	for _, it := range items {
		if it.UserID == userID {
			return it
		}
	}
	return PreflightItem{}
}

// StdLogger пишет в stderr.
func StdLogger() Logger {
	return func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}
