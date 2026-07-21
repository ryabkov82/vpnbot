package shmaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var atDigitsLogin = regexp.MustCompile(`^@[0-9]+$`)

// ParseSettings безопасно извлекает brand_id и telegram hints из settings.
func ParseSettings(raw json.RawMessage) SettingsHints {
	var out SettingsHints
	if len(raw) == 0 || string(raw) == "null" {
		return out
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return out
	}
	if brandRaw, ok := obj["brand_id"]; ok {
		out.BrandIDPresent = true
		var brand string
		if err := json.Unmarshal(brandRaw, &brand); err == nil {
			out.BrandID = strings.TrimSpace(brand)
		}
	}
	telegramRaw, telegramOK := obj["telegram"]
	out.TelegramKeyPresent = telegramOK
	if !telegramOK || len(telegramRaw) == 0 || string(telegramRaw) == "null" {
		return out
	}
	out.Telegram.Present = true
	var tg map[string]json.RawMessage
	if err := json.Unmarshal(telegramRaw, &tg); err != nil {
		return out
	}
	if uRaw, ok := tg["username"]; ok {
		var u string
		if err := json.Unmarshal(uRaw, &u); err == nil {
			out.Telegram.Username = strings.TrimSpace(u)
		}
	}
	chatRaw, ok := tg["chat_id"]
	if !ok || len(chatRaw) == 0 || string(chatRaw) == "null" {
		return out
	}
	chatID, valid := parsePositiveInt64(chatRaw)
	out.Telegram.ChatID = chatID
	out.Telegram.ChatIDValid = valid
	out.Telegram.ChatIDRawOK = valid
	return out
}

func parsePositiveInt64(raw json.RawMessage) (int64, bool) {
	// JSON number
	var n json.Number
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return 0, false
	}
	switch t := v.(type) {
	case json.Number:
		n = t
		i, err := n.Int64()
		if err != nil {
			f, err2 := n.Float64()
			if err2 != nil {
				return 0, false
			}
			i = int64(f)
			if float64(i) != f {
				return 0, false
			}
		}
		if i > 0 {
			return i, true
		}
		return 0, false
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil || i <= 0 {
			return 0, false
		}
		return i, true
	case float64:
		i := int64(t)
		if float64(i) != t || i <= 0 {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// IsLegacyTelegramCandidate — brand_id пуст и есть Telegram-признаки.
func IsLegacyTelegramCandidate(user AuditUser, hints SettingsHints) bool {
	if strings.TrimSpace(hints.BrandID) != "" {
		return false
	}
	if hints.TelegramKeyPresent {
		return true
	}
	return atDigitsLogin.MatchString(strings.TrimSpace(user.Login))
}

// ValidLegacyIdentity — chat_id > 0 и login == @<chat_id>.
func ValidLegacyIdentity(login string, hints SettingsHints) bool {
	if !hints.Telegram.ChatIDValid || hints.Telegram.ChatID <= 0 {
		return false
	}
	want := fmt.Sprintf("@%d", hints.Telegram.ChatID)
	return strings.TrimSpace(login) == want
}

// ProposedFCLogin возвращает @fc_<chat_id>.
func ProposedFCLogin(chatID int64) string {
	return fmt.Sprintf("@fc_%d", chatID)
}

type userEvidence struct {
	serviceCategories    []string
	serviceStatuses      []string
	serviceCount         int
	withdrawalCategories []string
	withdrawalCount      int
	paymentCount         int
	paySystemIDs         []string
	otherCategories      []string
	unresolvedServiceIDs []int
	hasFC                bool
	hasVFF               bool
}

func collectEvidence(
	userID int,
	fcCategory, vffCategory string,
	servicesByUser map[int][]AuditUserService,
	withdrawsByUser map[int][]AuditWithdraw,
	paysByUser map[int][]AuditPay,
	serviceCatalog map[int]AuditService,
) userEvidence {
	var ev userEvidence
	catSet := map[string]struct{}{}
	statusSet := map[string]struct{}{}
	withdrawCatSet := map[string]struct{}{}
	otherSet := map[string]struct{}{}
	unresolvedSet := map[int]struct{}{}
	paySysSet := map[string]struct{}{}

	for _, us := range servicesByUser[userID] {
		ev.serviceCount++
		cat := strings.TrimSpace(us.Category)
		if cat != "" {
			catSet[cat] = struct{}{}
		}
		st := strings.TrimSpace(us.Status)
		if st != "" {
			statusSet[st] = struct{}{}
		}
	}
	for _, w := range withdrawsByUser[userID] {
		ev.withdrawalCount++
		svc, ok := serviceCatalog[w.ServiceID]
		if !ok {
			if w.ServiceID != 0 {
				unresolvedSet[w.ServiceID] = struct{}{}
			}
			continue
		}
		cat := strings.TrimSpace(svc.Category)
		if cat != "" {
			withdrawCatSet[cat] = struct{}{}
			catSet[cat] = struct{}{}
		}
	}
	for _, p := range paysByUser[userID] {
		ev.paymentCount++
		ps := strings.TrimSpace(p.PaySystemID)
		if ps != "" {
			paySysSet[ps] = struct{}{}
		}
	}

	for cat := range catSet {
		switch cat {
		case fcCategory:
			ev.hasFC = true
		case vffCategory:
			ev.hasVFF = true
		default:
			otherSet[cat] = struct{}{}
		}
	}
	svcOnly := map[string]struct{}{}
	for _, us := range servicesByUser[userID] {
		cat := strings.TrimSpace(us.Category)
		if cat != "" {
			svcOnly[cat] = struct{}{}
		}
	}

	ev.serviceCategories = sortedKeys(svcOnly)
	ev.serviceStatuses = sortedKeys(statusSet)
	ev.withdrawalCategories = sortedKeys(withdrawCatSet)
	ev.paySystemIDs = sortedKeys(paySysSet)
	ev.otherCategories = sortedKeys(otherSet)
	ev.unresolvedServiceIDs = sortedInts(unresolvedSet)
	return ev
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedInts(m map[int]struct{}) []int {
	if len(m) == 0 {
		return []int{}
	}
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// ClassifyUser классифицирует одного legacy-кандидата.
func ClassifyUser(
	user AuditUser,
	hints SettingsHints,
	fcCategory, vffCategory string,
	servicesByUser map[int][]AuditUserService,
	withdrawsByUser map[int][]AuditWithdraw,
	paysByUser map[int][]AuditPay,
	serviceCatalog map[int]AuditService,
	loginIndex map[string]int,
) AuditRecord {
	ev := collectEvidence(user.UserID, fcCategory, vffCategory, servicesByUser, withdrawsByUser, paysByUser, serviceCatalog)

	rec := AuditRecord{
		UserID:               user.UserID,
		Login:                user.Login,
		Created:              user.Created,
		LastLogin:            user.LastLogin,
		BrandID:              hints.BrandID,
		Balance:              user.Balance,
		Bonus:                user.Bonus,
		Credit:               user.Credit,
		Login2Present:        strings.TrimSpace(user.Login2) != "",
		ServiceCategories:    ensureStringSlice(ev.serviceCategories),
		ServiceStatuses:      ensureStringSlice(ev.serviceStatuses),
		ServiceCount:         ev.serviceCount,
		WithdrawalCategories: ensureStringSlice(ev.withdrawalCategories),
		WithdrawalCount:      ev.withdrawalCount,
		PaymentCount:         ev.paymentCount,
		PaySystemIDs:         ensureStringSlice(ev.paySystemIDs),
		OtherCategories:      ensureStringSlice(ev.otherCategories),
		UnresolvedServiceIDs: ensureIntSlice(ev.unresolvedServiceIDs),
		Reasons:              []string{},
	}
	if hints.Telegram.Username != "" {
		rec.TelegramUsername = hints.Telegram.Username
	}
	if hints.Telegram.ChatIDValid {
		rec.TelegramChatID = hints.Telegram.ChatID
		rec.ProposedLogin = ProposedFCLogin(hints.Telegram.ChatID)
		if uid, exists := loginIndex[rec.ProposedLogin]; exists && uid != user.UserID {
			rec.TargetLoginExists = true
			rec.TargetLoginUserID = uid
		}
	}

	identityOK := ValidLegacyIdentity(user.Login, hints)
	reasons := []string{}

	if !hints.Telegram.ChatIDValid {
		reasons = append(reasons, "invalid_or_missing_telegram_chat_id")
	} else if !identityOK {
		reasons = append(reasons, "login_chat_id_mismatch")
	}
	if len(ev.otherCategories) > 0 {
		reasons = append(reasons, "unknown_categories")
	}
	if len(ev.unresolvedServiceIDs) > 0 {
		reasons = append(reasons, "unresolved_service_ids")
	}
	if rec.TargetLoginExists {
		reasons = append(reasons, "target_login_occupied")
	}

	hasAnyEvidence := ev.hasFC || ev.hasVFF || len(ev.otherCategories) > 0 || len(ev.unresolvedServiceIDs) > 0
	emptyLike := ev.serviceCount == 0 && ev.withdrawalCount == 0 && ev.paymentCount == 0 &&
		user.Balance == 0 && user.Bonus == 0 && user.Credit == 0 && !hasAnyEvidence

	switch {
	case !identityOK || !hints.Telegram.ChatIDValid:
		rec.Classification = ClassAmbiguous
		rec.ProposedAction = ActionManualReview
		if len(reasons) == 0 {
			reasons = append(reasons, "invalid_legacy_identity")
		}

	case len(ev.otherCategories) > 0 || len(ev.unresolvedServiceIDs) > 0:
		rec.Classification = ClassAmbiguous
		rec.ProposedAction = ActionManualReview

	case ev.hasFC && ev.hasVFF:
		rec.Classification = ClassShared
		rec.ProposedAction = ActionManualReview
		reasons = append(reasons, "fc_and_vff_evidence")

	case ev.hasFC && !ev.hasVFF:
		if rec.TargetLoginExists {
			rec.Classification = ClassAmbiguous
			rec.ProposedAction = ActionManualReview
		} else {
			rec.Classification = ClassFCOnly
			rec.ProposedAction = ActionRenameFC
			reasons = append(reasons, "fc_evidence_only")
		}

	case ev.hasVFF && !ev.hasFC:
		rec.Classification = ClassVFFOnly
		rec.ProposedAction = ActionDoNotMigrate
		reasons = append(reasons, "vff_evidence_only")

	case emptyLike:
		rec.Classification = ClassEmpty
		rec.ProposedAction = ActionDoNotMigrateAuto
		reasons = append(reasons, "brand_cannot_be_determined")

	default:
		rec.Classification = ClassAmbiguous
		rec.ProposedAction = ActionManualReview
		if ev.serviceCount == 0 && ev.paymentCount > 0 {
			reasons = append(reasons, "no_services_but_payments")
		}
		if ev.serviceCount == 0 && user.Balance != 0 {
			reasons = append(reasons, "no_services_but_balance")
		}
		if ev.serviceCount == 0 && user.Bonus != 0 {
			reasons = append(reasons, "no_services_but_bonus")
		}
		if ev.serviceCount == 0 && user.Credit != 0 {
			reasons = append(reasons, "no_services_but_credit")
		}
		if ev.serviceCount == 0 && ev.withdrawalCount > 0 {
			reasons = append(reasons, "no_services_but_withdrawals")
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "unclassified")
		}
	}

	rec.Reasons = uniqueSorted(reasons)
	rec.EvidenceHash = ComputeEvidenceHash(rec)
	return rec
}

func ensureStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func ensureIntSlice(s []int) []int {
	if s == nil {
		return []int{}
	}
	return s
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	m := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			m[s] = struct{}{}
		}
	}
	return sortedKeys(m)
}

// evidenceHashPayload — стабильная структура для SHA-256.
type evidenceHashPayload struct {
	UserID               int      `json:"user_id"`
	Login                string   `json:"login"`
	TelegramChatID       int64    `json:"telegram_chat_id"`
	BrandID              string   `json:"brand_id"`
	Balance              float64  `json:"balance"`
	Bonus                float64  `json:"bonus"`
	Credit               float64  `json:"credit"`
	Login2Present        bool     `json:"login2_present"`
	ServiceCategories    []string `json:"service_categories"`
	ServiceStatuses      []string `json:"service_statuses"`
	ServiceCount         int      `json:"service_count"`
	WithdrawalCategories []string `json:"withdrawal_categories"`
	WithdrawalCount      int      `json:"withdrawal_count"`
	PaymentCount         int      `json:"payment_count"`
	PaySystemIDs         []string `json:"pay_system_ids"`
	OtherCategories      []string `json:"other_categories"`
	UnresolvedServiceIDs []int    `json:"unresolved_service_ids"`
	TargetLoginExists    bool     `json:"target_login_exists"`
	TargetLoginUserID    int      `json:"target_login_user_id"`
}

// ComputeEvidenceHash — SHA-256 от нормализованных classification-affecting данных.
func ComputeEvidenceHash(rec AuditRecord) string {
	payload := evidenceHashPayload{
		UserID:               rec.UserID,
		Login:                rec.Login,
		TelegramChatID:       rec.TelegramChatID,
		BrandID:              rec.BrandID,
		Balance:              rec.Balance,
		Bonus:                rec.Bonus,
		Credit:               rec.Credit,
		Login2Present:        rec.Login2Present,
		ServiceCategories:    append([]string{}, rec.ServiceCategories...),
		ServiceStatuses:      append([]string{}, rec.ServiceStatuses...),
		ServiceCount:         rec.ServiceCount,
		WithdrawalCategories: append([]string{}, rec.WithdrawalCategories...),
		WithdrawalCount:      rec.WithdrawalCount,
		PaymentCount:         rec.PaymentCount,
		PaySystemIDs:         append([]string{}, rec.PaySystemIDs...),
		OtherCategories:      append([]string{}, rec.OtherCategories...),
		UnresolvedServiceIDs: append([]int{}, rec.UnresolvedServiceIDs...),
		TargetLoginExists:    rec.TargetLoginExists,
		TargetLoginUserID:    rec.TargetLoginUserID,
	}
	sort.Strings(payload.ServiceCategories)
	sort.Strings(payload.ServiceStatuses)
	sort.Strings(payload.WithdrawalCategories)
	sort.Strings(payload.PaySystemIDs)
	sort.Strings(payload.OtherCategories)
	sort.Ints(payload.UnresolvedServiceIDs)
	raw, err := json.Marshal(payload)
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("user_id=%d", rec.UserID)))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// BuildLoginIndex строит login → user_id.
func BuildLoginIndex(users []AuditUser) map[string]int {
	idx := make(map[string]int, len(users))
	for _, u := range users {
		login := strings.TrimSpace(u.Login)
		if login == "" {
			continue
		}
		if _, exists := idx[login]; !exists {
			idx[login] = u.UserID
		}
	}
	return idx
}

// ClassifyAll находит legacy-кандидатов и классифицирует их.
func ClassifyAll(ds Dataset, fcCategory, vffCategory string) []AuditRecord {
	fcCategory = strings.TrimSpace(fcCategory)
	vffCategory = strings.TrimSpace(vffCategory)

	servicesByUser := map[int][]AuditUserService{}
	for _, s := range ds.UserServices {
		servicesByUser[s.UserID] = append(servicesByUser[s.UserID], s)
	}
	withdrawsByUser := map[int][]AuditWithdraw{}
	for _, w := range ds.Withdrawals {
		withdrawsByUser[w.UserID] = append(withdrawsByUser[w.UserID], w)
	}
	paysByUser := map[int][]AuditPay{}
	for _, p := range ds.Payments {
		paysByUser[p.UserID] = append(paysByUser[p.UserID], p)
	}
	catalog := map[int]AuditService{}
	for _, s := range ds.Services {
		catalog[s.ServiceID] = s
	}
	loginIndex := BuildLoginIndex(ds.Users)

	var out []AuditRecord
	for _, u := range ds.Users {
		hints := ParseSettings(u.Settings)
		if !IsLegacyTelegramCandidate(u, hints) {
			continue
		}
		out = append(out, ClassifyUser(
			u, hints, fcCategory, vffCategory,
			servicesByUser, withdrawsByUser, paysByUser, catalog, loginIndex,
		))
	}
	SortRecords(out)
	return out
}

// SortRecords сортирует по classification, затем user_id.
func SortRecords(recs []AuditRecord) {
	sort.SliceStable(recs, func(i, j int) bool {
		if recs[i].Classification != recs[j].Classification {
			return recs[i].Classification < recs[j].Classification
		}
		return recs[i].UserID < recs[j].UserID
	})
}

// CountClassifications считает классы.
func CountClassifications(recs []AuditRecord) ClassificationCounts {
	var c ClassificationCounts
	for _, r := range recs {
		switch r.Classification {
		case ClassFCOnly:
			c.FCOnly++
		case ClassVFFOnly:
			c.VFFOnly++
		case ClassShared:
			c.Shared++
		case ClassEmpty:
			c.Empty++
		case ClassAmbiguous:
			c.Ambiguous++
		}
	}
	return c
}

// FilterByClass возвращает записи указанного класса.
func FilterByClass(recs []AuditRecord, class string) []AuditRecord {
	out := make([]AuditRecord, 0)
	for _, r := range recs {
		if r.Classification == class {
			out = append(out, r)
		}
	}
	return out
}
