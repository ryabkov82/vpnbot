package shmaudit

import (
	"encoding/json"
	"testing"
)

const (
	testFC  = "vpn-mz-fc"
	testVFF = "vpn-mz-test"
)

func settingsJSON(brandID string, chatID any, username string, withTelegram bool) json.RawMessage {
	m := map[string]any{}
	if brandID != "" {
		m["brand_id"] = brandID
	}
	if withTelegram {
		tg := map[string]any{}
		if chatID != nil {
			tg["chat_id"] = chatID
		}
		if username != "" {
			tg["username"] = username
		}
		m["telegram"] = tg
	}
	raw, _ := json.Marshal(m)
	return raw
}

func baseUser(id int, login string, settings json.RawMessage) AuditUser {
	return AuditUser{
		UserID:   id,
		Login:    login,
		Settings: settings,
	}
}

func classifyOne(t *testing.T, ds Dataset, userID int) AuditRecord {
	t.Helper()
	recs := ClassifyAll(ds, testFC, testVFF)
	for _, r := range recs {
		if r.UserID == userID {
			return r
		}
	}
	t.Fatalf("user %d not in legacy candidates", userID)
	return AuditRecord{}
}

func TestClassify_FCOnly(t *testing.T) {
	u := baseUser(1, "@685999844", settingsJSON("", int64(685999844), "alice", true))
	ds := Dataset{
		Users: []AuditUser{u},
		UserServices: []AuditUserService{
			{UserID: 1, UserServiceID: 10, ServiceID: 1, Category: testFC, Status: "ACTIVE"},
		},
		Services: []AuditService{{ServiceID: 1, Category: testFC}},
	}
	rec := classifyOne(t, ds, 1)
	if rec.Classification != ClassFCOnly {
		t.Fatalf("got %s want fc_only", rec.Classification)
	}
	if rec.ProposedLogin != "@fc_685999844" {
		t.Fatalf("proposed_login=%q", rec.ProposedLogin)
	}
	if rec.ProposedAction != ActionRenameFC {
		t.Fatalf("action=%q", rec.ProposedAction)
	}
}

func TestClassify_VFFOnly(t *testing.T) {
	u := baseUser(2, "@100", settingsJSON("", 100, "", true))
	ds := Dataset{
		Users: []AuditUser{u},
		UserServices: []AuditUserService{
			{UserID: 2, Category: testVFF, Status: "ACTIVE"},
		},
	}
	rec := classifyOne(t, ds, 2)
	if rec.Classification != ClassVFFOnly {
		t.Fatalf("got %s", rec.Classification)
	}
	if rec.ProposedAction != ActionDoNotMigrate {
		t.Fatalf("action=%q", rec.ProposedAction)
	}
}

func TestClassify_Shared(t *testing.T) {
	u := baseUser(3, "@200", settingsJSON("", 200, "", true))
	ds := Dataset{
		Users: []AuditUser{u},
		UserServices: []AuditUserService{
			{UserID: 3, Category: testFC},
			{UserID: 3, Category: testVFF},
		},
	}
	rec := classifyOne(t, ds, 3)
	if rec.Classification != ClassShared {
		t.Fatalf("got %s", rec.Classification)
	}
	if rec.ProposedAction != ActionManualReview {
		t.Fatalf("action=%q", rec.ProposedAction)
	}
}

func TestClassify_Empty(t *testing.T) {
	u := baseUser(4, "@300", settingsJSON("", 300, "", true))
	ds := Dataset{Users: []AuditUser{u}}
	rec := classifyOne(t, ds, 4)
	if rec.Classification != ClassEmpty {
		t.Fatalf("got %s", rec.Classification)
	}
	if rec.ProposedAction != ActionDoNotMigrateAuto {
		t.Fatalf("action=%q", rec.ProposedAction)
	}
}

func TestClassify_NoServicesButPayment(t *testing.T) {
	u := baseUser(5, "@400", settingsJSON("", 400, "", true))
	ds := Dataset{
		Users:    []AuditUser{u},
		Payments: []AuditPay{{ID: 1, UserID: 5, Money: 10, PaySystemID: "yookassa"}},
	}
	rec := classifyOne(t, ds, 5)
	if rec.Classification != ClassAmbiguous {
		t.Fatalf("got %s", rec.Classification)
	}
}

func TestClassify_NoServicesButBalance(t *testing.T) {
	u := baseUser(6, "@500", settingsJSON("", 500, "", true))
	u.Balance = 1.5
	ds := Dataset{Users: []AuditUser{u}}
	rec := classifyOne(t, ds, 6)
	if rec.Classification != ClassAmbiguous {
		t.Fatalf("got %s", rec.Classification)
	}
}

func TestClassify_FCWithdrawalOnly(t *testing.T) {
	u := baseUser(7, "@600", settingsJSON("", 600, "", true))
	ds := Dataset{
		Users: []AuditUser{u},
		Services: []AuditService{
			{ServiceID: 17, Category: testFC},
		},
		Withdrawals: []AuditWithdraw{
			{WithdrawID: 1, UserID: 7, ServiceID: 17, Total: 100},
		},
	}
	rec := classifyOne(t, ds, 7)
	if rec.Classification != ClassFCOnly {
		t.Fatalf("got %s want fc_only", rec.Classification)
	}
	if len(rec.WithdrawalCategories) != 1 || rec.WithdrawalCategories[0] != testFC {
		t.Fatalf("withdrawal_categories=%v", rec.WithdrawalCategories)
	}
}

func TestClassify_UnknownServiceID(t *testing.T) {
	u := baseUser(8, "@700", settingsJSON("", 700, "", true))
	ds := Dataset{
		Users: []AuditUser{u},
		Withdrawals: []AuditWithdraw{
			{WithdrawID: 1, UserID: 8, ServiceID: 17},
		},
	}
	rec := classifyOne(t, ds, 8)
	if rec.Classification != ClassAmbiguous {
		t.Fatalf("got %s", rec.Classification)
	}
	if len(rec.UnresolvedServiceIDs) != 1 || rec.UnresolvedServiceIDs[0] != 17 {
		t.Fatalf("unresolved=%v", rec.UnresolvedServiceIDs)
	}
}

func TestClassify_UnknownCategory(t *testing.T) {
	u := baseUser(9, "@800", settingsJSON("", 800, "", true))
	ds := Dataset{
		Users: []AuditUser{u},
		UserServices: []AuditUserService{
			{UserID: 9, Category: "vpn-mz-other"},
		},
	}
	rec := classifyOne(t, ds, 9)
	if rec.Classification != ClassAmbiguous {
		t.Fatalf("got %s", rec.Classification)
	}
	if len(rec.OtherCategories) != 1 || rec.OtherCategories[0] != "vpn-mz-other" {
		t.Fatalf("other=%v", rec.OtherCategories)
	}
}

func TestClassify_TargetOccupied(t *testing.T) {
	legacy := baseUser(10, "@900", settingsJSON("", 900, "", true))
	occupant := baseUser(99, "@fc_900", settingsJSON("fc", 900, "", true))
	ds := Dataset{
		Users: []AuditUser{legacy, occupant},
		UserServices: []AuditUserService{
			{UserID: 10, Category: testFC},
		},
	}
	rec := classifyOne(t, ds, 10)
	if rec.Classification != ClassAmbiguous {
		t.Fatalf("got %s", rec.Classification)
	}
	if !rec.TargetLoginExists || rec.TargetLoginUserID != 99 {
		t.Fatalf("target exists=%v uid=%d", rec.TargetLoginExists, rec.TargetLoginUserID)
	}
}

func TestClassify_LoginChatMismatch(t *testing.T) {
	u := baseUser(11, "@123", settingsJSON("", 999, "", true))
	ds := Dataset{
		Users: []AuditUser{u},
		UserServices: []AuditUserService{
			{UserID: 11, Category: testFC},
		},
	}
	rec := classifyOne(t, ds, 11)
	if rec.Classification != ClassAmbiguous {
		t.Fatalf("got %s", rec.Classification)
	}
}

func TestClassify_InvalidChatID(t *testing.T) {
	cases := []struct {
		name   string
		chatID any
	}{
		{"missing", nil},
		{"garbage", "not-a-number"},
		{"zero", 0},
		{"negative", -5},
		{"null_via_omit", "omit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var settings json.RawMessage
			if tc.name == "null_via_omit" || tc.chatID == nil {
				settings = json.RawMessage(`{"telegram":{}}`)
			} else {
				settings = settingsJSON("", tc.chatID, "", true)
			}
			u := baseUser(12, "@1000", settings)
			// login matches @digits so still a legacy candidate via login format
			ds := Dataset{Users: []AuditUser{u}}
			rec := classifyOne(t, ds, 12)
			if rec.Classification != ClassAmbiguous {
				t.Fatalf("got %s", rec.Classification)
			}
		})
	}
}

func TestClassify_ExistingBrandExcluded(t *testing.T) {
	u := baseUser(13, "@1100", settingsJSON("fc", 1100, "", true))
	ds := Dataset{
		Users: []AuditUser{u},
		UserServices: []AuditUserService{
			{UserID: 13, Category: testFC},
		},
	}
	recs := ClassifyAll(ds, testFC, testVFF)
	for _, r := range recs {
		if r.UserID == 13 {
			t.Fatal("brand_id=fc must not be a legacy candidate")
		}
	}
}

func TestClassify_ChatIDNumericString(t *testing.T) {
	u := baseUser(14, "@1200", settingsJSON("", "1200", "bob", true))
	ds := Dataset{
		Users: []AuditUser{u},
		UserServices: []AuditUserService{
			{UserID: 14, Category: testFC},
		},
	}
	rec := classifyOne(t, ds, 14)
	if rec.Classification != ClassFCOnly {
		t.Fatalf("got %s", rec.Classification)
	}
	if rec.TelegramChatID != 1200 {
		t.Fatalf("chat_id=%d", rec.TelegramChatID)
	}
}

func TestClassify_Determinism(t *testing.T) {
	mk := func() Dataset {
		return Dataset{
			Users: []AuditUser{
				baseUser(30, "@30", settingsJSON("", 30, "z", true)),
				baseUser(20, "@20", settingsJSON("", 20, "a", true)),
			},
			UserServices: []AuditUserService{
				{UserID: 30, Category: testVFF, Status: "BLOCK"},
				{UserID: 20, Category: testFC, Status: "ACTIVE"},
				{UserID: 20, Category: testFC, Status: "ACTIVE"},
			},
			Payments: []AuditPay{
				{ID: 2, UserID: 20, PaySystemID: "b"},
				{ID: 1, UserID: 20, PaySystemID: "a"},
			},
		}
	}
	a := ClassifyAll(mk(), testFC, testVFF)
	// reverse input order
	ds := mk()
	ds.Users[0], ds.Users[1] = ds.Users[1], ds.Users[0]
	ds.UserServices[0], ds.UserServices[2] = ds.UserServices[2], ds.UserServices[0]
	b := ClassifyAll(ds, testFC, testVFF)
	if len(a) != len(b) {
		t.Fatalf("len %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Classification != b[i].Classification || a[i].UserID != b[i].UserID {
			t.Fatalf("order mismatch at %d: %+v vs %+v", i, a[i], b[i])
		}
		if a[i].EvidenceHash != b[i].EvidenceHash {
			t.Fatalf("hash mismatch user %d", a[i].UserID)
		}
	}
}

func TestParseSettings_ChatIDVariants(t *testing.T) {
	raw := json.RawMessage(`{"telegram":{"chat_id":"42","username":"x"}}`)
	h := ParseSettings(raw)
	if !h.Telegram.ChatIDValid || h.Telegram.ChatID != 42 {
		t.Fatalf("%+v", h.Telegram)
	}
	raw2 := json.RawMessage(`{"telegram":{"chat_id":null}}`)
	h2 := ParseSettings(raw2)
	if h2.Telegram.ChatIDValid {
		t.Fatal("null chat_id must be invalid")
	}
}

func TestIsLegacy_LoginAtDigitsWithoutTelegramKey(t *testing.T) {
	u := baseUser(1, "@42", json.RawMessage(`{}`))
	h := ParseSettings(u.Settings)
	if !IsLegacyTelegramCandidate(u, h) {
		t.Fatal("expected legacy via @digits login")
	}
}
