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

type counters struct{ reads, previews, dos atomic.Int32 }

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
				p := filepath.Join(a.DownloadDir, "sub", "Skript 1.pdf")
				os.MkdirAll(filepath.Dir(p), 0o755)
				os.WriteFile(p, []byte("%PDF-1.4 skript"), 0o600)
				return map[string]any{"path": p, "bytes": 15}, nil
			}},
		&tools.Tool{Name: "fake_drift", Kind: tools.Read, Desc: "Kaputt.",
			Run: func(a *app.App, args tools.Args) (any, error) {
				return nil, drift.New("/studium/x", "a grades table", "<html><body><main><p>x</p></main></body></html>")
			}},
	)
	return r
}

type harness struct {
	srv *httptest.Server
	c   *counters
	app *app.App
	now *time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	c := &counters{}
	a := app.FromEnv()
	a.DownloadDir = t.TempDir()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	h := &harness{c: c, app: a, now: &now}
	s := New(a, fakeRegistry(c), Config{Token: token, CacheTTL: 5 * time.Minute, Version: "9.9.9", Now: func() time.Time { return *h.now }})
	h.srv = httptest.NewServer(s.Handler())
	t.Cleanup(h.srv.Close)
	return h
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
	if len(list) != 4 {
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
	form := func(tok string) *http.Response {
		req, _ := http.NewRequest("POST", h.srv.URL+"/login", strings.NewReader(url.Values{"token": {tok}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := noRedirect().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
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

func TestLoginLinkFromLog(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, "GET", "/login?token="+token, "", nil)
	if res.StatusCode != http.StatusSeeOther || len(res.Cookies()) == 0 {
		t.Fatalf("login link: %d", res.StatusCode)
	}
	// without a token the login form is shown
	res = h.do(t, "GET", "/login", "", nil)
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(b), `name="token"`) {
		t.Fatalf("login form: %d %s", res.StatusCode, b)
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newHarness(t)
	var last int
	for i := 0; i < maxLoginFailures+1; i++ {
		last = h.do(t, "GET", "/login?token=wrong", "", nil).StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after %d failures: %d", maxLoginFailures+1, last)
	}
	// even the right token is refused while locked
	if res := h.do(t, "GET", "/login?token="+token, "", nil); res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("locked: %d", res.StatusCode)
	}
	*h.now = h.now.Add(loginWindow + time.Second)
	if res := h.do(t, "GET", "/login?token="+token, "", nil); res.StatusCode != http.StatusSeeOther {
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
	*h.now = h.now.Add(6 * time.Minute)
	call("/api/tools/fake_read", `{"q":"x"}`)
	if h.c.reads.Load() != 4 {
		t.Fatalf("expired entry not refetched: %d", h.c.reads.Load())
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

func TestSecurityHeaders(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, "GET", "/healthz", "", nil)
	for k, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
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
