package planning

import (
	"os"
	"testing"
	"time"
)

func fixture(t *testing.T, n string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + n)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseStudienplan(t *testing.T) {
	sp := ParseStudienplan(fixture(t, "studienplan.html"))
	if len(sp.Modules) != 30 {
		t.Fatalf("modules = %d, want 30", len(sp.Modules))
	}
	m := sp.Modules[0]
	if m.ModuleNr != "I140" || m.Title != "Automatentheorie und formale Sprachen" || m.Group != "Informatik" || m.Credits != 5 {
		t.Errorf("first = %+v", m)
	}
	if m.ExamSemester != 4 || m.ExamForm != "K" || m.ExamFormName != "Klausur" {
		t.Errorf("exam = %d %s", m.ExamSemester, m.ExamForm)
	}
	if len(m.Semesters) != 1 || m.Semesters[0] != (SemesterLoad{Semester: 3, Hours: 4}) {
		t.Errorf("load = %+v", m.Semesters)
	}
	byNr := map[string]Module{}
	for _, x := range sp.Modules {
		byNr[x.ModuleNr] = x
	}
	// "4 K" in one cell: hours and exam in the same semester.
	if x := byNr["I178"]; x.ExamSemester != 3 || x.TotalHours() != 8 {
		t.Errorf("I178 = %+v", x)
	}
	if x := byNr["I163"]; x.Group != "Bachelorarbeit" || x.ExamForm != "B" || x.Credits != 12 {
		t.Errorf("thesis = %+v", x)
	}
	if x := byNr["I177"]; x.ExamForm != "Pf" || x.ExamSemester != 7 {
		t.Errorf("Englisch = %+v", x)
	}
	if sp.TotalCredits() != 173 {
		t.Errorf("total credits = %d", sp.TotalCredits())
	}
	if sp.Legend["H"] != "Hausarbeit" {
		t.Errorf("legend = %v", sp.Legend)
	}
}

func TestParseVorlesungszeiten(t *testing.T) {
	qs := ParseVorlesungszeiten(fixture(t, "vorlesungszeiten.html"))
	if len(qs) != 16 {
		t.Fatalf("quarters = %d", len(qs))
	}
	if qs[0] != (Quarter{Name: "I/25", From: "06.01.2025", To: "14.03.2025"}) {
		t.Errorf("first = %+v", qs[0])
	}
	if qs[3].From != "13.10.2025" || qs[3].To != "19.12.2025" {
		t.Errorf("nbsp handling: %+v", qs[3])
	}
	if qs[10].Name != "III/27" || qs[10].From != "26.07.2027" || qs[10].To != "01.10.2027" {
		t.Errorf("no-space dash: %+v", qs[10])
	}
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	cur, next := Current(qs, now)
	if cur == nil || cur.Name != "III/26" || next == nil || next.Name != "IV/26" {
		t.Errorf("current/next = %+v / %+v", cur, next)
	}
}

func TestParseAbschlussfristen(t *testing.T) {
	fs := ParseAbschlussfristen(fixture(t, "startseite.html"))
	if len(fs) != 6 {
		t.Fatalf("fristen = %d", len(fs))
	}
	f := fs[2]
	if f.Abschluss != "März 2027" || f.LetzteNoten != "24.03.2027" || f.Pruefungsausschuss != "31.03.2027" || f.Graduierung != "Bachelor-Graduierung: 16.04.2027" {
		t.Errorf("frist = %+v", f)
	}
}
