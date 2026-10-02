package mensa

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func read(t *testing.T, f string) string {
	b, err := os.ReadFile("testdata/" + f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseMenu(t *testing.T) {
	ds := ParseMenu(read(t, "menuplan.html"))
	if len(ds) != 9 { // Friday's "Menü 2 veg." is only a placeholder
		t.Fatalf("got %d dishes: %+v", len(ds), ds)
	}
	d := ds[0]
	if d.Date != "2026-09-28" || d.Line != "Menü 1" || d.Name != "Griechische Hirtenrolle, Tomatenreis, Krautsalat und Tzatziki" || d.Price != 5 {
		t.Errorf("first %+v", d)
	}
	if strings.Join(d.Allergens, ",") != "Glutenhaltiges Getreide,Eier,Milch,Sellerie,Senf" {
		t.Errorf("allergens %q", d.Allergens)
	}
	if strings.Join(d.Tags, ",") != "Rind,Schwein" { // Symbolik 6
		t.Errorf("tags %q", d.Tags)
	}
	if v := ds[1]; v.Line != "Menü 2 Veg." || !v.Vegetarian() {
		t.Errorf("second %+v", v)
	}
}

func TestParseAccount(t *testing.T) {
	acc, err := ParseAccount(read(t, "history.html"), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Balance != 8.07 || acc.Card != "11111" {
		t.Errorf("account %+v", acc)
	}
	b := acc.Bookings
	if len(b) == 0 {
		t.Fatal("no bookings")
	}
	// "Hinweis: <dish>" rows describe the meal booked in the same minute
	first := b[0]
	if first.Item != "Menü 1" || !strings.HasPrefix(first.Dish, "Putenschnitzel") || first.Amount != -5 || first.Qty != 1 ||
		first.At.Format("2006-01-02 15:04") != "2026-09-23 13:31" {
		t.Errorf("first %+v", first)
	}
	for _, x := range b {
		if strings.HasPrefix(x.Item, "Hinweis") {
			t.Errorf("hint row kept: %+v", x)
		}
	}
}

func TestSpending(t *testing.T) {
	bs := []Booking{
		{Item: "Menü 1", Qty: 1, Amount: -5},
		{Item: "Becher Filterkaffee 0,3", Qty: 1, Amount: -1},
		{Item: "Aufladung 50€", Qty: 1, Amount: 50},
		{Item: "Belastung Pfand", Qty: 1, Amount: -2.5},
		{Item: "Erstattung Pfand", Qty: 1, Amount: 2.5},
		{Item: "Menü 1", Qty: 2, Amount: -10},
	}
	s := Spend(bs)
	if s.Food != 16 || s.Meals != 3 || s.TopUps != 50 {
		t.Errorf("spending %+v", s)
	}
}

// fake OPC server: cookie consent first, then the LDAP login, then pages
func fakeOPC(t *testing.T, logins *int32) *httptest.Server {
	consent := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "cookie_consent") {
			consent = true
			http.SetCookie(w, &http.Cookie{Name: "opc-cookies_accepted", Value: "1", Path: "/"})
			w.Write([]byte(`true`))
			return
		}
		var req map[string]any
		json.Unmarshal(body, &req)
		if !consent || req["service"] != "login" || req["action"] != "ldap" || req["username"] != "12345" || req["password"] != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		atomic.AddInt32(logins, 1)
		http.SetCookie(w, &http.Cookie{Name: "opc-webbestellung", Value: "s", Path: "/"})
		w.Write([]byte(`true`))
	})
	page := func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("opc-webbestellung"); err != nil || c.Value != "s" {
			w.Write([]byte(`<form id="login-form"><input id="f_kartennr" name="f_kartennr"></form>`))
			return
		}
		w.Write([]byte(read(t, "history.html")))
	}
	mux.HandleFunc("/", page)
	mux.HandleFunc("/mdhistory.php", page)
	mux.HandleFunc("/menuplan.php", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(read(t, "menuplan.html"))) })
	return httptest.NewServer(mux)
}

func TestClientAccount(t *testing.T) {
	var logins int32
	srv := fakeOPC(t, &logins)
	defer srv.Close()
	c := New(srv.URL, "12345", "pw")
	acc, err := c.Account(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Now())
	if err != nil || acc.Balance != 8.07 {
		t.Fatalf("%+v %v", acc, err)
	}
	if _, err := c.Account(time.Now().AddDate(0, -1, 0), time.Now()); err != nil {
		t.Fatal(err)
	}
	if logins != 1 {
		t.Errorf("session not reused: %d logins", logins)
	}
	bad := New(srv.URL, "12345", "wrong")
	if _, err := bad.Account(time.Now(), time.Now()); err != ErrAuth {
		t.Errorf("want ErrAuth, got %v", err)
	}
}

func TestClientMenu(t *testing.T) {
	var logins int32
	srv := fakeOPC(t, &logins)
	defer srv.Close()
	ds, err := New(srv.URL, "", "").Menu(1)
	if err != nil || len(ds) != 9 {
		t.Fatalf("%d %v", len(ds), err)
	}
}
