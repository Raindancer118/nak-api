package thesis

import (
	"os"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.ParseInLocation("02.01.2006", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

var quarters = []Quarter{
	{"III/26", day("27.07.2026"), day("02.10.2026")}, {"IV/26", day("12.10.2026"), day("18.12.2026")},
	{"I/27", day("11.01.2027"), day("19.03.2027")}, {"II/27", day("12.04.2027"), day("18.06.2027")},
	{"III/27", day("26.07.2027"), day("01.10.2027")},
}

var deadlines = []Deadline{
	{"November 2026", day("17.11.2026"), day("24.11.2026"), ""},
	{"März 2027", day("24.03.2027"), day("31.03.2027"), "Bachelor-Graduierung: 16.04.2027"},
	{"Juni 2027", day("01.06.2027"), day("08.06.2027"), ""},
	{"September 2027", day("23.09.2027"), day("30.09.2027"), ""},
}

// the official "Planungshilfe Studienabschluss Jahrgang 2023": latest
// registration KW 50 → start 21.12.2026 → submission 22.02.2027, review
// four lecture weeks, last grades 24.03.2027
func TestCalcMatchesThePlanningAid(t *testing.T) {
	c := Calc(day("21.12.2026"), quarters, deadlines)
	if c.Submission != "22.02.2027" {
		t.Errorf("submission %s, want 22.02.2027 (21.02. is a Sunday)", c.Submission)
	}
	if c.ReviewUntil != "22.03.2027" {
		t.Errorf("review until %s", c.ReviewUntil)
	}
	if c.Graduation == nil || c.Graduation.Name != "März 2027" || c.BufferDays != 2 {
		t.Errorf("graduation %+v buffer %d", c.Graduation, c.BufferDays)
	}
	if c.WithExtension != "29.03.2027" {
		t.Errorf("with the full extension (5 weeks): %s", c.WithExtension)
	}
	// earliest: registration 10.10.2026 → start 19.10.2026 → 19.12. is a Saturday
	e := Calc(StartFor(day("10.10.2026")), quarters, deadlines)
	if e.Start != "19.10.2026" || e.Submission != "21.12.2026" {
		t.Errorf("earliest: start %s submission %s", e.Start, e.Submission)
	}
	// the review counts lecture weeks only: after 21.12. the next lecture week is 11.01.
	if e.ReviewUntil != "08.02.2027" {
		t.Errorf("earliest review until %s", e.ReviewUntil)
	}
}

func TestLatestPerGraduation(t *testing.T) {
	l := Latest(deadlines[1], quarters, deadlines)
	if l.Start != "21.12.2026" || l.RegisterWeekFrom != "07.12.2026" || l.RegisterWeekTo != "13.12.2026" || l.Submission != "22.02.2027" {
		t.Errorf("latest for März 2027: %+v (planning aid: KW 50, start 21.12., submission 22.02.)", l)
	}
	if l.KW != 50 {
		t.Errorf("KW %d", l.KW)
	}
	// TL 1–5 must be passed at registration: the last one starts nine weeks before
	if l.TLStartBy != "11.10.2026" {
		t.Errorf("TL start by %s", l.TLStartBy)
	}
}

func TestParsePlanningAid(t *testing.T) {
	b, _ := os.ReadFile("testdata/planungshilfe-2023.txt")
	p := ParsePlanningAid(string(b))
	want := map[string]string{
		"cohort": "2023", "earliest_registration": "10.10.2026", "earliest_start": "19.10.2026",
		"latest_register_from": "07.12.2026", "latest_register_to": "13.12.2026", "latest_start": "21.12.2026",
		"latest_submission": "22.02.2027", "last_grades": "24.03.2027", "board": "31.03.2027", "ceremony": "16.04.2027",
		"semester7_from": "26.10.2026", "semester7_to": "13.11.2026", "repeat_from": "25.01.2027", "repeat_to": "05.02.2027",
	}
	got := map[string]string{"cohort": p.Cohort, "earliest_registration": p.EarliestRegistration, "earliest_start": p.EarliestStart,
		"latest_register_from": p.LatestRegisterFrom, "latest_register_to": p.LatestRegisterTo, "latest_start": p.LatestStart,
		"latest_submission": p.LatestSubmission, "last_grades": p.LastGrades, "board": p.Board, "ceremony": p.Ceremony,
		"semester7_from": p.Semester7From, "semester7_to": p.Semester7To, "repeat_from": p.RepeatFrom, "repeat_to": p.RepeatTo}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
	if !p.Found {
		t.Error("planning aid not recognised")
	}
}

func TestEligibility(t *testing.T) {
	b, _ := os.ReadFile("testdata/anmeldung-gesperrt.html")
	e := ParseEligibility(string(b))
	if e.Open || len(e.Reasons) != 2 || e.Reasons[1] != "Es wurden nicht alle benötigten Transferleistungen bestanden." {
		t.Errorf("blocked: %+v", e)
	}
	b, _ = os.ReadFile("testdata/anmeldung-offen.html")
	if e := ParseEligibility(string(b)); !e.Open || len(e.Reasons) != 0 {
		t.Errorf("open: %+v", e)
	}
}

func TestReviewers(t *testing.T) {
	b, _ := os.ReadFile("testdata/gutachtende.html")
	rs := ParseReviewers(string(b))
	if len(rs) != 3 {
		t.Fatalf("got %d reviewers", len(rs))
	}
	r := rs[0]
	if r.Name != "Beispiel, Erika" || r.Email != "erika.beispiel@nordakademie.example" || r.Department != "Informatik" || r.Load != 0 || r.LoadLabel != "frei" {
		t.Errorf("first %+v", r)
	}
	// wrapped lines inside one area join, separate areas stay separate
	if len(r.Areas) != 2 || r.Areas[1] != "Softwaretechnik und Softwarearchitektur" {
		t.Errorf("areas %q", r.Areas)
	}
	if rs[1].Load != 2 || rs[1].LoadLabel != "voll" {
		t.Errorf("second load %+v", rs[1])
	}
	if rs[2].Load != -1 {
		t.Errorf("no workload shown: %+v", rs[2])
	}
	if got := FilterReviewers(rs, "software"); len(got) != 1 || got[0].Name != "Beispiel, Erika" {
		t.Errorf("filter: %+v", got)
	}
}
