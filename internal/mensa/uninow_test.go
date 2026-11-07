package mensa

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fakeUniNow(t *testing.T, tokens *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UserAgent     json.RawMessage `json:"userAgent"`
			ApplicationID string          `json:"applicationId"`
			GrantType     string          `json:"grantType"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		ts := r.Header.Get("X-Authorization-Timestamp")
		m := hmac.New(sha256.New, []byte(uniNowVKey("com.uninow.ne", "9.9.9", "s3cret")))
		m.Write(append([]byte(body.UserAgent), ts...))
		if r.Header.Get("Authorization") != "HMAC-SHA256" || body.GrantType != "hmac" || body.ApplicationID != "com.uninow.ne" ||
			r.Header.Get("X-Authorization-Content-SHA256") != hex.EncodeToString(m.Sum(nil)) || r.Header.Get("X-App-Version") != "9.9.9" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"status":"Not Authenticated"}`)
			return
		}
		atomic.AddInt32(tokens, 1)
		exp := time.Now().Add(24 * time.Hour).Unix()
		claims, _ := json.Marshal(map[string]any{"exp": exp})
		jwt := "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"
		io.WriteString(w, `{"token":{"type":"Bearer","token":"`+jwt+`"},"expires_in":86400}`)
	})
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Variables map[string]string `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&q)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer h.") || r.Header.Get("Apollo-Require-Preflight") != "true" || q.Variables["uniID"] != "de_ne" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, read(t, "uninow_cafeteria.json"))
	})
	return httptest.NewServer(mux)
}

func testUniNow(srv *httptest.Server) *UniNow {
	u := NewUniNow("s3cret")
	u.AccountsURL, u.GraphQLURL, u.AppVersion = srv.URL, srv.URL+"/graphql", "9.9.9"
	u.Zone = time.UTC
	return u
}

func TestUniNowMenu(t *testing.T) {
	var tokens int32
	srv := fakeUniNow(t, &tokens)
	defer srv.Close()
	u := testUniNow(srv)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) // Saturday
	ds, err := u.Menu(now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 3 {
		t.Fatalf("dishes = %+v", ds)
	}
	d := ds[0]
	if d.Date != "2026-11-09" || d.Line != "Menü 1" || d.Name != "Spaghetti Bolognese" || d.Price != 4.2 {
		t.Errorf("dish = %+v", d)
	}
	if strings.Join(d.Allergens, ",") != "Glutenhaltiges Getreide,Eier" || strings.Join(d.Additives, ",") != "mit Konservierungsstoff" {
		t.Errorf("allergens %v additives %v", d.Allergens, d.Additives)
	}
	if !ds[1].Vegetarian() || ds[0].Vegetarian() || strings.Join(ds[2].Tags, ",") != "Vegetarisch,Vegan" {
		t.Errorf("tags %v %v %v", ds[0].Tags, ds[1].Tags, ds[2].Tags)
	}
	if _, err := u.Menu(now, 2); err != nil || tokens != 1 {
		t.Errorf("token not reused: %d (%v)", tokens, err)
	}
}

func TestUniNowWithoutSecretIsOff(t *testing.T) {
	if NewUniNow("") != nil {
		t.Fatal("no secret must mean no UniNow client")
	}
}

func TestMenuPrefersUniNowAndFallsBackToOPC(t *testing.T) {
	var tokens, logins int32
	un := fakeUniNow(t, &tokens)
	defer un.Close()
	opc := fakeOPC(t, &logins)
	defer opc.Close()

	c := New(opc.URL, "", "")
	c.UniNow = testUniNow(un)
	c.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
	ds, err := c.Menu(1)
	if err != nil || len(ds) != 3 || ds[0].Name != "Spaghetti Bolognese" {
		t.Fatalf("uninow first: %d %v", len(ds), err)
	}

	// UniNow broken → OPC still delivers
	c.UniNow.GraphQLURL = un.URL + "/nope"
	ds, err = c.Menu(1)
	if err != nil || len(ds) != 9 {
		t.Fatalf("fallback: %d %v", len(ds), err)
	}

	// UniNow empty (weekend) and OPC down → empty, not an error
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer dead.Close()
	c2 := New(dead.URL, "", "")
	c2.UniNow = testUniNow(un)
	c2.Now = func() time.Time { return time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC) }
	if ds, err := c2.Menu(1); err != nil || len(ds) != 0 {
		t.Fatalf("empty uninow + dead opc: %d %v", len(ds), err)
	}
}
