package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Raindancer118/nak-api/internal/client"
)

const loginPage = `<html><body><form method="post" action="/?tx_felogin_login%5Baction%5D=login&amp;cHash=1">
<input type="hidden" name="logintype" value="login" /><input type="hidden" name="pid" value="7" />
<input type="text" name="user" /><input type="password" name="pass" />
<input type="submit" name="submit" value="Anmelden" /></form></body></html>`

func TestLoginAndRelogin(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodPost {
			r.ParseForm()
			got = map[string]string{"user": r.PostForm.Get("user"), "pass": r.PostForm.Get("pass"), "pid": r.PostForm.Get("pid"), "submit": r.PostForm.Get("submit")}
			if r.PostForm.Get("pass") == "ok" {
				http.SetCookie(w, &http.Cookie{Name: client.SessionCookie, Value: "s1", Path: "/"})
				io.WriteString(w, `<main>Willkommen</main><a href="/login/?logintype=logout">Logout</a>`)
				return
			}
		}
		if c, err := r.Cookie(client.SessionCookie); err == nil && c.Value == "s1" {
			io.WriteString(w, `<main>Daten</main><a href="/login/?logintype=logout">Logout</a>`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, loginPage)
	}))
	defer srv.Close()
	c, _ := client.NewWith(client.Options{BaseURL: srv.URL, SessionDir: t.TempDir()})

	if err := Login(c, "10000", "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	if err := Login(c, "10000", "ok"); err != nil {
		t.Fatal(err)
	}
	if got["user"] != "10000" || got["pid"] != "7" || got["submit"] != "Anmelden" {
		t.Errorf("posted = %v", got)
	}
	if p, err := c.Page("/mein-profil"); err != nil || p.Status != 200 {
		t.Fatalf("after login: %v", err)
	}

	// Expired session: Page re-logs in transparently.
	c.ClearSession()
	c.Relogin = func(c *client.Client) error { return Login(c, "10000", "ok") }
	if _, err := c.Page("/mein-profil"); err != nil {
		t.Fatalf("relogin: %v", err)
	}
}
