package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTest(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewWith(Options{BaseURL: srv.URL, SessionDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestReadOnlyBlocksEveryWritePath(t *testing.T) {
	hits := 0
	c, srv := newTest(t, func(w http.ResponseWriter, r *http.Request) { hits++ })
	c.ReadOnly = true

	if _, err := c.WriteGet(srv.URL + "/x?a[action]=register"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("WriteGet err = %v", err)
	}
	if _, err := c.WritePost(srv.URL+"/x", url.Values{"a": {"1"}}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("WritePost err = %v", err)
	}
	if _, err := c.WriteMultipart(srv.URL+"/x", nil, nil); !errors.Is(err, ErrReadOnly) {
		t.Errorf("WriteMultipart err = %v", err)
	}
	if hits != 0 {
		t.Fatalf("server was hit %d times in read-only mode", hits)
	}
}

func TestPageDetectsLoggedOutAndRelogsOnce(t *testing.T) {
	loggedIn := false
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if !loggedIn {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<form><input name="user"><input type="password" name="pass"></form>`)
			return
		}
		io.WriteString(w, "<main>ok</main>")
	})

	if _, err := c.Page("/a"); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("without relogin hook: err = %v", err)
	}

	calls := 0
	c.Relogin = func(*Client) error { calls++; loggedIn = true; return nil }
	p, err := c.Page("/a")
	if err != nil || !strings.Contains(p.Body, "ok") || calls != 1 {
		t.Fatalf("relogin: err=%v calls=%d body=%q", err, calls, p.Body)
	}
}

func TestWritesAreAuditLogged(t *testing.T) {
	c, srv := newTest(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<main>done</main>") })
	if _, err := c.WritePost(srv.URL+"/p?cHash=secret", url.Values{"f[iban]": {"DE00"}, "f[x]": {"1"}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(c.sessionDir, "writes.log"))
	if err != nil {
		t.Fatal(err)
	}
	log := string(b)
	if !strings.Contains(log, "POST") || !strings.Contains(log, "f[x]") {
		t.Errorf("audit log missing request: %q", log)
	}
	if strings.Contains(log, "DE00") || strings.Contains(log, "secret") {
		t.Errorf("audit log must not contain values or cHash: %q", log)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	c, srv := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "abc", Path: "/"})
	})
	if _, err := c.Page("/"); err != nil {
		t.Fatal(err)
	}
	if !c.IsLoggedIn() {
		t.Fatal("cookie not stored")
	}
	if err := c.SaveSession(); err != nil {
		t.Fatal(err)
	}
	c2, _ := NewWith(Options{BaseURL: srv.URL, SessionDir: c.sessionDir})
	if !c2.IsLoggedIn() {
		t.Fatal("session not restored")
	}
}

func TestForeignHostRejected(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, _, _, err := c.Download("https://evil.example/x"); err == nil {
		t.Fatal("download from foreign host must fail")
	}
}
