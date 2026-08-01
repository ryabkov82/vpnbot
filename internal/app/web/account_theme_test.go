package web

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func accountCSSPath(t *testing.T) string {
	t.Helper()
	_, fname, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(fname), "static", "account", "account.css")
}

func countStylesheetAccountCSS(html string) int {
	return strings.Count(html, `href="/account/assets/account.css"`)
}

func TestAccountTheme_BrandIDFromConfig(t *testing.T) {
	fc := friendsConnectAccountTestCfg()
	fc.Brand.Name = "Not Used For Brand Attr"
	fc.Brand.LandingURL = "https://landing-should-not-become-brand-id.example"
	login := mustRenderAccountLoginHTML(t, fc, accountLocaleRU)
	session := mustRenderAccountSessionHTML(t, fc, accountLocaleRU)
	for _, page := range []string{login, session} {
		if !strings.Contains(page, `data-brand="fc"`) {
			t.Fatal("fc pages must use cfg.BrandID()")
		}
		if strings.Contains(page, `data-brand="Not Used`) || strings.Contains(page, `data-brand="https://`) {
			t.Fatal("BrandID must not come from Brand.Name or LandingURL")
		}
	}

	vff := orderStartTestCfg()
	vff.Brand.Name = "Friends Connect Spoof"
	vffLogin := mustRenderAccountLoginHTML(t, vff, accountLocaleRU)
	vffSession := mustRenderAccountSessionHTML(t, vff, accountLocaleRU)
	for _, page := range []string{vffLogin, vffSession} {
		if !strings.Contains(page, `data-brand="vff"`) {
			t.Fatal("vff pages must use cfg.BrandID()")
		}
		if strings.Contains(page, `data-brand="fc"`) {
			t.Fatal("vff must not render fc brand attribute")
		}
	}
}

func TestAccountTheme_StylesheetAndNoLargeStyleBlocks(t *testing.T) {
	for _, brandCfg := range []*config.Config{orderStartTestCfg(), friendsConnectAccountTestCfg()} {
		login := mustRenderAccountLoginHTML(t, brandCfg, accountLocaleRU)
		session := mustRenderAccountSessionHTML(t, brandCfg, accountLocaleRU)
		for name, page := range map[string]string{"login": login, "session": session} {
			if n := countStylesheetAccountCSS(page); n != 1 {
				t.Fatalf("%s brand=%s: want exactly one account.css link, got %d", name, brandCfg.BrandID(), n)
			}
			if strings.Contains(page, "<style>") || strings.Contains(page, "<style ") {
				t.Fatalf("%s still has <style> block", name)
			}
		}
	}

	for _, name := range []string{"index.html", "session.html"} {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(accountCSSPath(t)), name))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, "<style>") || strings.Contains(s, "<style ") {
			t.Fatalf("%s template still contains <style>", name)
		}
		if !strings.Contains(s, `href="/account/assets/account.css"`) {
			t.Fatalf("%s missing account.css link", name)
		}
		if !strings.Contains(s, `data-brand="{{.BrandID}}"`) {
			t.Fatalf("%s missing data-brand BrandID template", name)
		}
	}
}

func TestAccountTheme_SessionHooksPreserved(t *testing.T) {
	html := mustRenderAccountSessionHTML(t, friendsConnectAccountTestCfg(), accountLocaleRU)
	for _, needle := range []string{
		`id="cabinet-tabs"`,
		`id="tab-services-tab"`,
		`id="tab-buy-tab"`,
		`id="tab-payments-tab"`,
		`id="tab-help-tab"`,
		`id="balance-wrap"`,
		`id="balance-num"`,
		`id="catalog-tariffs"`,
		`id="cards"`,
		`id="topupModal"`,
		`id="new-catalog-wrap"`,
		`{{.LangSwitchRU}}`,
		`/api/account/session/start`,
		`/api/account/services`,
		`/api/account/catalog/services`,
		`/api/account/payments`,
		`/api/account/service/order`,
		`/api/account/service/delete`,
		`/api/account/service/connect`,
		`/api/account/balance/topup`,
	} {
		src := html
		if strings.HasPrefix(needle, "{{.") {
			src = accountSessionPageTemplateSrc
		}
		if !strings.Contains(src, needle) {
			t.Fatalf("session missing preserved hook %q", needle)
		}
	}
	if !strings.Contains(accountSessionPageTemplateSrc, "{{.SupportLinkHTML}}") {
		t.Fatal("SupportLinkHTML placeholder missing")
	}
	if !strings.Contains(accountSessionPageTemplateSrc, "{{.LangSwitchEN}}") {
		t.Fatal("EN lang switch missing")
	}
	if strings.Contains(html, `style="background: var(--bs-secondary-bg)`) {
		t.Fatal("new-catalog-wrap must not keep inline background style")
	}
}

func TestAccountTheme_FCOverridesScoped(t *testing.T) {
	css := string(accountCSS)
	re := regexp.MustCompile(`html\[data-brand="fc"\]`)
	if !re.MatchString(css) {
		t.Fatal("FC theme selector missing")
	}
	// VFF must not hardcode the FC palette as the default surface colors.
	rootIdx := strings.Index(css, ":root {")
	fcIdx := strings.Index(css, `html[data-brand="fc"]`)
	if rootIdx < 0 || fcIdx < 0 || fcIdx < rootIdx {
		t.Fatal("expected :root defaults before FC overrides")
	}
	rootBlock := css[rootIdx:fcIdx]
	if strings.Contains(rootBlock, "#0b1020") || strings.Contains(rootBlock, "#5b8cff") {
		t.Fatal("VFF/default :root must not hardcode FC palette")
	}
	if !strings.Contains(rootBlock, "--account-bg: #282a36") {
		t.Fatal("default --account-bg should keep VFF color")
	}
	if !strings.Contains(css, "@media (max-width: 576px)") {
		t.Fatal("responsive rules missing")
	}
}

func TestAccountTheme_NoPerBrandTemplateCopies(t *testing.T) {
	dir := filepath.Dir(accountCSSPath(t))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
		if strings.Contains(e.Name(), "_fc.") || strings.Contains(e.Name(), "_vff.") {
			t.Fatalf("must not have brand-specific template copy %q", e.Name())
		}
	}
	for _, need := range []string{"account.css", "index.html", "session.html"} {
		found := false
		for _, n := range names {
			if n == need {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing required account file %q in %v", need, names)
		}
	}
}
