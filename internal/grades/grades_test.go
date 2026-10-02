package grades

import (
	"os"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseOverview(t *testing.T) {
	o := ParseOverview(fixture(t, "leistungsuebersicht.html"), "https://cis.example")
	if o.Student != "Max Mustermann" {
		t.Errorf("student = %q", o.Student)
	}
	if len(o.Modules) != 36 {
		t.Fatalf("modules = %d, want 36", len(o.Modules))
	}
	m := o.Modules[0]
	if m.ModuleNr != "I140" || m.Title != "Automatentheorie und formale Sprachen" || m.ExamDate != "22.04.2025" ||
		m.EntryDate != "19.05.2025" || m.Grade != "1,0" || m.Credits != "5" || m.Status != StatusPassed {
		t.Errorf("first module = %+v", m)
	}
	if m.GradeValue == nil || *m.GradeValue != 1.0 || m.Attempt != 1 {
		t.Errorf("value/attempt = %v/%d", m.GradeValue, m.Attempt)
	}
	if m.StatisticURL == "" || m.AttendanceURL == "" || m.PerformanceID == "" {
		t.Errorf("links missing: %+v", m)
	}

	byNr := map[string]ModuleGrade{}
	for _, x := range o.Modules {
		byNr[x.ModuleNr] = x
	}
	if d := byNr["I168"]; d.Attempt != 2 || d.GradeValue == nil || *d.GradeValue != 3.3 {
		t.Errorf("I168 attempt parsing: %+v", d)
	}
	if s := byNr["I151"]; s.Status != StatusOpen || s.GradeValue != nil || s.AttendanceURL == "" || s.StatisticURL != "" {
		t.Errorf("open module: %+v", s)
	}
	if tm := byNr["TM1"]; tm.Status != StatusPassed || tm.GradeValue != nil {
		t.Errorf("TM1 bestanden: %+v", tm)
	}

	if o.Average != "2.1" || o.CreditsTotal != "126" {
		t.Errorf("summary avg=%q credits=%q", o.Average, o.CreditsTotal)
	}
	if len(o.Seminars) != 7 || o.Seminars[1].Title != "Pitch perfect - Erfolgreiche Präsentationen gestalten" ||
		o.Seminars[1].Credits != "0,5" || o.Seminars[1].Period != "10.08.2024" {
		t.Errorf("seminars = %+v", o.Seminars)
	}
	if !o.Seminars[2].Recognized {
		t.Errorf("'best.*' must mark recognized: %+v", o.Seminars[2])
	}
	if o.SeminarCredits != "6,5" {
		t.Errorf("seminar credits = %q", o.SeminarCredits)
	}
	if len(o.Transfers) != 4 || o.Transfers[0].No != "1" || o.Transfers[0].Module != "I169 Allgemeine Betriebswirtschaftslehre" ||
		o.Transfers[0].Status != "bewertet und freigegeben" {
		t.Errorf("transfers = %+v", o.Transfers)
	}
	if o.TransferCredits != "20" {
		t.Errorf("transfer credits = %q", o.TransferCredits)
	}
	if len(o.Transcripts) != 2 || o.Transcripts[0].Lang != "de" || o.Transcripts[1].Lang != "en" {
		t.Errorf("transcripts = %+v", o.Transcripts)
	}
	st := o.Stats()
	if st.Passed == 0 || st.Open == 0 || st.WeightedAverage == 0 {
		t.Errorf("stats = %+v", st)
	}
}

func TestParseGradeCell(t *testing.T) {
	cases := []struct {
		in      string
		val     float64
		has     bool
		attempt int
		status  string
	}{
		{"1,7", 1.7, true, 1, StatusPassed},
		{"5,0 (2.Versuch)", 5.0, true, 2, StatusFailed},
		{"4,0 (m)", 4.0, true, 1, StatusPassed},
		{"bestanden", 0, false, 1, StatusPassed},
		{"best.*", 0, false, 1, StatusPassed},
		{"nicht bestanden", 0, false, 1, StatusFailed},
		{"", 0, false, 0, StatusOpen},
	}
	for _, c := range cases {
		v, attempt, status := parseGrade(c.in)
		if (v != nil) != c.has || (v != nil && *v != c.val) || attempt != c.attempt || status != c.status {
			t.Errorf("%q -> %v %d %s", c.in, v, attempt, status)
		}
	}
}

func TestParseDistribution(t *testing.T) {
	d, err := ParseDistribution(fixture(t, "statistic.html"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Module != "Automatentheorie und formale Sprachen" || d.Count != 110 || d.Average != 3.64 {
		t.Errorf("header = %+v", d)
	}
	if len(d.Lecturers) != 3 || d.Lecturers[0] != "Marcus Soll" {
		t.Errorf("lecturers = %v", d.Lecturers)
	}
	if len(d.Buckets) != 11 || d.Buckets[0].Grade != "1,0" || d.Buckets[10].Count != 30 {
		t.Errorf("buckets = %+v", d.Buckets)
	}
	sum := 0
	for _, b := range d.Buckets {
		sum += b.Count
	}
	if sum != d.Count {
		t.Errorf("bucket sum %d != count %d", sum, d.Count)
	}
	d.Place(3.7)
	if d.Better != 1+1+2+8+8+5+13+10 || d.Same != 10 || d.Worse != 52 {
		t.Errorf("place = better %d same %d worse %d", d.Better, d.Same, d.Worse)
	}
	if d.FailRate < 27 || d.FailRate > 28 {
		t.Errorf("fail rate = %.2f", d.FailRate)
	}
}

func TestParseAttendance(t *testing.T) {
	a := ParseAttendance(fixture(t, "anwesenheiten.html"))
	if a.ModuleNr != "I140" || a.Module != "Automatentheorie und formale Sprachen" {
		t.Errorf("header = %+v", a)
	}
	if len(a.Sessions) != 10 || a.Sessions[0].From != "Montag, 06.01.2025 11:30" || a.Sessions[6].Status != "Teilgenommen (online)" {
		t.Errorf("sessions = %+v", a.Sessions)
	}
	if a.Summary["Teilgenommen"] != 9 || a.Summary["Teilgenommen (online)"] != 1 || a.AttendanceRate != 100 {
		t.Errorf("summary = %v rate %.1f", a.Summary, a.AttendanceRate)
	}
	if a.Period != "06.01.2025 bis einschließlich 03.03.2025" {
		t.Errorf("period = %q", a.Period)
	}
}
