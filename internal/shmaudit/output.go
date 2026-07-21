package shmaudit

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var reportBasenames = []string{
	"summary.json",
	"legacy-users.json",
	"legacy-users.csv",
	"fc-only.json",
	"fc-only.csv",
	"vff-only.json",
	"vff-only.csv",
	"shared.json",
	"shared.csv",
	"empty.json",
	"empty.csv",
	"ambiguous.json",
	"ambiguous.csv",
}

var csvColumns = []string{
	"classification",
	"user_id",
	"login",
	"proposed_login",
	"telegram_chat_id",
	"telegram_username",
	"created",
	"last_login",
	"brand_id",
	"balance",
	"bonus",
	"credit",
	"login2_present",
	"service_categories",
	"service_statuses",
	"service_count",
	"withdrawal_categories",
	"withdrawal_count",
	"payment_count",
	"pay_system_ids",
	"other_categories",
	"unresolved_service_ids",
	"target_login_exists",
	"target_login_user_id",
	"proposed_action",
	"reasons",
	"evidence_hash",
}

// WriteReports создаёт output directory (0700) и атомарно пишет все отчёты (0600).
func WriteReports(outputDir string, summary Summary, records []AuditRecord) error {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		return fmt.Errorf("output directory is empty")
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.Chmod(outputDir, 0o700); err != nil {
		return fmt.Errorf("chmod output directory: %w", err)
	}

	for _, name := range reportBasenames {
		path := filepath.Join(outputDir, name)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("output file already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat output file %s: %w", path, err)
		}
	}

	// Normalize nil slices before serialize.
	for i := range records {
		normalizeRecord(&records[i])
	}

	classFiles := []struct {
		class    string
		jsonName string
		csvName  string
	}{
		{ClassFCOnly, "fc-only.json", "fc-only.csv"},
		{ClassVFFOnly, "vff-only.json", "vff-only.csv"},
		{ClassShared, "shared.json", "shared.csv"},
		{ClassEmpty, "empty.json", "empty.csv"},
		{ClassAmbiguous, "ambiguous.json", "ambiguous.csv"},
	}

	if err := writeJSONAtomic(filepath.Join(outputDir, "summary.json"), summary); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(outputDir, "legacy-users.json"), records); err != nil {
		return err
	}
	if err := writeCSVAtomic(filepath.Join(outputDir, "legacy-users.csv"), records); err != nil {
		return err
	}
	for _, cf := range classFiles {
		subset := FilterByClass(records, cf.class)
		if err := writeJSONAtomic(filepath.Join(outputDir, cf.jsonName), subset); err != nil {
			return err
		}
		if err := writeCSVAtomic(filepath.Join(outputDir, cf.csvName), subset); err != nil {
			return err
		}
	}
	return nil
}

func normalizeRecord(r *AuditRecord) {
	r.ServiceCategories = ensureStringSlice(r.ServiceCategories)
	r.ServiceStatuses = ensureStringSlice(r.ServiceStatuses)
	r.WithdrawalCategories = ensureStringSlice(r.WithdrawalCategories)
	r.PaySystemIDs = ensureStringSlice(r.PaySystemIDs)
	r.OtherCategories = ensureStringSlice(r.OtherCategories)
	r.UnresolvedServiceIDs = ensureIntSlice(r.UnresolvedServiceIDs)
	r.Reasons = ensureStringSlice(r.Reasons)
}

func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data)
}

func writeCSVAtomic(path string, records []AuditRecord) error {
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	if err := w.Write(csvColumns); err != nil {
		return fmt.Errorf("csv header: %w", err)
	}
	for _, r := range records {
		if err := w.Write(recordCSVRow(r)); err != nil {
			return fmt.Errorf("csv row user_id=%d: %w", r.UserID, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("csv flush: %w", err)
	}
	return writeFileAtomic(path, []byte(buf.String()))
}

func recordCSVRow(r AuditRecord) []string {
	chatID := ""
	if r.TelegramChatID != 0 {
		chatID = strconv.FormatInt(r.TelegramChatID, 10)
	}
	targetUID := ""
	if r.TargetLoginUserID != 0 {
		targetUID = strconv.Itoa(r.TargetLoginUserID)
	}
	return []string{
		r.Classification,
		strconv.Itoa(r.UserID),
		r.Login,
		r.ProposedLogin,
		chatID,
		r.TelegramUsername,
		r.Created,
		r.LastLogin,
		r.BrandID,
		formatFloat(r.Balance),
		formatFloat(r.Bonus),
		formatFloat(r.Credit),
		strconv.FormatBool(r.Login2Present),
		joinStrings(r.ServiceCategories),
		joinStrings(r.ServiceStatuses),
		strconv.Itoa(r.ServiceCount),
		joinStrings(r.WithdrawalCategories),
		strconv.Itoa(r.WithdrawalCount),
		strconv.Itoa(r.PaymentCount),
		joinStrings(r.PaySystemIDs),
		joinStrings(r.OtherCategories),
		joinInts(r.UnresolvedServiceIDs),
		strconv.FormatBool(r.TargetLoginExists),
		targetUID,
		r.ProposedAction,
		joinStrings(r.Reasons),
		r.EvidenceHash,
	}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func joinStrings(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return strings.Join(ss, ";;")
}

func joinInts(is []int) string {
	if len(is) == 0 {
		return ""
	}
	parts := make([]string, len(is))
	for i, v := range is {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ";;")
}

func writeFileAtomic(finalPath string, data []byte) error {
	dir := filepath.Dir(finalPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(finalPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", filepath.Base(finalPath), err)
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
		return fmt.Errorf("chmod temp %s: %w", filepath.Base(finalPath), err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp %s: %w", filepath.Base(finalPath), err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp %s: %w", filepath.Base(finalPath), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", filepath.Base(finalPath), err)
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		return fmt.Errorf("rename %s: %w", filepath.Base(finalPath), err)
	}
	cleanup = false
	if err := os.Chmod(finalPath, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", filepath.Base(finalPath), err)
	}
	return nil
}
