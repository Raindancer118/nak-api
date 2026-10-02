package profile

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Raindancer118/nak-api/internal/client"
)

const base = "https://cis.example"

func fx(t *testing.T, n string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + n)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParsePersonal(t *testing.T) {
	p := ParsePersonal(fx(t, "meine-daten.html"))
	if p.Vorname != "Max" || p.Nachname != "Mustermann" || p.Login != "10000" || p.Matrikelnr != "99999" ||
		p.Zenturie != "I23a" || p.Studiengang != "Wirtschaftsinformatik" || p.Firma != "Beispiel GmbH" ||
		p.EmailPrivat != "max@example.org" || p.Ort != "12345 Musterstadt" {
		t.Errorf("personal = %+v", p)
	}
	if p.Jahrgang() != "23" {
		t.Errorf("jahrgang = %q", p.Jahrgang())
	}
}

func TestParseSettings(t *testing.T) {
	f, err := ParseFreigabe(fx(t, "notenfreigabe.html"), base)
	if err != nil || !f.Noten || !f.Anmeldungen {
		t.Errorf("freigabe = %+v %v", f, err)
	}
	d, err := ParseDatenfreigabe(fx(t, "datenfreigabe.html"), base)
	if err != nil || len(d) != 7 || !strings.HasPrefix(d["Adresse"], "0 ") || !strings.HasPrefix(d["Mobil"], "1 ") {
		t.Errorf("datenfreigabe = %v %v", d, err)
	}
	e, ip, err := ParseEmail(fx(t, "emails.html"), base)
	if err != nil || e != "max@example.org" || !ip {
		t.Errorf("email = %q %v %v", e, ip, err)
	}
	fest, mobil, fax, err := ParsePhone(fx(t, "telefon.html"), base)
	if err != nil || fest != "0123-4567890" || mobil != "0123-4567890" || fax != "" {
		t.Errorf("phone = %q %q %q %v", fest, mobil, fax, err)
	}
	a, err := ParseAddress(fx(t, "adressen.html"), base)
	if err != nil || a.TypeID != "1" || a.TypeName != "Hauptadresse" || a.Strasse != "Musterweg 1" || a.PLZ != "12345" ||
		a.Types["5"] != "Semesteradresse" || !strings.Contains(a.LetterForm, "Musterweg 1") {
		t.Errorf("address = %+v %v", a, err)
	}
	pay, err := ParsePayment(fx(t, "zahlung.html"), base)
	if err != nil || pay.MandateGiven || pay.IBANMasked != "" || pay.Kontoinhaber != "Max Mustermann" {
		t.Errorf("payment = %+v %v", pay, err)
	}
	v, err := ParseVertiefung(fx(t, "vertiefung.html"), base)
	if err != nil || v.Options["8"] != "E-Commerce" || v.Current == "" {
		t.Errorf("vertiefung = %+v %v", v, err)
	}
	bal := ParseBalance(fx(t, "guthaben.html"))
	if bal["Kopierguthaben"] != "0,00 €" {
		t.Errorf("balance = %v", bal)
	}
}

func TestClassmatesExportNoContactData(t *testing.T) {
	cm := ParseClassmates(fx(t, "kommilitonen.html"), base)
	if cm.Zenturie != "I23a" || cm.Zenturien["A23a"] != "636" {
		t.Errorf("zenturie = %q (%d options)", cm.Zenturie, len(cm.Zenturien))
	}
	if len(cm.People) != 36 {
		t.Fatalf("people = %d", len(cm.People))
	}
	found := false
	for _, p := range cm.People {
		if p.Name == "Max Mustermann" && p.Betrieb == "Beispiel GmbH" {
			found = true
		}
	}
	if !found {
		t.Error("own entry missing")
	}
}

// fakeCIS serves fixtures by path and records every request.
type fakeCIS struct {
	mu    sync.Mutex
	pages map[string]string
	reqs  []*http.Request
	forms []url.Values
}

func newFake(t *testing.T, pages map[string]string) (*fakeCIS, *client.Client) {
	f := &fakeCIS{pages: pages}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		r.ParseForm()
		f.reqs = append(f.reqs, r)
		f.forms = append(f.forms, r.PostForm)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if body, ok := f.pages[r.URL.Path]; ok {
			w.Write([]byte(body))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	c, err := client.NewWith(client.Options{BaseURL: srv.URL, SessionDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func (f *fakeCIS) posts() int {
	n := 0
	for _, r := range f.reqs {
		if r.Method == http.MethodPost {
			n++
		}
	}
	return n
}

func TestPrepareEmailBuildsButDoesNotSend(t *testing.T) {
	fake, c := newFake(t, map[string]string{EmailsPath: fx(t, "emails.html")})
	email := "New@Example.ORG"
	off := false
	s, err := PrepareEmail(c, &email, &off)
	if err != nil {
		t.Fatal(err)
	}
	if fake.posts() != 0 {
		t.Fatal("prepare must not POST")
	}
	v := s.Values()
	if v.Get("tx_nastudentdata_nastudentdataemail[email][privat]") != "new@example.org" {
		t.Errorf("email not lower-cased: %v", v)
	}
	if got := v["tx_nastudentdata_nastudentdataemail[email][infopost]"]; len(got) != 1 || got[0] != "" {
		t.Errorf("infopost = %v", got)
	}
	if v.Get("tx_nastudentdata_nastudentdataemail[email][typ]") != "privat" || v.Get("tx_nastudentdata_nastudentdataemail[__trustedProperties]") == "" {
		t.Errorf("hidden fields lost: %v", v)
	}

	// Sending goes to the fake, never to the real CIS.
	if _, err := s.Send(c); err != nil {
		t.Fatal(err)
	}
	if fake.posts() != 1 || fake.forms[len(fake.forms)-1].Get("tx_nastudentdata_nastudentdataemail[email][privat]") != "new@example.org" {
		t.Errorf("sent form = %v", fake.forms)
	}

	bad := "not an address"
	if _, err := PrepareEmail(c, &bad, nil); err == nil {
		t.Error("invalid address accepted")
	}
}

func TestPrepareOthers(t *testing.T) {
	_, c := newFake(t, map[string]string{
		TelefonPath: fx(t, "telefon.html"), NotenfreigabePath: fx(t, "notenfreigabe.html"),
		DatenfreigabePath: fx(t, "datenfreigabe.html"), AdressenPath: fx(t, "adressen.html"), VertiefungPath: fx(t, "vertiefung.html"),
	})
	mob := "0170-1234567"
	s, err := PreparePhone(c, nil, &mob, nil)
	if err != nil || s.Values().Get("tx_nastudentdata_nastudentdataphone[telefon][mobil]") != mob || len(s.Changes) != 1 {
		t.Errorf("phone: %v %+v", err, s)
	}
	bad := "abc"
	if _, err := PreparePhone(c, &bad, nil, nil); err == nil {
		t.Error("letters in phone accepted")
	}
	no := false
	s, err = PrepareFreigabe(c, &no, nil)
	if err != nil || s.Values().Get("tx_nastudentdata_nastudentdatabetriebsfreigabe[notenfreigabe]") != "0" ||
		s.Values().Get("tx_nastudentdata_nastudentdatabetriebsfreigabe[anmeldungfreigabe]") != "1" {
		t.Errorf("freigabe: %v %v", err, s.Values())
	}
	s, err = PrepareDatenfreigabe(c, map[string]string{"Adresse": "1"})
	if err != nil || s.Values().Get("tx_nastudentdata_nastudentdatasettings[Adresse]") != "1" {
		t.Errorf("datenfreigabe: %v", err)
	}
	if _, err := PrepareDatenfreigabe(c, map[string]string{"Adresse": "9"}); err == nil {
		t.Error("invalid level accepted")
	}
	if _, err := PrepareDatenfreigabe(c, map[string]string{"Schuhgröße": "1"}); err == nil {
		t.Error("unknown field accepted")
	}
	street := "Neuer Weg 2"
	s, err = PrepareAddress(c, "", nil, &street, nil, nil, false)
	if err != nil || s.Values().Get("tx_nastudentdata_nastudentdataaddress[address][street]") != street || s.Button != "Adresse ändern" {
		t.Errorf("address: %v %+v", err, s)
	}
	s, err = PrepareAddress(c, "", nil, nil, nil, nil, true)
	if err != nil || s.Button != "Adresse löschen" {
		t.Errorf("address delete: %v %+v", err, s)
	}
	s, err = PrepareVertiefung(c, "8")
	if err != nil || s.Values().Get("tx_navertiefungsrichtung_navertiefungsrichtung[vertiefungsrichtung]") != "8" {
		t.Errorf("vertiefung: %v", err)
	}
}
