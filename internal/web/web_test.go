package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/drift"
	"github.com/Raindancer118/nak-api/internal/tools"
)

const token = "test-token-0123456789"

type counters struct{ reads, previews, dos, downloads atomic.Int32 }

func fakeRegistry(c *counters) *tools.Registry {
	r := tools.NewRegistry()
	r.Add(
		&tools.Tool{Name: "fake_read", Kind: tools.Read, Desc: "Liest etwas.",
			Params: []tools.Param{{Name: "q", Desc: "Suchwort"}},
			Run: func(a *app.App, args tools.Args) (any, error) {
				n := c.reads.Add(1)
				return map[string]any{"q": args.Str("q"), "call": n}, nil
			}},
		&tools.Tool{Name: "fake_write", Kind: tools.Write, Desc: "Ändert etwas.",
			Preview: func(a *app.App, args tools.Args) (any, error) { c.previews.Add(1); return "would do", nil },
			Do:      func(a *app.App, args tools.Args) (any, error) { c.dos.Add(1); return "done", nil }},
		&tools.Tool{Name: "fake_download", Kind: tools.Local, Desc: "Lädt herunter.",
			Params: []tools.Param{{Name: "out", Desc: "Zielpfad"}},
			Run: func(a *app.App, args tools.Args) (any, error) {
				if args.Has("out") {
					return nil, errors.New("out must not reach the tool from the web")
				}
				c.downloads.Add(1)
				p := filepath.Join(a.DownloadDir, "sub", "Skript 1.pdf")
				os.MkdirAll(filepath.Dir(p), 0o755)
				os.WriteFile(p, []byte("%PDF-1.4 skript"), 0o600)
				return map[string]any{"path": p, "bytes": 15}, nil
			}},
		&tools.Tool{Name: "nak_agenda", Kind: tools.Read, Desc: "Kalender.",
			Params: []tools.Param{{Name: "days", Type: "integer"}},
			Run: func(a *app.App, args tools.Args) (any, error) {
				return map[string]any{"days": []map[string]any{{"date": "Fr 02.10.2026", "events": []map[string]any{
					{"date": "Fr 02.10.2026", "start": "09:00", "end": "12:15", "kind": "Vorlesung", "module_nr": "I151", "title": "Softwaretechnik; Teil 2, Übung", "room": "A101", "lecturer": "Muster", "source": "cis_stundenplan"},
					{"date": "Fr 02.10.2026", "start": "11:30", "end": "13:00", "kind": "Klausur", "module_nr": "A222,I222", "title": "Diskrete Mathematik 2", "source": "cis_pruefung"},
					{"date": "Fr 02.10.2026", "start": "23:59", "end": "23:59", "kind": "Moodle-Frist", "title": "Übungsblatt 3 (I160) — eine sehr lange Beschreibung, die über die fünfundsiebzig Zeichen einer ICS-Zeile hinausgeht", "source": "moodle"},
				}}}}, nil
			}},
		&tools.Tool{Name: "fake_drift", Kind: tools.Read, Desc: "Kaputt.",
			Run: func(a *app.App, args tools.Args) (any, error) {
				return nil, drift.New("/studium/x", "a grades table", "<html><body><main><p>x</p></main></body></html>")
			}},
	)
	return r
}

type harness struct {
	web *Server
	srv *httptest.Server
	c   *counters
	app *app.App
	now *time.Time
}

func newHarness(t *testing.T) *harness { return newHarnessWith(t, "") }

func newHarnessWith(t *testing.T, cacheFile string) *harness {
	t.Helper()
	c := &counters{}
	a := app.FromEnv()
	a.DownloadDir = t.TempDir()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	h := &harness{c: c, app: a, now: &now}
	s := New(a, fakeRegistry(c), Config{Token: token, CacheTTL: 5 * time.Minute, Version: "9.9.9", Now: func() time.Time { return *h.now }, CacheFile: cacheFile})
	h.web = s
	h.srv = httptest.NewServer(s.Handler())
	t.Cleanup(h.srv.Close)
	return h
}

func TestCacheSurvivesRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "web-cache.json")
	h1 := newHarnessWith(t, file)
	h1.do(t, "POST", "/api/tools/fake_read", `{"q":"x"}`, bearer)
	if err := h1.web.SaveCache(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(file)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("cache file %v %v", st, err)
	}
	h2 := newHarnessWith(t, file)
	m := decode(t, h2.do(t, "POST", "/api/tools/fake_read", `{"q":"x"}`, bearer))
	if m["cached"] != true || h2.c.reads.Load() != 0 {
		t.Fatalf("after restart: %v reads=%d", m, h2.c.reads.Load())
	}
}

func TestConfirmedWriteDropsCache(t *testing.T) {
	h := newHarness(t)
	h.do(t, "POST", "/api/tools/fake_read", `{}`, bearer)
	h.do(t, "POST", "/api/tools/fake_write", `{"confirm":true}`, with(map[string]string{confirmHeader: "JA"}))
	m := decode(t, h.do(t, "POST", "/api/tools/fake_read", `{}`, bearer))
	if m["cached"] != false || h.c.reads.Load() != 2 {
		t.Fatalf("read after write: %v reads=%d", m, h.c.reads.Load())
	}
}

func (h *harness) do(t *testing.T, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (h *harness) login(t *testing.T, tok string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", h.srv.URL+"/login", strings.NewReader(url.Values{"token": {tok}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

var bearer = map[string]string{"Authorization": "Bearer " + token}

func with(extra map[string]string) map[string]string {
	out := map[string]string{"Authorization": "Bearer " + token}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func decode(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	b, _ := io.ReadAll(res.Body)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON (%d): %s", res.StatusCode, b)
	}
	return m
}

func TestHealthzNeedsNoAuth(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, "GET", "/healthz", "", nil)
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if m := decode(t, res); m["status"] != "ok" || m["version"] != "9.9.9" {
		t.Fatalf("body %v", m)
	}
}

func TestAPIRequiresAuth(t *testing.T) {
	h := newHarness(t)
	for _, p := range []string{"/api/tools", "/files/x"} {
		if res := h.do(t, "GET", p, "", nil); res.StatusCode != 401 {
			t.Errorf("%s without auth: %d", p, res.StatusCode)
		}
	}
	if res := h.do(t, "POST", "/api/tools/fake_read", `{}`, nil); res.StatusCode != 401 {
		t.Errorf("tool call without auth: %d", res.StatusCode)
	}
	if res := h.do(t, "GET", "/api/tools", "", map[string]string{"Authorization": "Bearer wrong"}); res.StatusCode != 401 {
		t.Errorf("wrong token: %d", res.StatusCode)
	}
	if h.c.reads.Load() != 0 {
		t.Fatal("tool ran without auth")
	}
	// The UI itself redirects to the login page instead of a bare 401.
	res := h.do(t, "GET", "/", "", nil)
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Fatalf("index without auth: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestToolList(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, "GET", "/api/tools", "", bearer)
	m := decode(t, res)
	list, _ := m["tools"].([]any)
	if len(list) != 5 {
		t.Fatalf("tools %v", m)
	}
	kinds := map[string]string{}
	for _, x := range list {
		e := x.(map[string]any)
		kinds[e["name"].(string)] = e["kind"].(string)
	}
	if kinds["fake_read"] != "read" || kinds["fake_write"] != "write" || kinds["fake_download"] != "local" {
		t.Fatalf("kinds %v", kinds)
	}
	if m["read_only"] != false {
		t.Fatalf("read_only flag missing: %v", m)
	}
}

func TestLoginSetsStrictCookie(t *testing.T) {
	h := newHarness(t)
	form := func(tok string) *http.Response { return h.login(t, tok) }
	if res := form("nope"); res.StatusCode != 401 || len(res.Cookies()) != 0 {
		t.Fatalf("wrong token: %d %v", res.StatusCode, res.Cookies())
	}
	res := form(token)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/" {
		t.Fatalf("login: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	var sess *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == cookieName {
			sess = c
		}
	}
	if sess == nil || !sess.HttpOnly || sess.SameSite != http.SameSiteStrictMode || sess.Value == token {
		t.Fatalf("cookie %+v", sess)
	}
	// the cookie alone authorises the API
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(h.srv.URL)
	jar.SetCookies(u, []*http.Cookie{sess})
	cl := &http.Client{Jar: jar}
	r2, err := cl.Get(h.srv.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatalf("cookie auth: %d", r2.StatusCode)
	}
}

func TestTokenNeverTravelsInTheQuery(t *testing.T) {
	h := newHarness(t)
	// The log link uses a #fragment, which browsers never send; a token in the
	// query string (history, proxy logs) is not accepted.
	res := h.do(t, "GET", "/login?token="+token, "", nil)
	if len(res.Cookies()) != 0 {
		t.Fatal("query token logged in")
	}
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(b), `name="token"`) || !strings.Contains(string(b), "/assets/login.js") {
		t.Fatalf("login form: %d %s", res.StatusCode, b)
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newHarness(t)
	var last int
	for i := 0; i < maxLoginFailures+1; i++ {
		last = h.login(t, "wrong").StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after %d failures: %d", maxLoginFailures+1, last)
	}
	// even the right token is refused while locked
	if res := h.login(t, token); res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("locked: %d", res.StatusCode)
	}
	*h.now = h.now.Add(loginWindow + time.Second)
	if res := h.login(t, token); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("after window: %d", res.StatusCode)
	}
}

func TestReadToolIsCached(t *testing.T) {
	h := newHarness(t)
	call := func(path, body string) map[string]any {
		res := h.do(t, "POST", path, body, bearer)
		if res.StatusCode != 200 {
			t.Fatalf("%s: %d", path, res.StatusCode)
		}
		return decode(t, res)
	}
	a := call("/api/tools/fake_read", `{"q":"x"}`)
	b := call("/api/tools/fake_read", `{"q":"x"}`)
	if h.c.reads.Load() != 1 || a["cached"] != false || b["cached"] != true {
		t.Fatalf("reads=%d a=%v b=%v", h.c.reads.Load(), a, b)
	}
	call("/api/tools/fake_read", `{"q":"y"}`) // other args, other entry
	call("/api/tools/fake_read?fresh=1", `{"q":"x"}`)
	if h.c.reads.Load() != 3 {
		t.Fatalf("reads=%d", h.c.reads.Load())
	}
	// Stale data is answered at once and refreshed in the background;
	// ?wait=1 returns the refreshed result.
	*h.now = h.now.Add(6 * time.Minute)
	st := call("/api/tools/fake_read", `{"q":"x"}`)
	if st["stale"] != true || st["cached"] != true {
		t.Fatalf("stale answer: %v", st)
	}
	fr := call("/api/tools/fake_read?wait=1", `{"q":"x"}`)
	if fr["stale"] == true || h.c.reads.Load() != 4 {
		t.Fatalf("revalidated answer: %v reads=%d", fr, h.c.reads.Load())
	}
	if r := call("/api/tools/fake_read", `{"q":"x"}`)["result"].(map[string]any); r["q"] != "x" {
		t.Fatalf("result %v", r)
	}
}

func TestWriteNeedsPreviewThenExplicitConfirm(t *testing.T) {
	h := newHarness(t)
	m := decode(t, h.do(t, "POST", "/api/tools/fake_write", `{}`, bearer))
	if res := m["result"].(map[string]any); res["mode"] != "preview" {
		t.Fatalf("preview %v", m)
	}
	// confirm in the body alone is not enough
	res := h.do(t, "POST", "/api/tools/fake_write", `{"confirm":true}`, bearer)
	if res.StatusCode != http.StatusPreconditionRequired || h.c.dos.Load() != 0 {
		t.Fatalf("confirm without header: %d dos=%d", res.StatusCode, h.c.dos.Load())
	}
	res = h.do(t, "POST", "/api/tools/fake_write", `{"confirm":true}`, with(map[string]string{confirmHeader: "JA"}))
	if res.StatusCode != 200 || h.c.dos.Load() != 1 {
		t.Fatalf("confirmed: %d dos=%d", res.StatusCode, h.c.dos.Load())
	}
	// writes are never cached
	h.do(t, "POST", "/api/tools/fake_write", `{"confirm":true}`, with(map[string]string{confirmHeader: "JA"}))
	if h.c.dos.Load() != 2 {
		t.Fatalf("write was cached: dos=%d", h.c.dos.Load())
	}
}

func TestCSRFProtection(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, "POST", "/api/tools/fake_read", "", with(map[string]string{"Content-Type": "text/plain"}))
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain: %d", res.StatusCode)
	}
	res = h.do(t, "POST", "/api/tools/fake_read", `{}`, with(map[string]string{"Origin": "https://evil.example"}))
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin: %d", res.StatusCode)
	}
	if h.c.reads.Load() != 0 {
		t.Fatal("tool ran on a rejected request")
	}
	res = h.do(t, "POST", "/api/tools/fake_read", `{}`, with(map[string]string{"Origin": h.srv.URL}))
	if res.StatusCode != 200 {
		t.Fatalf("own origin: %d", res.StatusCode)
	}
}

// A browser form post from the login page itself must pass the CSRF check.
func TestBrowserLoginPostIsSameOrigin(t *testing.T) {
	h := newHarness(t)
	post := func(hdr map[string]string) int {
		req, _ := http.NewRequest("POST", h.srv.URL+"/login", strings.NewReader(url.Values{"token": {token}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res, err := noRedirect().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if c := post(map[string]string{"Origin": "null", "Sec-Fetch-Site": "same-origin"}); c != http.StatusSeeOther { // what Chrome sends
		t.Fatalf("same-origin form post: %d", c)
	}
	if c := post(map[string]string{"Origin": "null", "Sec-Fetch-Site": "cross-site"}); c != http.StatusForbidden {
		t.Fatalf("cross-site form post: %d", c)
	}
	if c := post(map[string]string{"Origin": "https://evil.example"}); c != http.StatusForbidden {
		t.Fatalf("foreign origin without fetch metadata: %d", c)
	}
}

func TestErrors(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, "POST", "/api/tools/nope", `{}`, bearer); res.StatusCode != 404 {
		t.Fatalf("unknown tool: %d", res.StatusCode)
	}
	if res := h.do(t, "POST", "/api/tools/fake_read", `{not json`, bearer); res.StatusCode != 400 {
		t.Fatalf("bad json: %d", res.StatusCode)
	}
	res := h.do(t, "POST", "/api/tools/fake_drift", `{}`, bearer)
	m := decode(t, res)
	if res.StatusCode != http.StatusBadGateway || m["drift"] != true || !strings.Contains(m["error"].(string), "grades table") {
		t.Fatalf("drift: %d %v", res.StatusCode, m)
	}
	if u, _ := m["report_url"].(string); !strings.HasPrefix(u, "https://github.com/") {
		t.Fatalf("report_url %v", m)
	}
}

func TestDownloadedFilesAreServed(t *testing.T) {
	h := newHarness(t)
	m := decode(t, h.do(t, "POST", "/api/tools/fake_download", `{"out":"/etc/evil"}`, bearer))
	u, _ := m["download_url"].(string)
	if u != "/files/sub/Skript%201.pdf" {
		t.Fatalf("download_url %q (%v)", u, m)
	}
	res := h.do(t, "GET", u, "", bearer)
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(b) != "%PDF-1.4 skript" {
		t.Fatalf("file: %d %q", res.StatusCode, b)
	}
	if !strings.Contains(res.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition %q", res.Header.Get("Content-Disposition"))
	}
	secret := filepath.Join(filepath.Dir(h.app.DownloadDir), "secret.txt")
	os.WriteFile(secret, []byte("x"), 0o600)
	for _, p := range []string{"/files/../secret.txt", "/files/%2e%2e/secret.txt", "/files/sub", "/files/missing.pdf"} {
		if res := h.do(t, "GET", p, "", bearer); res.StatusCode == 200 {
			t.Errorf("%s served", p)
		}
	}
}

func TestFilesInlineOnlyForSafeTypes(t *testing.T) {
	h := newHarness(t)
	os.WriteFile(filepath.Join(h.app.DownloadDir, "a.pdf"), []byte("%PDF-1.4"), 0o600)
	os.WriteFile(filepath.Join(h.app.DownloadDir, "evil.html"), []byte("<script>alert(1)</script>"), 0o600)
	res := h.do(t, "GET", "/files/a.pdf?inline=1", "", bearer)
	if d := res.Header.Get("Content-Disposition"); !strings.HasPrefix(d, "inline") {
		t.Fatalf("pdf inline: %q", d)
	}
	os.WriteFile(filepath.Join(h.app.DownloadDir, "b.png"), []byte("png"), 0o600)
	res = h.do(t, "GET", "/files/b.png?inline=1", "", bearer)
	if !strings.HasPrefix(res.Header.Get("Content-Disposition"), "inline") || !strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("image inline without sandbox: %q %q", res.Header.Get("Content-Disposition"), res.Header.Get("Content-Security-Policy"))
	}
	res = h.do(t, "GET", "/files/evil.html?inline=1", "", bearer)
	if d := res.Header.Get("Content-Disposition"); !strings.HasPrefix(d, "attachment") {
		t.Fatalf("html must never be inline: %q", d)
	}
}

func TestSameDownloadIsReused(t *testing.T) {
	h := newHarness(t)
	a := decode(t, h.do(t, "POST", "/api/tools/fake_download", `{}`, bearer))
	b := decode(t, h.do(t, "POST", "/api/tools/fake_download", `{}`, bearer))
	if h.c.downloads.Load() != 1 || a["download_url"] != b["download_url"] {
		t.Fatalf("downloads=%d a=%v b=%v", h.c.downloads.Load(), a["download_url"], b["download_url"])
	}
	// a file deleted from disk is fetched again
	os.RemoveAll(filepath.Join(h.app.DownloadDir, "sub"))
	h.do(t, "POST", "/api/tools/fake_download", `{}`, bearer)
	if h.c.downloads.Load() != 2 {
		t.Fatalf("missing file not refetched: %d", h.c.downloads.Load())
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, "GET", "/healthz", "", nil)
	for k, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "same-origin",
	} {
		if !strings.Contains(res.Header.Get(k), want) {
			t.Errorf("%s = %q", k, res.Header.Get(k))
		}
	}
	if res.Header.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS over plain http")
	}
	res = h.do(t, "GET", "/healthz", "", map[string]string{"X-Forwarded-Proto": "https"})
	if res.Header.Get("Strict-Transport-Security") == "" {
		t.Error("no HSTS behind a TLS proxy")
	}
}

func TestUIIsServed(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, "GET", "/assets/app.css", "", nil); res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("asset: %d %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	res := h.do(t, "GET", "/", "", bearer)
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(b), "<html") {
		t.Fatalf("index: %d", res.StatusCode)
	}
}

func TestTokenIsCreatedOnceAndPrivate(t *testing.T) {
	dir := t.TempDir()
	t1, created, err := LoadOrCreateToken(dir)
	if err != nil || !created || len(t1) < 32 {
		t.Fatalf("token %q created=%v err=%v", t1, created, err)
	}
	st, _ := os.Stat(filepath.Join(dir, tokenFile))
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	t2, created, _ := LoadOrCreateToken(dir)
	if t2 != t1 || created {
		t.Fatal("token changed on restart")
	}
}

func fakeEduVaultLogin(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Token string }
		json.NewDecoder(r.Body).Decode(&in)
		if in.Token != "evm_good_secretpart" {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":"Ungültige Zugangsdaten"}`)
			return
		}
		io.WriteString(w, `{"session":"s","expires_in":7200}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEduVaultSettings(t *testing.T) {
	t.Setenv("EDUVAULT_TOKEN", "")
	t.Setenv("EDUVAULT_MCP_SECRET", "")
	h := newHarness(t)
	h.app.ConfigDir = t.TempDir()
	ev := fakeEduVaultLogin(t)

	m := decode(t, h.do(t, "GET", "/api/settings", "", bearer))
	if e := m["eduvault"].(map[string]any); e["configured"] != false {
		t.Fatalf("initial: %v", m)
	}
	// wrong credentials are rejected and not stored
	res := h.do(t, "PUT", "/api/settings/eduvault", `{"url":"`+ev.URL+`","token":"evm_bad_x","secret":"JBSWY3DPEHPK3PXP"}`, bearer)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad credentials: %d", res.StatusCode)
	}
	if h.app.EduVaultSource() != "" {
		t.Fatal("bad credentials were saved")
	}
	res = h.do(t, "PUT", "/api/settings/eduvault", `{"url":"`+ev.URL+`","token":"evm_good_secretpart","secret":"JBSWY3DPEHPK3PXP"}`, bearer)
	if res.StatusCode != 200 {
		t.Fatalf("good credentials: %d", res.StatusCode)
	}
	res = h.do(t, "GET", "/api/settings", "", bearer)
	b, _ := io.ReadAll(res.Body)
	if strings.Contains(string(b), "secretpart") || strings.Contains(string(b), "JBSWY3DPEHPK3PXP") {
		t.Fatalf("settings leak secrets: %s", b)
	}
	if !strings.Contains(string(b), `"configured":true`) || !strings.Contains(string(b), "evm_good") {
		t.Fatalf("settings: %s", b)
	}
	if res := h.do(t, "DELETE", "/api/settings/eduvault", "", bearer); res.StatusCode != 200 || h.app.EduVaultSource() != "" {
		t.Fatalf("delete: %d %s", res.StatusCode, h.app.EduVaultSource())
	}
	// settings need auth and same-origin like everything else
	if res := h.do(t, "PUT", "/api/settings/eduvault", `{}`, nil); res.StatusCode != 401 {
		t.Fatalf("no auth: %d", res.StatusCode)
	}
	if res := h.do(t, "DELETE", "/api/settings/eduvault", "", with(map[string]string{"Origin": "https://evil.example"})); res.StatusCode != 403 {
		t.Fatalf("cross-site delete: %d", res.StatusCode)
	}
}

func TestCacheClearEndpoint(t *testing.T) {
	h := newHarness(t)
	h.do(t, "POST", "/api/tools/fake_read", `{}`, bearer)
	if res := h.do(t, "POST", "/api/cache/clear", `{}`, bearer); res.StatusCode != 200 {
		t.Fatalf("clear: %d", res.StatusCode)
	}
	h.do(t, "POST", "/api/tools/fake_read", `{}`, bearer)
	if h.c.reads.Load() != 2 {
		t.Fatalf("cache not cleared: reads=%d", h.c.reads.Load())
	}
}

// fakeCISLogin is a minimal TYPO3 felogin: one account 12345 / right.
func fakeCISLogin(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var posts atomic.Int32
	form := `<html><body><form action="/" method="post"><input name="user"><input type="password" name="pass"><input type="hidden" name="logintype" value="login"><button type="submit">Anmelden</button></form></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			r.ParseForm()
			if r.PostForm.Get("user") == "12345" && r.PostForm.Get("pass") == "right" {
				http.SetCookie(w, &http.Cookie{Name: "fe_typo_user_cae070b", Value: "sess", Path: "/"})
				io.WriteString(w, `<main>Willkommen</main><a href="/login/?logintype=logout">Abmelden</a>`)
				return
			}
			w.WriteHeader(http.StatusForbidden)
		}
		io.WriteString(w, form)
	}))
	t.Cleanup(srv.Close)
	return srv, &posts
}

func accountHarness(t *testing.T) (*harness, *atomic.Int32) {
	cis, posts := fakeCISLogin(t)
	t.Setenv("CIS_BASE_URL", cis.URL)
	t.Setenv("CIS_USER", "")
	t.Setenv("CIS_PASS", "")
	t.Setenv("NAK_DATA_DIR", t.TempDir())
	return newHarness(t), posts
}

func (h *harness) accountLogin(t *testing.T, user, pass string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", h.srv.URL+"/login", strings.NewReader(url.Values{"username": {user}, "password": {pass}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestFirstLoginClaimsTheInstance(t *testing.T) {
	h, posts := accountHarness(t)
	res := h.do(t, "GET", "/login", "", nil)
	b, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(b), `name="username"`) || !strings.Contains(string(b), `name="password"`) {
		t.Fatalf("login form without account fields: %s", b)
	}
	if res := h.accountLogin(t, "12345", "wrong"); res.StatusCode != 401 || h.app.AccountUser() != "" {
		t.Fatalf("wrong password: %d owner=%q", res.StatusCode, h.app.AccountUser())
	}
	res = h.accountLogin(t, "12345", "right")
	if res.StatusCode != http.StatusSeeOther || len(res.Cookies()) == 0 {
		t.Fatalf("first login: %d", res.StatusCode)
	}
	if h.app.AccountUser() != "12345" || h.app.AccountSource() != "file" {
		t.Fatalf("owner = %q (%s)", h.app.AccountUser(), h.app.AccountSource())
	}
	// later logins are checked locally — no extra CIS round trip
	before := posts.Load()
	if res := h.accountLogin(t, "12345", "right"); res.StatusCode != http.StatusSeeOther || posts.Load() != before {
		t.Fatalf("second login: %d, CIS posts %d→%d", res.StatusCode, before, posts.Load())
	}
	// another NAK account cannot take over, even with valid credentials
	if res := h.accountLogin(t, "99999", "right"); res.StatusCode != 401 {
		t.Fatalf("other account: %d", res.StatusCode)
	}
	b2, _ := io.ReadAll(h.accountLogin(t, "99999", "x").Body)
	if !strings.Contains(string(b2), "anderen") {
		t.Fatalf("other account message: %s", b2)
	}
}

func TestEmptyCredentialsAndRateLimit(t *testing.T) {
	h, _ := accountHarness(t)
	if res := h.accountLogin(t, "", ""); res.StatusCode != 400 {
		t.Fatalf("empty: %d", res.StatusCode)
	}
	var last int
	for i := 0; i < maxLoginFailures+1; i++ {
		last = h.accountLogin(t, "12345", "wrong").StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", last)
	}
}

func TestSettingsShowAccountButNoPassword(t *testing.T) {
	h, _ := accountHarness(t)
	h.accountLogin(t, "12345", "right")
	res := h.do(t, "GET", "/api/settings", "", bearer)
	b, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(b), `"user":"12345"`) || strings.Contains(string(b), "right") {
		t.Fatalf("settings: %s", b)
	}
}

func TestPWAFiles(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, "GET", "/sw.js", "", nil)
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), "javascript") || res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("sw.js: %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Cache-Control"))
	}
	res = h.do(t, "GET", "/manifest.webmanifest", "", nil)
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), "manifest+json") || !strings.Contains(string(b), `"naknak"`) {
		t.Fatalf("manifest: %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
}
