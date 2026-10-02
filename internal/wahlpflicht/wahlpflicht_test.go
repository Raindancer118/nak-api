package wahlpflicht

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Raindancer118/nak-api/internal/client"
)

func fx(t *testing.T, n string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + n)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseOpenList(t *testing.T) {
	l := Parse(fx(t, "liste-offen.html"))
	if len(l.Chosen) != 2 || l.Chosen[0].ID != "1081" || l.Chosen[0].Name != "IT Sicherheit" || l.Chosen[0].Termin != "Donnerstag" || !l.Chosen[0].Chosen {
		t.Errorf("chosen = %+v", l.Chosen)
	}
	if !l.Open || len(l.Available) < 10 {
		t.Fatalf("available = %d open=%v", len(l.Available), l.Open)
	}
	m := l.Available[0]
	if m.ID != "500" || m.CurriculumID != "161" || m.Name != "Marketing Projekt" || m.Dozent != "Thomas Gey" || m.Vertiefung != "Marketing" || m.Termin != "Montag" {
		t.Errorf("available[0] = %+v", m)
	}
}

func TestParseClosedList(t *testing.T) {
	l := Parse(fx(t, "liste-geschlossen.html"))
	if l.Open || len(l.Available) != 0 || len(l.Chosen) != 3 {
		t.Errorf("closed list = %+v", l)
	}
	if !strings.Contains(l.Notice, "noch keine Wahlpflichtmodule") {
		t.Errorf("notice = %q", l.Notice)
	}
}

func TestParseDetail(t *testing.T) {
	d := ParseDetail("1657", fx(t, "detail.html"), "https://cis.example")
	if d.Title != "KI und Management" || d.Pruefung != "Hausarbeit" || d.Credits != "6" || !strings.Contains(d.Workload, "180") ||
		len(d.Dozenten) != 1 || !strings.Contains(d.Dozenten[0], "Böhmke") || !strings.Contains(d.Lerninhalte, "Chatbots") ||
		d.Literatur == "" || d.SelectAvail || len(d.Termine) != 0 {
		t.Errorf("detail = %+v", d)
	}
	if _, err := choose(d, ""); err == nil {
		t.Error("choice outside the period must fail")
	}
}

// detail-wahl-offen.html is SYNTHETIC: the real detail page with two Termine
// whose order links follow the request pattern recorded in the HAR capture.
func TestChooseTermin(t *testing.T) {
	d := ParseDetail("1657", fx(t, "detail-wahl-offen.html"), "https://cis.example")
	if !d.SelectAvail || len(d.Termine) != 2 || d.Termine[0].ID != "9532" || d.Termine[0].Text != "Mittwoch, 14:00 - 19:00 Uhr (Gruppe A)" {
		t.Fatalf("termine = %+v", d.Termine)
	}
	if _, err := choose(d, ""); err == nil || !strings.Contains(err.Error(), "9533") {
		t.Errorf("ambiguous choice must list termin ids: %v", err)
	}
	ch, err := choose(d, "9533")
	if err != nil || !strings.Contains(ch.Termin.OrderURL, "action%5D=order") || !strings.Contains(ch.Termin.OrderURL, "%5Bid%5D=9533") {
		t.Errorf("choice = %+v %v", ch, err)
	}
}

func TestFetchDetailUsesReadOnlyShowForm(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		methods = append(methods, r.Method+" "+r.URL.Query().Get("tx_nawahlpflichtmodule_na_wahlpflichtmodule[action]")+" "+r.PostForm.Get("tx_nawahlpflichtmodule_na_wahlpflichtmodule[id]"))
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodPost {
			w.Write([]byte(fx(t, "detail-wahl-offen.html")))
			return
		}
		w.Write([]byte(fx(t, "liste-geschlossen.html")))
	}))
	defer srv.Close()
	c, _ := client.NewWith(client.Options{BaseURL: srv.URL, SessionDir: t.TempDir()})
	c.ReadOnly = true // the detail view must work without write permission
	ch, err := PrepareSelect(c, "1657", "9532")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, "|") != "GET  |POST show 1657" {
		t.Errorf("requests = %v", methods)
	}
	if _, err := Submit(c, ch); err != client.ErrReadOnly {
		t.Errorf("submit in read-only = %v", err)
	}
}
