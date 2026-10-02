package exams

import (
	"os"
	"strings"
	"testing"
	"time"
)

func loadFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/exams.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseExams(t *testing.T) {
	list := legacy(t)
	if len(list) == 0 {
		t.Fatal("expected exams, got none")
	}

	// First row is known from the fixture.
	first := list[0]
	if first.ModuleNr != "I210" {
		t.Errorf("module = %q, want I210", first.ModuleNr)
	}
	if first.Title != "Betriebliche Anwendungssysteme" {
		t.Errorf("title = %q", first.Title)
	}
	if len(first.Zenturien) != 2 || first.Zenturien[0] != "I24a" || first.Zenturien[1] != "I24b" {
		t.Errorf("zenturien = %v, want [I24a I24b]", first.Zenturien)
	}
	if len(first.Dozenten) != 1 || first.Dozenten[0] != "Mustermann, Max" {
		t.Errorf("dozenten = %v", first.Dozenten)
	}
	if first.Start != "23.06.2026 11:30" || first.Ende != "23.06.2026 13:00" {
		t.Errorf("start/ende = %q / %q", first.Start, first.Ende)
	}
	if first.Action != "register" {
		t.Errorf("action = %q, want register", first.Action)
	}
	if first.ExamID != "11993" {
		t.Errorf("examID = %q, want 11993", first.ExamID)
	}
	if first.ActionLabel != "Anmelden" {
		t.Errorf("label = %q, want Anmelden", first.ActionLabel)
	}
}

func TestActionURLDecoded(t *testing.T) {
	list := legacy(t)
	for _, e := range list {
		if e.ActionURL == "" {
			continue
		}
		if e.ActionURL[:4] != "http" {
			t.Errorf("action url not absolute: %q", e.ActionURL)
		}
		if containsSub(e.ActionURL, "&amp;") {
			t.Errorf("action url still encoded: %q", e.ActionURL)
		}
		if e.ExamID == "" {
			t.Errorf("action url without examId: %q", e.ActionURL)
		}
	}
}

func TestAllRowsHaveExamID(t *testing.T) {
	list := legacy(t)
	ids := map[string]bool{}
	for _, e := range list {
		if e.ExamID != "" {
			ids[e.ExamID] = true
		}
	}
	// The fixture is known to expose four distinct exams.
	for _, want := range []string{"11993", "11994", "12022", "12023"} {
		if !ids[want] {
			t.Errorf("missing examID %s", want)
		}
	}
}

func containsSub(s, sub string) bool { return strings.Contains(s, sub) }

func legacy(t *testing.T) []Exam {
	list, found := parseExams(loadFixture(t), "https://cis.example", time.Now())
	if !found {
		t.Fatal("table not found")
	}
	return list
}

func TestPersonalPageAndDeadlines(t *testing.T) {
	b, err := os.ReadFile("testdata/meine-pruefungen.html")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, berlin)
	list, found := parseExams(string(b), "https://cis.example", now)
	if !found || len(list) != 17 {
		t.Fatalf("found=%v rows=%d", found, len(list))
	}
	reg := list[1]
	if !reg.Registered || reg.Action != "deregister" || reg.ExamID != "12260" || reg.Section != "Studiengangsleistungen" {
		t.Errorf("registered row = %+v", reg)
	}
	if list[0].Registered || list[0].Action != "register" {
		t.Errorf("open row = %+v", list[0])
	}
	d := list[0].Deadlines
	if d == nil || d.RegisterOpens != "07.09.2026" || d.RegisterCloses != "22.09.2026 23:59" ||
		d.DeregisterUntil != "30.09.2026 23:59" || d.DaysUntilExam != 12 {
		t.Errorf("deadlines = %+v", d)
	}
	var wp *Exam
	for i := range list {
		if strings.Contains(list[i].Title, "Internationale Beziehungen") {
			wp = &list[i]
		}
	}
	if wp == nil || len(wp.Dozenten) != 2 || wp.Deadlines != nil {
		t.Errorf("multi-lecturer Hausarbeit row = %+v", wp)
	}
}

func TestFlashMessage(t *testing.T) {
	body := `<main><div class="typo3-messages"><div class="alert alert-success">Sie wurden angemeldet.</div></div></main>`
	if got := FlashMessage(body); got != "Sie wurden angemeldet." {
		t.Errorf("flash = %q", got)
	}
}
