package grades

import (
	"os"
	"testing"
)

func TestParseTranscriptText(t *testing.T) {
	b, err := os.ReadFile("testdata/notenuebersicht.txt")
	if err != nil {
		t.Fatal(err)
	}
	tg, err := ParseTranscriptText(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if tg.Average != "2,4" || tg.Date != "02.10.2026" {
		t.Errorf("average %q date %q", tg.Average, tg.Date)
	}
	want := map[string][2]string{ // nr → grade, credits
		"I140": {"3,7", "5"},
		"I166": {"1,7", "6"},
		"I168": {"4,0 (2. Versuch)", ""},
		"I179": {"bestanden", "6"},
		"I201": {"2,0 *", "5"},
	}
	got := map[string]TranscriptGrade{}
	for _, g := range tg.Modules {
		got[g.ModuleNr] = g
	}
	for nr, w := range want {
		g, ok := got[nr]
		if !ok || g.Grade != w[0] || g.Credits != w[1] {
			t.Errorf("%s = %+v, want grade %q credits %q", nr, g, w[0], w[1])
		}
	}
	if _, ok := got["I151"]; ok {
		t.Error("modules without an exam (#) are not grades")
	}
	if g := got["I166"]; g.Title != "Einführung in die objektorientierte Programmierung" {
		t.Errorf("title %q", g.Title)
	}
	if _, err := ParseTranscriptText("irgendein anderes PDF"); err == nil {
		t.Error("a PDF without the grade table must be reported as drift")
	}
}

func TestSameGrade(t *testing.T) {
	for _, c := range [][2]string{{"4,0 (2. Versuch)", "4,0 (2.Versuch)"}, {"1,7", " 1,7 "}, {"2,0 *", "2,0"}, {"bestanden", "Bestanden"}} {
		if NormalizeGrade(c[0]) != NormalizeGrade(c[1]) {
			t.Errorf("%q and %q should be the same grade", c[0], c[1])
		}
	}
	if NormalizeGrade("4,0 (2. Versuch)") == NormalizeGrade("4,0") {
		t.Error("a retake is a different result than the first attempt")
	}
}
