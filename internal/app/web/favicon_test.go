package web

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func accountIndexHTMLPath(t *testing.T) string {
	t.Helper()
	_, fname, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(fname), "static", "account", "index.html")
}

func TestAccountIndexStatic_LoggedOutUX(t *testing.T) {
	ru := mustRenderAccountLoginHTML(t, orderStartTestCfg(), accountLocaleRU)
	if !strings.Contains(ru, `logged-out-msg`) {
		t.Fatal("logged-out-msg id missing")
	}
	if !strings.Contains(ru, `Вы вышли из личного кабинета.`) {
		t.Fatal("logged-out copy missing")
	}
	if !strings.Contains(ru, `params.get('logged_out') === '1'`) {
		t.Fatal("logged_out query gate missing")
	}
	if !strings.Contains(ru, `window.history.replaceState({}, document.title, "/account")`) {
		t.Fatal("RU logged_out strip query via replaceState missing")
	}
	en := mustRenderAccountLoginHTML(t, orderStartTestCfg(), accountLocaleEN)
	if !strings.Contains(en, `window.history.replaceState({}, document.title, "/account?lang=en")`) {
		t.Fatal("EN logged_out must preserve lang=en in replaceState")
	}
}

func TestAccountIndexStaticHasFaviconLinks(t *testing.T) {
	b, err := os.ReadFile(accountIndexHTMLPath(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, line := range []string{
		`<link rel="icon" href="/favicon.ico?v=2" sizes="any">`,
		`<link rel="icon" type="image/png" href="/favicon-32x32.png?v=2">`,
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png?v=2">`,
	} {
		if !strings.Contains(s, line) {
			t.Fatalf("account index.html missing %q", line)
		}
	}
}

func TestAccountSessionStaticHasFaviconLinks(t *testing.T) {
	b, err := os.ReadFile(sessionHTMLPath(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, line := range []string{
		`<link rel="icon" href="/favicon.ico?v=2" sizes="any">`,
		`<link rel="icon" type="image/png" href="/favicon-32x32.png?v=2">`,
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png?v=2">`,
	} {
		if !strings.Contains(s, line) {
			t.Fatalf("session.html missing %q", line)
		}
	}
}

func TestFaviconAssetsForBrand_Matrix(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		want faviconAssetSet
	}{
		{
			name: "vff",
			id:   "vff",
			want: faviconAssetSet{ICO: vffFaviconICO, PNG32: vffFavicon32PNG, AppleTouch: vffAppleTouchIconPNG},
		},
		{
			name: "fc",
			id:   "fc",
			want: faviconAssetSet{ICO: fcFaviconICO, PNG32: fcFavicon32PNG, AppleTouch: fcAppleTouchIconPNG},
		},
		{
			name: "fc_trimmed",
			id:   " fc ",
			want: faviconAssetSet{ICO: fcFaviconICO, PNG32: fcFavicon32PNG, AppleTouch: fcAppleTouchIconPNG},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Brand.ID = tc.id
			got, err := faviconAssetsForBrand(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.ICO) == 0 || len(got.PNG32) == 0 || len(got.AppleTouch) == 0 {
				t.Fatal("asset slices must be non-empty")
			}
			if !bytes.Equal(got.ICO, tc.want.ICO) ||
				!bytes.Equal(got.PNG32, tc.want.PNG32) ||
				!bytes.Equal(got.AppleTouch, tc.want.AppleTouch) {
				t.Fatal("resolver returned unexpected asset set")
			}
		})
	}

	if bytes.Equal(vffFaviconICO, fcFaviconICO) ||
		bytes.Equal(vffFavicon32PNG, fcFavicon32PNG) ||
		bytes.Equal(vffAppleTouchIconPNG, fcAppleTouchIconPNG) {
		t.Fatal("VFF and FC favicon bytes must differ for each asset type")
	}
}

func TestFaviconAssetsForBrand_FailClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil", cfg: nil},
		{name: "empty", cfg: &config.Config{}},
		{name: "whitespace", cfg: func() *config.Config {
			c := &config.Config{}
			c.Brand.ID = "   "
			return c
		}()},
		{name: "unknown", cfg: func() *config.Config {
			c := &config.Config{}
			c.Brand.ID = "other"
			return c
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := faviconAssetsForBrand(tc.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.name == "unknown" {
				if !strings.Contains(err.Error(), `unsupported favicon brand id "other"`) {
					t.Fatalf("err=%v", err)
				}
			} else if !strings.Contains(err.Error(), "favicon brand id is required") {
				t.Fatalf("err=%v", err)
			}

			for _, h := range []http.HandlerFunc{
				serveBrandFaviconICO(tc.cfg),
				serveBrandFavicon32PNG(tc.cfg),
				serveBrandAppleTouchIcon(tc.cfg),
			} {
				rec := httptest.NewRecorder()
				h(rec, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
				if rec.Code != http.StatusInternalServerError {
					t.Fatalf("code=%d", rec.Code)
				}
				body := rec.Body.Bytes()
				if bytes.Contains(body, vffFaviconICO) ||
					bytes.Contains(body, vffFavicon32PNG) ||
					bytes.Contains(body, vffAppleTouchIconPNG) {
					t.Fatal("500 must not leak VFF asset bytes")
				}
			}
		})
	}
}

func TestBrandFaviconHTTP_Matrix(t *testing.T) {
	type routeCase struct {
		path        string
		contentType string
		handler     func(*config.Config) http.HandlerFunc
		pick        func(faviconAssetSet) []byte
	}
	routes := []routeCase{
		{
			path: "/favicon.ico", contentType: "image/x-icon",
			handler: serveBrandFaviconICO,
			pick:    func(s faviconAssetSet) []byte { return s.ICO },
		},
		{
			path: "/favicon-32x32.png", contentType: "image/png",
			handler: serveBrandFavicon32PNG,
			pick:    func(s faviconAssetSet) []byte { return s.PNG32 },
		},
		{
			path: "/apple-touch-icon.png", contentType: "image/png",
			handler: serveBrandAppleTouchIcon,
			pick:    func(s faviconAssetSet) []byte { return s.AppleTouch },
		},
	}

	for _, brand := range []struct {
		name string
		id   string
	}{
		{name: "vff", id: "vff"},
		{name: "fc", id: "fc"},
	} {
		t.Run(brand.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Brand.ID = brand.id
			want, err := faviconAssetsForBrand(cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, rc := range routes {
				t.Run(rc.path, func(t *testing.T) {
					h := rc.handler(cfg)
					rec := httptest.NewRecorder()
					h(rec, httptest.NewRequest(http.MethodGet, rc.path, nil))
					if rec.Code != http.StatusOK {
						t.Fatalf("code=%d", rec.Code)
					}
					body := rec.Body.Bytes()
					expected := rc.pick(want)
					if !bytes.Equal(body, expected) {
						t.Fatalf("body mismatch len=%d want=%d", len(body), len(expected))
					}
					if ct := rec.Header().Get("Content-Type"); ct != rc.contentType {
						t.Fatalf("Content-Type=%q", ct)
					}
					if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=604800" {
						t.Fatalf("Cache-Control=%q", cc)
					}
					if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(len(expected)) {
						t.Fatalf("Content-Length=%q", cl)
					}

					rec = httptest.NewRecorder()
					h(rec, httptest.NewRequest(http.MethodPost, rc.path, nil))
					if rec.Code != http.StatusMethodNotAllowed {
						t.Fatalf("POST code=%d", rec.Code)
					}
					if got := rec.Header().Get("Allow"); got != "GET" {
						t.Fatalf("Allow=%q", got)
					}
				})
			}
		})
	}
}

func TestFCFaviconAssetIntegrity(t *testing.T) {
	cfg32, _, err := image.DecodeConfig(bytes.NewReader(fcFavicon32PNG))
	if err != nil {
		t.Fatal(err)
	}
	if cfg32.Width != 32 || cfg32.Height != 32 {
		t.Fatalf("favicon-32x32.png size=%dx%d", cfg32.Width, cfg32.Height)
	}

	cfgApple, _, err := image.DecodeConfig(bytes.NewReader(fcAppleTouchIconPNG))
	if err != nil {
		t.Fatal(err)
	}
	if cfgApple.Width != 180 || cfgApple.Height != 180 {
		t.Fatalf("apple-touch-icon.png size=%dx%d", cfgApple.Width, cfgApple.Height)
	}

	if len(fcFaviconICO) < 6 {
		t.Fatal("ICO too short")
	}
	if fcFaviconICO[0] != 0 || fcFaviconICO[1] != 0 || fcFaviconICO[2] != 1 || fcFaviconICO[3] != 0 {
		t.Fatalf("ICO header %#v", fcFaviconICO[:4])
	}
	count := binary.LittleEndian.Uint16(fcFaviconICO[4:6])
	if count != 3 {
		t.Fatalf("ICO directory count=%d want 3", count)
	}

	svgPath := filepath.Join(filepath.Dir(accountIndexHTMLPath(t)), "..", "brands", "fc", "favicon.svg")
	svg, err := os.ReadFile(svgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(svg) == 0 {
		t.Fatal("svg empty")
	}
	s := string(svg)
	if !strings.Contains(s, `aria-label="Friends Connect"`) {
		t.Fatal("svg missing Friends Connect aria-label")
	}
	if !strings.Contains(s, `viewBox="0 0 64 64"`) {
		t.Fatal("svg missing viewBox")
	}
}
