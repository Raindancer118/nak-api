package web

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportContainsDataButNoSecrets(t *testing.T) {
	h, _ := accountHarness(t)
	h.accountLogin(t, "12345", "right")
	h.web.history.observe("cis_list_klausuren", []byte(`[{"exam_id":"1","module_nr":"I1","title":"X","start":"01.01.2027 09:00","registered":true}]`))
	h.do(t, "POST", "/api/tools/fake_read", `{}`, bearer)
	res := h.do(t, "GET", "/api/export", "", bearer)
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Disposition"), ".zip") {
		t.Fatalf("export: %d %q", res.StatusCode, res.Header.Get("Content-Disposition"))
	}
	b, _ := io.ReadAll(res.Body)
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		c, _ := io.ReadAll(rc)
		rc.Close()
		names[f.Name] = string(c)
	}
	for _, want := range []string{"README.txt", "account.json", "history.json", "cache.json"} {
		if _, ok := names[want]; !ok {
			t.Errorf("export lacks %s (has %v)", want, keysOf(names))
		}
	}
	all := strings.Join(mapValues(names), "\n")
	if strings.Contains(all, `"right"`) || strings.Contains(all, token) {
		t.Fatal("export contains the password or the access key")
	}
	if !strings.Contains(names["account.json"], "12345") {
		t.Fatalf("account export: %s", names["account.json"])
	}
}

func TestResetWipesTheInstance(t *testing.T) {
	h, _ := accountHarness(t)
	h.accountLogin(t, "12345", "right")
	dir := h.app.ConfigDir
	os.MkdirAll(filepath.Join(dir, "downloads"), 0o700)
	os.WriteFile(filepath.Join(dir, "downloads", "x.pdf"), []byte("x"), 0o600)
	if res := h.do(t, "POST", "/api/reset", `{"confirm":"ja"}`, bearer); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("reset without the word: %d", res.StatusCode)
	}
	if res := h.do(t, "POST", "/api/reset", `{"confirm":"LÖSCHEN"}`, bearer); res.StatusCode != 200 {
		t.Fatalf("reset: %d", res.StatusCode)
	}
	for _, f := range []string{"account.json", "downloads"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("%s survived the reset", f)
		}
	}
	if h.app.AccountUser() != "" {
		t.Fatalf("account still active: %q", h.app.AccountUser())
	}
	// the instance can be claimed again
	if res := h.accountLogin(t, "12345", "right"); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("login after reset: %d", res.StatusCode)
	}
}

func keysOf(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func mapValues(m map[string]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func TestLogoutEverywhereRotatesTheKey(t *testing.T) {
	h := newHarness(t)
	h.app.ConfigDir = t.TempDir()
	login := h.do(t, "GET", "/login", "", nil)
	login.Body.Close()
	res := h.login(t, token)
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == cookieName {
			cookie = c
		}
	}
	req, _ := http.NewRequest("POST", h.srv.URL+"/api/sessions/revoke", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	r2, err := http.DefaultClient.Do(req)
	if err != nil || r2.StatusCode != 200 {
		t.Fatalf("revoke: %v %v", r2, err)
	}
	r2.Body.Close()
	req, _ = http.NewRequest("GET", h.srv.URL+"/api/tools", nil)
	req.AddCookie(cookie)
	r3, _ := http.DefaultClient.Do(req)
	r3.Body.Close()
	if r3.StatusCode != 401 {
		t.Fatalf("old session still valid: %d", r3.StatusCode)
	}
	if res := h.do(t, "GET", "/api/tools", "", bearer); res.StatusCode != 401 {
		t.Fatalf("old key still valid: %d", res.StatusCode)
	}
}
