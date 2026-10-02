package eduvault

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA1, truncated to the 6 digits EduVault uses.
func TestTOTPMatchesRFC6238(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // "12345678901234567890"
	for _, c := range []struct {
		at   int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		got, err := TOTP(secret, time.Unix(c.at, 0))
		if err != nil || got != c.want {
			t.Errorf("TOTP(%d) = %q, %v; want %s", c.at, got, err, c.want)
		}
	}
	// lower case and spaces, as people paste them
	if got, _ := TOTP("gezd gnbv gy3t qojq gezd gnbv gy3t qojq", time.Unix(59, 0)); got != "287082" {
		t.Errorf("normalised secret: %q", got)
	}
	if _, err := TOTP("not base32!", time.Now()); err == nil {
		t.Error("invalid secret accepted")
	}
}

type fakeEV struct {
	mu       sync.Mutex
	logins   int
	searches []string
	valid    string
	srv      *httptest.Server
}

func newFake(t *testing.T) *fakeEV {
	f := &fakeEV{valid: "jwt-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/mcp/session", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Token, Code string }
		json.NewDecoder(r.Body).Decode(&in)
		want, _ := TOTP("JBSWY3DPEHPK3PXP", time.Now())
		if in.Token != "evm_abc_def" || in.Code != want {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":"Ungültige Zugangsdaten"}`)
			return
		}
		f.mu.Lock()
		f.logins++
		f.mu.Unlock()
		io.WriteString(w, `{"session":"`+f.valid+`","expires_in":7200}`)
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		ok := r.Header.Get("Authorization") == "Bearer "+f.valid
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(401)
		}
		return ok
	}
	mux.HandleFunc("GET /api/storage/search", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		f.searches = append(f.searches, r.URL.RawQuery)
		f.mu.Unlock()
		modul := r.URL.Query().Get("modul")
		var res []map[string]any
		switch modul {
		case "Diskrete Mathematik 2":
			res = []map[string]any{{"id": "11111111-1111-1111-1111-111111111111", "modul": "Diskrete Mathematik 2", "type": "exam", "jahr": "2024", "has_solutions": true, "original_name": "dm2-2024.pdf"}}
		case "Diskrete Mathematik II":
			res = []map[string]any{
				{"id": "11111111-1111-1111-1111-111111111111", "modul": "Diskrete Mathematik 2", "type": "exam", "jahr": "2024"},
				{"id": "22222222-2222-2222-2222-222222222222", "modul": "Diskrete Mathematik II", "type": "probeklausur", "jahr": "2025"},
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"results": res, "total": len(res)})
	})
	mux.HandleFunc("GET /api/storage/file/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="dm2-2024.pdf"`)
		io.WriteString(w, "%PDF-1.7 "+r.PathValue("id"))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeEV) client() *Client { return New(f.srv.URL, "evm_abc_def", "JBSWY3DPEHPK3PXP") }

func TestLoginSearchAndReloginOn401(t *testing.T) {
	f := newFake(t)
	c := f.client()
	r, err := c.Search(Query{Modul: "Diskrete Mathematik 2", Types: []string{"exam"}})
	if err != nil || len(r.Results) != 1 || f.logins != 1 {
		t.Fatalf("search: %v %+v logins=%d", err, r, f.logins)
	}
	if !strings.Contains(f.searches[0], "type=exam") || !strings.Contains(f.searches[0], "modul=Diskrete") {
		t.Fatalf("query %q", f.searches[0])
	}
	c.Search(Query{Modul: "x"})
	if f.logins != 1 {
		t.Fatalf("session not reused: logins=%d", f.logins)
	}
	// server-side revocation: one silent relogin
	f.mu.Lock()
	f.valid = "jwt-2"
	f.mu.Unlock()
	if _, err := c.Search(Query{Modul: "x"}); err != nil || f.logins != 2 {
		t.Fatalf("relogin: %v logins=%d", err, f.logins)
	}
}

func TestWrongCredentialsAreReadable(t *testing.T) {
	f := newFake(t)
	_, err := New(f.srv.URL, "evm_abc_def", "GEZDGNBVGY3TQOJQ").Search(Query{Modul: "x"})
	if !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "Ungültige Zugangsdaten") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(f.srv.URL, "", "").Search(Query{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
}

func TestModuleSearchMergesSpellings(t *testing.T) {
	f := newFake(t)
	got, err := f.client().ForModule("Diskrete Mathematik 2")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["jahr"] != "2025" {
		t.Fatalf("merged = %v", got)
	}
	if len(f.searches) != 2 {
		t.Fatalf("searches = %v", f.searches)
	}
}

func TestSpellings(t *testing.T) {
	for in, want := range map[string][]string{
		"Diskrete Mathematik 2":  {"Diskrete Mathematik 2", "Diskrete Mathematik II"},
		"Diskrete Mathematik II": {"Diskrete Mathematik II", "Diskrete Mathematik 2"},
		"Mathematik1":            {"Mathematik1", "Mathematik 1", "Mathematik I"},
		"Marketing":              {"Marketing"},
	} {
		got := Spellings(in)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("Spellings(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileOnlyForUUIDs(t *testing.T) {
	f := newFake(t)
	c := f.client()
	data, ctype, name, err := c.File("11111111-1111-1111-1111-111111111111")
	if err != nil || !strings.HasPrefix(string(data), "%PDF") || ctype != "application/pdf" || name != "dm2-2024.pdf" {
		t.Fatalf("file: %q %q %q %v", data, ctype, name, err)
	}
	if _, _, _, err := c.File("../admin"); err == nil {
		t.Fatal("non-UUID id accepted")
	}
}
