package transfer

import "testing"

func crit(name, note, weight string) Kriterium {
	return Kriterium{Kriterium: name, Note: note, Gewichtung: weight}
}

func TestComputeGrades(t *testing.T) {
	reports := []Report{
		{ID: "1", No: "1", Topic: "A", Module: "BWL (I169)", Abgabedatum: "10.07.2024", Status: "bewertet", Wertung: "bestanden"},
		{ID: "2", No: "2", Topic: "B", Module: "PSE (I143)", Abgabedatum: "29.04.2025", Status: "bewertet", Wertung: "bestanden"},
		{ID: "3", No: "3", Topic: "C", Module: "ITO (I162)", Abgabedatum: "12.11.2026", Status: "angemeldet"},
	}
	bew := map[string]*Bewertung{
		// 80 % at 2,0 and 20 % at 3,0 = 2,2
		"1": {Kriterien: []Kriterium{crit("Inhalt", "2.0", "80 %"), crit("Methoden", "3.0", "20 %")}},
		// 2,25 is truncated to 2,2 (PVO § 17 Abs. 4), a must-pass criterion at 5,0
		"2": {Kriterien: []Kriterium{crit("Inhalt", "2,0", "75 %"), crit("Literatur. Diese Kategorie muss bestanden werden", "3,0", "25 %")}},
	}
	g := ComputeGrades(reports, bew)
	if g.Items[0].ModuleNr != "I169" {
		t.Errorf("module number from %q: %q", g.Items[0].Module, g.Items[0].ModuleNr)
	}
	if len(g.Items) != 2 {
		t.Fatalf("only assessed reports count, got %d", len(g.Items))
	}
	if g.Items[0].Grade != "2,20" || g.Items[0].Exact < 2.199 || g.Items[0].Exact > 2.201 {
		t.Errorf("first = %+v", g.Items[0])
	}
	// Transferleistungen are not graded (PVO § 18): no rounding rule applies,
	// the criteria value is shown as it is
	if g.Items[1].Grade != "2,25" || !g.Items[1].MustPassOK {
		t.Errorf("second = %+v", g.Items[1])
	}
	if g.Average != "2,23" || g.Count != 2 {
		t.Errorf("average %q count %d (mean of 2,20 and 2,25)", g.Average, g.Count)
	}

	bew["2"].Kriterien[1].Note = "5,0"
	if g := ComputeGrades(reports, bew); g.Items[1].MustPassOK {
		t.Error("a failed must-pass criterion must be flagged")
	}
}

func TestCriteriaValue(t *testing.T) {
	for in, want := range map[float64]string{2.2: "2,20", 2.55: "2,55", 1.0: "1,00", 2.225: "2,23", 2.6999999999: "2,70"} {
		if got := CriteriaValue(in); got != want {
			t.Errorf("CriteriaValue(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFinalGrade(t *testing.T) {
	for in, want := range map[float64]string{2.55: "2,5", 2.2: "2,2", 2.4: "2,4", 2.6999999999: "2,7", 1.0: "1,0"} {
		if got := FinalGrade(in); got != want {
			t.Errorf("FinalGrade(%v) = %q, want %q", in, got, want)
		}
	}
	g := ComputeGrades([]Report{{ID: "1", No: "1", Status: "bewertet"}}, map[string]*Bewertung{"1": {Kriterien: []Kriterium{crit("a", "2,0", "45 %"), crit("b", "3,0", "55 %")}}})
	if g.Items[0].Final != "2,5" || g.Final != "2,5" {
		t.Errorf("2,55 as a grade: %q / %q", g.Items[0].Final, g.Final)
	}
}
