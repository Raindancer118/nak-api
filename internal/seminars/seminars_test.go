package seminars

import (
	"os"
	"strings"
	"testing"
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

func TestParseList(t *testing.T) {
	l := Parse(fx(t, "liste.html"), base)
	if l.Quarter != "2026 - 4" || l.Quarters["2025 - 1"] != "129" {
		t.Errorf("quarter = %q %v", l.Quarter, l.Quarters)
	}
	if !strings.Contains(l.Notice, "Wahlzeitraum") {
		t.Errorf("notice = %q", l.Notice)
	}
	if len(l.Seminars) != 36 {
		t.Fatalf("seminars = %d", len(l.Seminars))
	}
	s := l.Seminars[0]
	if s.ID != "10606" || s.Title != "Chinesisch für Anfänger A1.1 ohne Vorkenntnisse" || s.Lecturer != "Chun Ping Lange" ||
		s.From != "Montag, 12.10.2026" || s.To != "Montag, 30.11.2026" || s.Category != "Internationales / Sprachen" ||
		!strings.Contains(s.Info, "17:45") {
		t.Errorf("first = %+v", s)
	}
	if len(s.Actions) != 1 || s.Actions[0].Action != "subscribeToSeminar" || s.Actions[0].Label != "Für dieses Seminar anmelden" {
		t.Errorf("actions = %+v", s.Actions)
	}
	cancelled, waits := 0, 0
	for _, x := range l.Seminars {
		if x.Cancelled {
			cancelled++
		}
		for _, a := range x.Actions {
			if a.Action == "anWartelisteAnmelden" {
				waits++
			}
		}
	}
	if cancelled != 11 || waits != 0 {
		t.Errorf("cancelled=%d waitlist actions=%d", cancelled, waits)
	}
}

func TestParseMine(t *testing.T) {
	l := Parse(fx(t, "meine.html"), base)
	if len(l.Seminars) != 6 && len(l.Seminars) != 7 {
		t.Fatalf("mine = %d", len(l.Seminars))
	}
	s := l.Seminars[0]
	if s.ID != "10231" || len(s.Status) != 2 || s.Status[0] != "Seminar besucht" || s.Status[1] != "Teilnehmer am Seminar" || len(s.Actions) != 0 {
		t.Errorf("mine[0] = %+v", s)
	}
}

func TestParseDetail(t *testing.T) {
	d := ParseDetail(fx(t, "detail.html"))
	if d.Title != "Führung & Verantwortung" || d.From != "Freitag, 16.10.2026" || d.To != "Samstag, 17.10.2026" ||
		d.Topic != "Ethik/Soziales" || d.ExamForm != "Test" || d.Credits != "1" || d.Workload != "30" || d.Lecturer != "Andreas Pfeil" {
		t.Errorf("detail = %+v", d)
	}
	if !strings.Contains(d.Remark, "Online- Seminar") || !strings.Contains(d.CV, "Business Coach") || !strings.Contains(d.Description, "GRID") {
		t.Errorf("texts: remark=%q cv=%q desc=%q", d.Remark, d.CV, d.Description)
	}
}

func TestCountList(t *testing.T) {
	n, pos := CountList(fx(t, "teilnehmer.html"), "Nachname", "Vorname")
	if n == 0 || pos != 1 {
		t.Errorf("n=%d pos=%d", n, pos)
	}
	if _, pos := CountList(fx(t, "teilnehmer.html"), "Mustermann", "Max"); pos != 0 {
		t.Error("false positive")
	}
}

func TestPageActionsOnWaitlistPage(t *testing.T) {
	body := `<main><a class="btn btn-lg" href="/studium/bachelor/seminare?tx_naseminar_naseminar%5Baction%5D=anWartelisteAnmelden&amp;tx_naseminar_naseminar%5BseminarId%5D=10605&amp;cHash=0">An Warteliste anmelden</a>
<a href="/studium/bachelor/seminare?tx_naseminar_naseminar%5Baction%5D=showWaitList&amp;tx_naseminar_naseminar%5BseminarId%5D=10605">x</a></main>`
	as := PageActions(body, base, "10605")
	if len(as) != 1 || as[0].Action != "anWartelisteAnmelden" || as[0].Label != "An Warteliste anmelden" {
		t.Fatalf("actions = %+v", as)
	}
	if len(PageActions(body, base, "1")) != 0 {
		t.Error("other seminar matched")
	}
}
