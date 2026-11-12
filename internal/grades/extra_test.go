package grades

import "testing"

// The exam of a module can run under another number (I222 for I168). The CIS
// then books the result under "weitere Leistungen" and leaves the module's own
// row at the earlier attempt.
func TestExtraResultCountsForItsModule(t *testing.T) {
	o := ParseOverview(fixture(t, "leistungsuebersicht_extra.html"), "https://cis.example")
	if len(o.Modules) != 3 || len(o.Extra) != 2 {
		t.Fatalf("modules = %d, extra = %d", len(o.Modules), len(o.Extra))
	}
	m := o.Find("I168")
	if m.Grade != "2,3" || m.GradeValue == nil || *m.GradeValue != 2.3 || m.Status != StatusPassed {
		t.Fatalf("I168 must show the result booked as I222: %+v", m)
	}
	if m.Attempt != 3 || m.Credits != "5" || m.EntryDate != "11.11.2026" || m.ExamDate != "" {
		t.Errorf("attempt after the failed second, credits and entry date of the new result, no exam date of the old one: %+v", m)
	}
	if m.Source != SourceExtra || m.RecordedAs != "I222" || m.Superseded != "5,0 (2.Versuch)" {
		t.Errorf("where the result comes from: %+v", m)
	}
	if m.StatisticURL != "" || m.PerformanceID != "" || m.AttendanceURL == "" {
		t.Errorf("the distribution belongs to the replaced attempt, attendance to the module: %+v", m)
	}
	if e := o.Extra[0]; e.CountsFor != "I168" || e.Grade != "2,3" || e.Status != StatusPassed {
		t.Errorf("extra row: %+v", e)
	}
	if e := o.Extra[1]; e.CountsFor != "" || e.Grade != "1,7" {
		t.Errorf("an extra result without a module of that name stays on its own: %+v", e)
	}
	if o.Find("I222") == nil || o.Find("X901") == nil {
		t.Error("extra results are still found by their own number")
	}
	st := o.Stats()
	if st.Passed != 2 || st.Failed != 0 || st.Open != 1 || st.CreditsEarned != 10 {
		t.Errorf("stats count the module as passed: %+v", st)
	}
}

func TestExtraOnlyReplacesWhatItSupersedes(t *testing.T) {
	val := func(g string) *float64 { v, _, _ := parseGrade(g); return v }
	extra := func(title, grade, entered string) ModuleGrade {
		_, a, st := parseGrade(grade)
		return ModuleGrade{ModuleNr: "I222", Title: title, Grade: grade, GradeValue: val(grade), Attempt: a, Status: st, EntryDate: entered, Credits: "5"}
	}
	mod := func(grade, entered string) ModuleGrade {
		_, a, st := parseGrade(grade)
		return ModuleGrade{ModuleNr: "I168", Title: "Diskrete Mathematik 2", Grade: grade, GradeValue: val(grade), Attempt: a, Status: st, EntryDate: entered}
	}
	cases := []struct {
		name          string
		mod, extra    ModuleGrade
		grade, status string
		attempt       int
	}{
		{"open module", mod("", ""), extra("Diskrete Mathematik 2", "3,0", "11.11.2026"), "3,0", StatusPassed, 1},
		{"roman numeral", mod("5,0", "10.03.2026"), extra("Diskrete  Mathematik II", "3,0", "11.11.2026"), "3,0", StatusPassed, 2},
		{"failed again", mod("5,0", "10.03.2026"), extra("Diskrete Mathematik 2", "5,0", "11.11.2026"), "5,0", StatusFailed, 2},
		{"already passed", mod("2,0", "10.03.2026"), extra("Diskrete Mathematik 2", "1,3", "11.11.2026"), "2,0", StatusPassed, 1},
		{"older failure", mod("5,0 (2.Versuch)", "10.03.2026"), extra("Diskrete Mathematik 2", "5,0", "01.02.2025"), "5,0 (2.Versuch)", StatusFailed, 2},
		{"other module", mod("5,0", "10.03.2026"), extra("Diskrete Mathematik 3", "1,0", "11.11.2026"), "5,0", StatusFailed, 1},
		{"no result", mod("5,0", "10.03.2026"), extra("Diskrete Mathematik 2", "", ""), "5,0", StatusFailed, 1},
	}
	for _, c := range cases {
		o := &Overview{Modules: []ModuleGrade{c.mod}, Extra: []ModuleGrade{c.extra}}
		o.applyExtra()
		m := o.Modules[0]
		if m.Grade != c.grade || m.Status != c.status || m.Attempt != c.attempt {
			t.Errorf("%s: %+v", c.name, m)
		}
		if replaced := m.Grade != c.mod.Grade || m.EntryDate != c.mod.EntryDate; replaced != (m.Source == SourceExtra) || replaced != (o.Extra[0].CountsFor == "I168") {
			t.Errorf("%s: marks do not match the replacement: %+v / %+v", c.name, m, o.Extra[0])
		}
	}
}

// The Notenübersicht PDF keeps listing the module with the superseded attempt.
func TestTranscriptDoesNotBringTheSupersededGradeBack(t *testing.T) {
	o := ParseOverview(fixture(t, "leistungsuebersicht_extra.html"), "https://cis.example")
	stale := &TranscriptGrades{Date: "12.11.2026", Modules: []TranscriptGrade{{ModuleNr: "I168", Title: "Diskrete Mathematik 2", Grade: "5,0 (2. Versuch)"}}}
	if c := o.ApplyTranscript(stale, true); len(c) != 0 || o.Find("I168").Grade != "2,3" {
		t.Fatalf("changed %v: %+v", c, o.Find("I168"))
	}
	// once the CIS books it on the module itself, the PDF is the news again
	booked := &TranscriptGrades{Date: "20.11.2026", Modules: []TranscriptGrade{{ModuleNr: "I168", Title: "Diskrete Mathematik 2", Grade: "2,3 (3. Versuch)", Credits: "5"}}}
	m := o.Find("I168")
	if c := o.ApplyTranscript(booked, true); len(c) != 1 || m.Source != SourceTranscript || m.RecordedAs != "" || m.Superseded != "" || m.Attempt != 3 {
		t.Fatalf("changed %v: %+v", c, m)
	}
}

func TestTitleKey(t *testing.T) {
	for a, b := range map[string]string{
		"Diskrete Mathematik II":  "Diskrete Mathematik 2",
		" diskrete  mathematik 2": "Diskrete Mathematik 2",
		"IT-Sicherheit":           "it-sicherheit",
	} {
		if TitleKey(a) != TitleKey(b) {
			t.Errorf("%q and %q are the same title", a, b)
		}
	}
	if TitleKey("Diskrete Mathematik 1") == TitleKey("Diskrete Mathematik 2") || TitleKey("Marketing") == TitleKey("Marketing I") {
		t.Error("different titles must stay apart")
	}
}
