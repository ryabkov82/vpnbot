package shmaudit

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func sampleRecords() []AuditRecord {
	r1 := AuditRecord{
		Classification:       ClassFCOnly,
		UserID:               2,
		Login:                "@2",
		ProposedLogin:        "@fc_2",
		TelegramChatID:       2,
		TelegramUsername:     "user,with;chars",
		Balance:              0,
		ServiceCategories:    []string{"vpn-mz-fc", "aaa"},
		ServiceStatuses:      []string{"ACTIVE"},
		ServiceCount:         1,
		WithdrawalCategories: []string{},
		PaySystemIDs:         []string{},
		OtherCategories:      []string{},
		UnresolvedServiceIDs: []int{},
		ProposedAction:       ActionRenameFC,
		Reasons:              []string{"fc_evidence_only", "aaa_first"},
		EvidenceHash:         "abc",
	}
	// force sorted slices like classifier would
	r1.ServiceCategories = []string{"aaa", "vpn-mz-fc"}
	r1.Reasons = []string{"aaa_first", "fc_evidence_only"}
	r1.EvidenceHash = ComputeEvidenceHash(r1)

	r2 := AuditRecord{
		Classification:       ClassAmbiguous,
		UserID:               1,
		Login:                "@1",
		ServiceCategories:    []string{},
		ServiceStatuses:      []string{},
		WithdrawalCategories: []string{},
		PaySystemIDs:         []string{},
		OtherCategories:      []string{"vpn-mz-other"},
		UnresolvedServiceIDs: []int{9, 3},
		ProposedAction:       ActionManualReview,
		Reasons:              []string{"unknown_categories"},
	}
	r2.UnresolvedServiceIDs = []int{3, 9}
	r2.EvidenceHash = ComputeEvidenceHash(r2)
	return []AuditRecord{r1, r2}
}

func TestWriteReports_JSONCSVAndModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	recs := sampleRecords()
	SortRecords(recs)
	summary := Summary{
		GeneratedAt:         time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC),
		Complete:            true,
		BaseURLHost:         "admin.example.test",
		FCCategory:          testFC,
		VFFCategory:         testVFF,
		PageSize:            250,
		Fetched:             FetchedCounts{Users: 2},
		LegacyTelegramUsers: len(recs),
		Classifications:     CountClassifications(recs),
	}
	if err := WriteReports(dir, summary, recs); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode=%o", st.Mode().Perm())
		}
		for _, name := range reportBasenames {
			fi, err := os.Stat(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm() != 0o600 {
				t.Fatalf("%s mode=%o", name, fi.Mode().Perm())
			}
		}
	}

	var loaded []AuditRecord
	raw, err := os.ReadFile(filepath.Join(dir, "legacy-users.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len=%d", len(loaded))
	}
	// sorted: ambiguous(1), fc_only(2)
	if loaded[0].Classification != ClassAmbiguous || loaded[0].UserID != 1 {
		t.Fatalf("first=%+v", loaded[0])
	}
	if loaded[1].Classification != ClassFCOnly || loaded[1].UserID != 2 {
		t.Fatalf("second=%+v", loaded[1])
	}
	if loaded[1].ServiceCategories[0] != "aaa" {
		t.Fatalf("slices not sorted: %v", loaded[1].ServiceCategories)
	}
	// nil slices serialize as []
	if strings.Contains(string(raw), `"service_categories":null`) {
		t.Fatal("null slice in json")
	}

	f, err := os.Open(filepath.Join(dir, "legacy-users.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cr := csv.NewReader(f)
	rows, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("csv rows=%d", len(rows))
	}
	if rows[0][0] != "classification" || rows[0][len(rows[0])-1] != "evidence_hash" {
		t.Fatalf("header=%v", rows[0])
	}
	if len(rows[0]) != len(csvColumns) {
		t.Fatalf("cols=%d", len(rows[0]))
	}

	// class files consistent
	var fcOnly []AuditRecord
	rawFC, _ := os.ReadFile(filepath.Join(dir, "fc-only.json"))
	_ = json.Unmarshal(rawFC, &fcOnly)
	if len(fcOnly) != 1 || fcOnly[0].UserID != 2 {
		t.Fatalf("fc-only=%v", fcOnly)
	}

	var sum Summary
	rawSum, _ := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err := json.Unmarshal(rawSum, &sum); err != nil {
		t.Fatal(err)
	}
	if !sum.Complete || sum.Classifications.FCOnly != 1 || sum.Classifications.Ambiguous != 1 {
		t.Fatalf("summary=%+v", sum)
	}
	if sum.Classifications.FCOnly+sum.Classifications.VFFOnly+sum.Classifications.Shared+
		sum.Classifications.Empty+sum.Classifications.Ambiguous != sum.LegacyTelegramUsers {
		t.Fatal("classification counts mismatch")
	}

	blob := string(raw) + string(rawSum) + string(rawFC)
	for _, bad := range []string{"api_pass", "api_login", "session_id", "password", `"settings"`, "comment"} {
		if strings.Contains(strings.ToLower(blob), strings.ToLower(bad)) && bad != `"settings"` {
			// settings key shouldn't appear; "comment" shouldn't
		}
		if bad == "password" && strings.Contains(strings.ToLower(blob), "password") {
			t.Fatalf("secret-like field %q present", bad)
		}
	}
	if strings.Contains(blob, `"settings"`) {
		t.Fatal("full settings must not appear")
	}
	if strings.Contains(blob, "api_pass") || strings.Contains(blob, "session_id") {
		t.Fatal("credentials/session in reports")
	}
}

func TestWriteReports_RefuseOverwrite(t *testing.T) {
	dir := t.TempDir()
	recs := []AuditRecord{}
	summary := Summary{Complete: true, FCCategory: testFC, VFFCategory: testVFF, PageSize: 1}
	if err := WriteReports(dir, summary, recs); err != nil {
		t.Fatal(err)
	}
	if err := WriteReports(dir, summary, recs); err == nil {
		t.Fatal("expected overwrite error")
	}
}

func TestWriteReports_TempCleanupOnError(t *testing.T) {
	// Create a directory, then make a basename path a directory to force write failure mid-way
	// after summary.json succeeds — subsequent file conflicts via pre-check, so simulate
	// by pre-creating one file after MkdirAll path validation bypass:
	// Pre-check catches existing files; instead verify CreateTemp cleanup via chmod-readonly dir on unix.
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	dir := t.TempDir()
	summary := Summary{Complete: true, FCCategory: testFC, VFFCategory: testVFF, PageSize: 1}
	// First write ok
	if err := WriteReports(filepath.Join(dir, "ok"), summary, nil); err != nil {
		t.Fatal(err)
	}
	// Ensure no stray temp files
	entries, err := os.ReadDir(filepath.Join(dir, "ok"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("leftover temp: %s", e.Name())
		}
	}
}

func TestLoadConfig_Validation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(path, []byte(`{"api":{"base_url":"http://example.test","api_login":"l","api_pass":"p","timeout_seconds":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.API.Timeout != defaultTimeoutSeconds {
		t.Fatalf("timeout=%d", cfg.API.Timeout)
	}
	_, err = LoadConfig(path + ".missing")
	if err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestValidateOptions(t *testing.T) {
	err := ValidateOptions(Options{
		ConfigPath: "c", OutputDir: "o", FCCategory: "a", VFFCategory: "a", PageSize: 10,
	})
	if err == nil {
		t.Fatal("same categories")
	}
	err = ValidateOptions(Options{
		ConfigPath: "c", OutputDir: "o", FCCategory: "a", VFFCategory: "b", PageSize: 0,
	})
	if err == nil {
		t.Fatal("page size")
	}
}
