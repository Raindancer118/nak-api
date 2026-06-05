package exams

import (
	"os"
	"testing"
)

func loadFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/exams.html")
	if err != nil {
		// Fixtures hold personal CIS data and are git-ignored; skip when absent.
		t.Skipf("fixture not present (kept local for privacy): %v", err)
	}
	return string(b)
}

func TestParseExams(t *testing.T) {
	list := parseExams(loadFixture(t))
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
	list := parseExams(loadFixture(t))
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
	list := parseExams(loadFixture(t))
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

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
